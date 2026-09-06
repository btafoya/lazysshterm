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

package ports

import (
	"errors"
	"io"
	"os"

	"github.com/btafoya/lazysshterm/internal/core/domain"
)

// ErrPasswordRequired signals that no key/agent auth method was usable and
// the server still accepts password authentication.
var ErrPasswordRequired = errors.New("password required")

// SessionIO wires an interactive shell session to real terminal streams.
// Stdin is a concrete *os.File because raw-mode/PTY sizing needs its fd.
type SessionIO struct {
	Stdin          *os.File
	Stdout, Stderr io.Writer
}

// SSHConnector opens native SSH connections and tunnels without shelling
// out to a system ssh binary. Implemented by internal/adapters/sshclient.
type SSHConnector interface {
	// EncryptedKeys returns identity file paths (for server and, recursively,
	// any ProxyJump hops resolvable to a configured alias) that are
	// passphrase-protected and need one collected before connecting.
	EncryptedKeys(server domain.Server) []string

	// Connect opens an interactive shell session and blocks until it exits.
	// If forward is non-nil, a tunnel is opened on the same connection and
	// closed when the shell session ends.
	Connect(server domain.Server, creds domain.Credentials, sio SessionIO, forward *domain.ForwardSpec) error

	// Forward opens one background tunnel; it runs until Close is called.
	Forward(server domain.Server, creds domain.Credentials, spec domain.ForwardSpec) (io.Closer, error)
}
