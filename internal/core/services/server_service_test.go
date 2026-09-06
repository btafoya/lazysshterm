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

package services

import (
	"errors"
	"io"
	"testing"

	"github.com/btafoya/lazysshterm/internal/core/domain"
	"github.com/btafoya/lazysshterm/internal/core/ports"
	"go.uber.org/zap"
)

// fakeRepo is a minimal ports.ServerRepository backed by an in-memory slice.
type fakeRepo struct {
	servers []domain.Server
}

func (f *fakeRepo) ListServers(string) ([]domain.Server, error)     { return f.servers, nil }
func (f *fakeRepo) UpdateServer(domain.Server, domain.Server) error { return nil }
func (f *fakeRepo) AddServer(domain.Server) error                   { return nil }
func (f *fakeRepo) DeleteServer(domain.Server) error                { return nil }
func (f *fakeRepo) SetPinned(string, bool) error                    { return nil }
func (f *fakeRepo) RecordSSH(string) error                          { return nil }

// fakeConnector is a minimal ports.SSHConnector with configurable results.
type fakeConnector struct {
	encryptedKeys []string
	connectErr    error
	forwardErr    error
}

func (f *fakeConnector) EncryptedKeys(domain.Server) []string { return f.encryptedKeys }
func (f *fakeConnector) Connect(domain.Server, domain.Credentials, ports.SessionIO, *domain.ForwardSpec) error {
	return f.connectErr
}

type fakeTunnel struct{ closed bool }

func (t *fakeTunnel) Close() error { t.closed = true; return nil }

func (f *fakeConnector) Forward(domain.Server, domain.Credentials, domain.ForwardSpec) (io.Closer, error) {
	if f.forwardErr != nil {
		return nil, f.forwardErr
	}
	return &fakeTunnel{}, nil
}

func newTestService(repo *fakeRepo, conn *fakeConnector) *serverService {
	return &serverService{
		logger:           zap.NewNop().Sugar(),
		serverRepository: repo,
		connector:        conn,
	}
}

func TestFindServer(t *testing.T) {
	repo := &fakeRepo{servers: []domain.Server{{Alias: "prod"}, {Alias: "staging"}}}
	svc := newTestService(repo, &fakeConnector{})

	got, err := svc.findServer("staging")
	if err != nil {
		t.Fatalf("findServer(staging) unexpected error: %v", err)
	}
	if got.Alias != "staging" {
		t.Fatalf("findServer(staging) = %+v, want alias staging", got)
	}

	if _, err := svc.findServer("missing"); err == nil {
		t.Fatal("findServer(missing) expected error, got nil")
	}
}

func TestEncryptedIdentityFiles(t *testing.T) {
	repo := &fakeRepo{servers: []domain.Server{{Alias: "prod"}}}
	conn := &fakeConnector{encryptedKeys: []string{"~/.ssh/id_ed25519"}}
	svc := newTestService(repo, conn)

	paths, err := svc.EncryptedIdentityFiles("prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "~/.ssh/id_ed25519" {
		t.Fatalf("EncryptedIdentityFiles = %v, want [~/.ssh/id_ed25519]", paths)
	}

	if _, err := svc.EncryptedIdentityFiles("missing"); err == nil {
		t.Fatal("expected error for unknown alias")
	}
}

func TestStartAndStopForwarding(t *testing.T) {
	repo := &fakeRepo{servers: []domain.Server{{Alias: "prod"}}}
	svc := newTestService(repo, &fakeConnector{})

	id, err := svc.StartForward("prod", domain.Credentials{}, domain.ForwardSpec{
		Type: domain.ForwardLocal, BindPort: 8080, DestHost: "internal", DestPort: 80,
	})
	if err != nil {
		t.Fatalf("StartForward unexpected error: %v", err)
	}
	if id != "L 8080->internal:80" {
		t.Fatalf("StartForward id = %q, want %q", id, "L 8080->internal:80")
	}
	if !svc.IsForwarding("prod") {
		t.Fatal("IsForwarding(prod) = false, want true after StartForward")
	}

	tunnel := svc.forwards["prod"][0].(*fakeTunnel)

	if err := svc.StopForwarding("prod"); err != nil {
		t.Fatalf("StopForwarding unexpected error: %v", err)
	}
	if svc.IsForwarding("prod") {
		t.Fatal("IsForwarding(prod) = true, want false after StopForwarding")
	}
	if !tunnel.closed {
		t.Fatal("StopForwarding did not close the tunnel")
	}
}

func TestForwardID(t *testing.T) {
	tests := []struct {
		name string
		spec domain.ForwardSpec
		want string
	}{
		{name: "local", spec: domain.ForwardSpec{Type: domain.ForwardLocal, BindPort: 8080, DestHost: "db", DestPort: 5432}, want: "L 8080->db:5432"},
		{name: "remote", spec: domain.ForwardSpec{Type: domain.ForwardRemote, BindPort: 80, DestHost: "localhost", DestPort: 3000}, want: "R 80->localhost:3000"},
		{name: "dynamic", spec: domain.ForwardSpec{Type: domain.ForwardDynamic, BindPort: 1080}, want: "D 1080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forwardID(tt.spec); got != tt.want {
				t.Fatalf("forwardID(%+v) = %q, want %q", tt.spec, got, tt.want)
			}
		})
	}
}

func TestConnectWrapsConnector(t *testing.T) {
	repo := &fakeRepo{servers: []domain.Server{{Alias: "prod"}}}
	wantErr := errors.New("boom")
	svc := newTestService(repo, &fakeConnector{connectErr: wantErr})

	err := svc.SSH("prod", domain.Credentials{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SSH() error = %v, want %v", err, wantErr)
	}

	if err := svc.SSH("missing", domain.Credentials{}); err == nil {
		t.Fatal("SSH(missing) expected error, got nil")
	}
}
