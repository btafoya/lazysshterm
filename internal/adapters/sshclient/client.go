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

// Package sshclient implements ports.SSHConnector on golang.org/x/crypto/ssh,
// so lazysshterm connects, opens a shell, and forwards ports without depending on
// a system ssh/ssh.exe binary.
package sshclient

import (
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"

	"github.com/btafoya/lazysshterm/internal/core/domain"
	"github.com/btafoya/lazysshterm/internal/core/ports"
)

// Connector implements ports.SSHConnector.
type Connector struct {
	repo ports.ServerRepository
}

// NewConnector builds a Connector. repo is used to resolve ProxyJump hops
// that match a configured alias.
func NewConnector(repo ports.ServerRepository) *Connector {
	return &Connector{repo: repo}
}

func (c *Connector) EncryptedKeys(server domain.Server) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s domain.Server) {
		for _, path := range s.IdentityFiles {
			if seen[path] {
				continue
			}
			seen[path] = true
			if isEncrypted(path) {
				out = append(out, path)
			}
		}
	}
	for _, hop := range parseProxyJump(server.ProxyJump) {
		if srv, ok := c.lookupAlias(hop); ok {
			add(srv)
		}
	}
	add(server)
	return out
}

func (c *Connector) Connect(server domain.Server, creds domain.Credentials, sio ports.SessionIO, forward *domain.ForwardSpec) error {
	client, err := c.dialChain(server, creds)
	if err != nil {
		return authError(server, creds, err)
	}
	defer func() { _ = client.Close() }()

	if forward != nil {
		tun, err := openTunnel(client, *forward)
		if err != nil {
			return fmt.Errorf("start forward: %w", err)
		}
		defer func() { _ = tun.Close() }()
	}

	return runShell(client, sio)
}

func (c *Connector) Forward(server domain.Server, creds domain.Credentials, spec domain.ForwardSpec) (io.Closer, error) {
	client, err := c.dialChain(server, creds)
	if err != nil {
		return nil, authError(server, creds, err)
	}
	tun, err := openTunnel(client, spec)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &clientTunnel{client: client, inner: tun}, nil
}

// clientTunnel closes both the forward-specific listener and the dedicated
// *ssh.Client opened for it (Connect reuses one client for shell+forward and
// closes it itself; Forward opens its own, so the tunnel owns its lifetime).
type clientTunnel struct {
	client *ssh.Client
	inner  io.Closer
}

func (t *clientTunnel) Close() error {
	_ = t.inner.Close()
	return t.client.Close()
}

// authError turns a dial failure into ports.ErrPasswordRequired when no
// password was supplied yet and the server permits password auth.
// ponytail: doesn't distinguish "auth rejected" from "host unreachable" —
// a real network failure will also prompt for a password once. Add
// error-type inspection if that proves annoying in practice.
func authError(server domain.Server, creds domain.Credentials, err error) error {
	if creds.Password == "" && server.PasswordAuthentication != "no" {
		return fmt.Errorf("%w: %w", ports.ErrPasswordRequired, err)
	}
	return err
}
