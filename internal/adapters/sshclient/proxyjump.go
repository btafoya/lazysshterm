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
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/btafoya/lazysshterm/internal/core/domain"
)

// dialChain establishes the *ssh.Client for server, walking any ProxyJump
// hops first (comma-separated, OpenSSH -J syntax). Each hop is resolved
// against the repository by alias when one matches (using that hop's own
// config), otherwise treated as a literal user@host[:port] destination
// reusing the target's credentials.
func (c *Connector) dialChain(server domain.Server, creds domain.Credentials) (*ssh.Client, error) {
	var client *ssh.Client
	for _, hop := range parseProxyJump(server.ProxyJump) {
		hopServer := c.resolveHop(hop)
		next, err := c.dialHop(client, hopServer, creds)
		if err != nil {
			if client != nil {
				_ = client.Close()
			}
			return nil, fmt.Errorf("proxy jump %s: %w", hop, err)
		}
		client = next
	}

	target, err := c.dialHop(client, server, creds)
	if err != nil && client != nil {
		_ = client.Close()
	}
	return target, err
}

func parseProxyJump(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var hops []string
	for _, h := range strings.Split(value, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hops = append(hops, h)
		}
	}
	return hops
}

func (c *Connector) resolveHop(hop string) domain.Server {
	if srv, ok := c.lookupAlias(hop); ok {
		return srv
	}
	return literalDestination(hop)
}

func (c *Connector) lookupAlias(alias string) (domain.Server, bool) {
	servers, err := c.repo.ListServers("")
	if err != nil {
		return domain.Server{}, false
	}
	for _, s := range servers {
		if s.Alias == alias {
			return s, true
		}
	}
	return domain.Server{}, false
}

// literalDestination parses a raw "user@host[:port]" ProxyJump token that
// doesn't match a configured alias.
func literalDestination(spec string) domain.Server {
	user, hostport, hasUser := strings.Cut(spec, "@")
	if !hasUser {
		hostport = user
		user = ""
	}
	host, portStr, err := net.SplitHostPort(hostport)
	port := 22
	if err != nil {
		host = hostport
	} else if p, perr := strconv.Atoi(portStr); perr == nil {
		port = p
	}
	return domain.Server{Alias: spec, Host: host, User: user, Port: port}
}

// dialHop opens a *ssh.Client to target, either directly (via == nil) or
// tunneled through an already-established hop connection.
func (c *Connector) dialHop(via *ssh.Client, target domain.Server, creds domain.Credentials) (*ssh.Client, error) {
	cfg, err := clientConfig(target, creds)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(target.Host, strconv.Itoa(effectivePort(target)))

	if via == nil {
		return ssh.Dial("tcp", addr, cfg)
	}

	conn, err := via.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(ncc, chans, reqs), nil
}

func clientConfig(server domain.Server, creds domain.Credentials) (*ssh.ClientConfig, error) {
	hostKeyCB, err := hostKeyCallback(server.UserKnownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	user := server.User
	if user == "" {
		user = currentUsername()
	}
	return &ssh.ClientConfig{
		User:            user,
		Auth:            buildAuthMethods(server, creds),
		HostKeyCallback: hostKeyCB,
		Timeout:         10 * time.Second,
	}, nil
}

func effectivePort(server domain.Server) int {
	if server.Port > 0 {
		return server.Port
	}
	return 22
}
