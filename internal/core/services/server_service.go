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
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/btafoya/lazysshterm/internal/core/domain"
	"github.com/btafoya/lazysshterm/internal/core/ports"
	"go.uber.org/zap"
)

type serverService struct {
	serverRepository ports.ServerRepository
	connector        ports.SSHConnector
	logger           *zap.SugaredLogger

	fwMu     sync.Mutex
	forwards map[string][]io.Closer
}

// NewServerService creates a new instance of serverService.
func NewServerService(logger *zap.SugaredLogger, sr ports.ServerRepository, conn ports.SSHConnector) ports.ServerService {
	return &serverService{
		logger:           logger,
		serverRepository: sr,
		connector:        conn,
	}
}

// ListServers returns a list of servers sorted with pinned on top.
func (s *serverService) ListServers(query string) ([]domain.Server, error) {
	servers, err := s.serverRepository.ListServers(query)
	if err != nil {
		s.logger.Errorw("failed to list servers", "error", err)
		return nil, err
	}

	// Sort: pinned first (PinnedAt non-zero), then by PinnedAt desc, then by Alias asc.
	sort.SliceStable(servers, func(i, j int) bool {
		pi := !servers[i].PinnedAt.IsZero()
		pj := !servers[j].PinnedAt.IsZero()
		if pi != pj {
			return pi
		}
		if pi && pj {
			return servers[i].PinnedAt.After(servers[j].PinnedAt)
		}
		return servers[i].Alias < servers[j].Alias
	})

	return servers, nil
}

// findServer looks up a configured server by exact alias.
func (s *serverService) findServer(alias string) (domain.Server, error) {
	servers, err := s.serverRepository.ListServers("")
	if err != nil {
		return domain.Server{}, err
	}
	for _, srv := range servers {
		if srv.Alias == alias {
			return srv, nil
		}
	}
	return domain.Server{}, fmt.Errorf("server with alias '%s' not found", alias)
}

// validateServer performs core validation of server fields.
func validateServer(srv domain.Server) error {
	if strings.TrimSpace(srv.Alias) == "" {
		return fmt.Errorf("alias is required")
	}
	if ok, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+$`, srv.Alias); !ok {
		return fmt.Errorf("alias may contain letters, digits, dot, dash, underscore")
	}
	if strings.TrimSpace(srv.Host) == "" {
		return fmt.Errorf("Host/IP is required")
	}
	if ip := net.ParseIP(srv.Host); ip == nil {
		if strings.Contains(srv.Host, " ") {
			return fmt.Errorf("host must not contain spaces")
		}
		if ok, _ := regexp.MatchString(`^[A-Za-z0-9.-]+$`, srv.Host); !ok {
			return fmt.Errorf("host contains invalid characters")
		}
		if strings.HasPrefix(srv.Host, ".") || strings.HasSuffix(srv.Host, ".") {
			return fmt.Errorf("host must not start or end with a dot")
		}
		for _, lbl := range strings.Split(srv.Host, ".") {
			if lbl == "" {
				return fmt.Errorf("host must not contain empty labels")
			}
			if strings.HasPrefix(lbl, "-") || strings.HasSuffix(lbl, "-") {
				return fmt.Errorf("hostname labels must not start or end with a hyphen")
			}
		}
	}
	if srv.Port != 0 && (srv.Port < 1 || srv.Port > 65535) {
		return fmt.Errorf("port must be a number between 1 and 65535")
	}
	return nil
}

// UpdateServer updates an existing server with new details.
func (s *serverService) UpdateServer(server domain.Server, newServer domain.Server) error {
	if err := validateServer(newServer); err != nil {
		s.logger.Warnw("validation failed on update", "error", err, "server", newServer)
		return err
	}
	err := s.serverRepository.UpdateServer(server, newServer)
	if err != nil {
		s.logger.Errorw("failed to update server", "error", err, "server", server)
	}
	return err
}

// AddServer adds a new server to the repository.
func (s *serverService) AddServer(server domain.Server) error {
	if err := validateServer(server); err != nil {
		s.logger.Warnw("validation failed on add", "error", err, "server", server)
		return err
	}
	err := s.serverRepository.AddServer(server)
	if err != nil {
		s.logger.Errorw("failed to add server", "error", err, "server", server)
	}
	return err
}

// DeleteServer removes a server from the repository.
func (s *serverService) DeleteServer(server domain.Server) error {
	err := s.serverRepository.DeleteServer(server)
	if err != nil {
		s.logger.Errorw("failed to delete server", "error", err, "server", server)
	}
	return err
}

// SetPinned sets or clears a pin timestamp for the server alias.
func (s *serverService) SetPinned(alias string, pinned bool) error {
	err := s.serverRepository.SetPinned(alias, pinned)
	if err != nil {
		s.logger.Errorw("failed to set pin state", "error", err, "alias", alias, "pinned", pinned)
	}
	return err
}

// EncryptedIdentityFiles returns identity file paths that need a passphrase
// before connecting, for the server and any resolvable ProxyJump hops.
func (s *serverService) EncryptedIdentityFiles(alias string) ([]string, error) {
	server, err := s.findServer(alias)
	if err != nil {
		return nil, err
	}
	return s.connector.EncryptedKeys(server), nil
}

// SSH starts a native interactive SSH session to the given alias.
func (s *serverService) SSH(alias string, creds domain.Credentials) error {
	return s.connect(alias, creds, nil)
}

// SSHWithForward starts a native interactive SSH session with a tunnel open
// alongside it for the session's lifetime.
func (s *serverService) SSHWithForward(alias string, creds domain.Credentials, spec domain.ForwardSpec) error {
	return s.connect(alias, creds, &spec)
}

func (s *serverService) connect(alias string, creds domain.Credentials, forward *domain.ForwardSpec) error {
	server, err := s.findServer(alias)
	if err != nil {
		return err
	}

	s.logger.Infow("ssh start", "alias", alias)
	sio := ports.SessionIO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	if err := s.connector.Connect(server, creds, sio, forward); err != nil {
		s.logger.Errorw("ssh failed", "alias", alias, "error", err)
		return err
	}

	if err := s.serverRepository.RecordSSH(alias); err != nil {
		s.logger.Errorw("failed to record ssh metadata", "alias", alias, "error", err)
	}
	s.logger.Infow("ssh end", "alias", alias)
	return nil
}

// StartForward opens a background tunnel and tracks it for StopForwarding.
// Returns a short id describing the tunnel for status display.
func (s *serverService) StartForward(alias string, creds domain.Credentials, spec domain.ForwardSpec) (string, error) {
	server, err := s.findServer(alias)
	if err != nil {
		return "", err
	}

	tunnel, err := s.connector.Forward(server, creds, spec)
	if err != nil {
		s.logger.Errorw("forward failed", "alias", alias, "error", err)
		return "", err
	}

	s.fwMu.Lock()
	if s.forwards == nil {
		s.forwards = make(map[string][]io.Closer)
	}
	s.forwards[alias] = append(s.forwards[alias], tunnel)
	s.fwMu.Unlock()

	return forwardID(spec), nil
}

// StopForwarding closes all active tunnels for the alias.
func (s *serverService) StopForwarding(alias string) error {
	s.fwMu.Lock()
	tunnels := s.forwards[alias]
	delete(s.forwards, alias)
	s.fwMu.Unlock()

	var errs []error
	for _, t := range tunnels {
		if err := t.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors stopping forwards: %v", errs)
	}
	return nil
}

// IsForwarding reports whether there is at least one active forward for alias.
func (s *serverService) IsForwarding(alias string) bool {
	s.fwMu.Lock()
	defer s.fwMu.Unlock()
	return len(s.forwards[alias]) > 0
}

// Ping checks if the server is reachable on its SSH port.
func (s *serverService) Ping(server domain.Server) (bool, time.Duration, error) {
	start := time.Now()

	host := strings.TrimSpace(server.Host)
	if host == "" {
		host = server.Alias
	}
	port := server.Port
	if port <= 0 {
		port = 22
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return false, time.Since(start), err
	}
	_ = conn.Close()
	return true, time.Since(start), nil
}

func forwardID(spec domain.ForwardSpec) string {
	kind := "L"
	switch spec.Type {
	case domain.ForwardRemote:
		kind = "R"
	case domain.ForwardDynamic:
		kind = "D"
	case domain.ForwardLocal:
	}
	if spec.Type == domain.ForwardDynamic {
		return fmt.Sprintf("%s %d", kind, spec.BindPort)
	}
	return fmt.Sprintf("%s %d->%s:%d", kind, spec.BindPort, spec.DestHost, spec.DestPort)
}
