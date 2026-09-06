// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sshclient

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"

	"github.com/btafoya/lazysshterm/internal/core/domain"
)

// openDynamicForward implements -D: a minimal local SOCKS5 proxy (CONNECT
// only, no auth) that tunnels each accepted connection over the SSH client.
// ponytail: hand-rolled instead of a SOCKS5 dependency — this is the entire
// subset lazysshterm needs. Add BIND/UDP ASSOCIATE/auth if that ever changes.
func openDynamicForward(client *ssh.Client, spec domain.ForwardSpec) (io.Closer, error) {
	ln, err := net.Listen("tcp", bindAddr(spec))
	if err != nil {
		return nil, err
	}
	go acceptLoop(ln, func(conn net.Conn) { serveSocks5(client, conn) })
	return ln, nil
}

func serveSocks5(client *ssh.Client, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	target, err := socks5Handshake(conn)
	if err != nil {
		return
	}
	remote, err := client.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	pipe(conn, remote)
}

// socks5Handshake performs the minimal SOCKS5 negotiation: no-auth greeting,
// then a CONNECT request. Returns the requested "host:port" destination.
func socks5Handshake(conn net.Conn) (string, error) {
	buf := make([]byte, 262)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 {
		return "", errors.New("unsupported socks version")
	}
	nmethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 || buf[1] != 0x01 {
		return "", errors.New("unsupported socks command")
	}

	var host string
	switch buf[3] {
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return "", err
		}
		host = net.IP(buf[:4]).String()
	case 0x03: // domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return "", err
		}
		n := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:n]); err != nil {
			return "", err
		}
		host = string(buf[:n])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return "", err
		}
		host = net.IP(buf[:16]).String()
	default:
		return "", errors.New("unsupported address type")
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}
