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
	"fmt"
	"io"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"

	"github.com/btafoya/lazysshterm/internal/core/domain"
)

func openTunnel(client *ssh.Client, spec domain.ForwardSpec) (io.Closer, error) {
	switch spec.Type {
	case domain.ForwardLocal:
		return openLocalForward(client, spec)
	case domain.ForwardRemote:
		return openRemoteForward(client, spec)
	case domain.ForwardDynamic:
		return openDynamicForward(client, spec)
	default:
		return nil, fmt.Errorf("unknown forward type %v", spec.Type)
	}
}

func bindAddr(spec domain.ForwardSpec) string {
	addr := spec.BindAddr
	if addr == "" {
		addr = "localhost"
	}
	return net.JoinHostPort(addr, strconv.Itoa(spec.BindPort))
}

// acceptLoop hands every accepted connection to handle in its own goroutine
// until the listener is closed.
func acceptLoop(ln net.Listener, handle func(net.Conn)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(conn)
	}
}

// pipe copies data both directions between a and b until either side closes.
func pipe(a, b net.Conn) {
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}

// openLocalForward implements -L: listen locally, dial the destination
// through the SSH connection for each accepted connection.
func openLocalForward(client *ssh.Client, spec domain.ForwardSpec) (io.Closer, error) {
	ln, err := net.Listen("tcp", bindAddr(spec))
	if err != nil {
		return nil, err
	}
	dest := net.JoinHostPort(spec.DestHost, strconv.Itoa(spec.DestPort))
	go acceptLoop(ln, func(conn net.Conn) {
		remote, err := client.Dial("tcp", dest)
		if err != nil {
			_ = conn.Close()
			return
		}
		pipe(conn, remote)
	})
	return ln, nil
}

// openRemoteForward implements -R: ask the SSH server to listen on our
// behalf, dial the destination locally for each connection it forwards back.
func openRemoteForward(client *ssh.Client, spec domain.ForwardSpec) (io.Closer, error) {
	ln, err := client.Listen("tcp", bindAddr(spec))
	if err != nil {
		return nil, err
	}
	dest := net.JoinHostPort(spec.DestHost, strconv.Itoa(spec.DestPort))
	go acceptLoop(ln, func(conn net.Conn) {
		local, err := net.Dial("tcp", dest)
		if err != nil {
			_ = conn.Close()
			return
		}
		pipe(conn, local)
	})
	return ln, nil
}
