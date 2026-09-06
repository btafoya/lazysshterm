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
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/btafoya/lazysshterm/internal/core/domain"
)

// buildAuthMethods tries, in order: identity files (decrypted with any
// passphrase supplied in creds), the local ssh-agent, then a password if one
// was supplied. The ssh package itself tries each method in turn and stops
// at the first success.
func buildAuthMethods(server domain.Server, creds domain.Credentials) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	var signers []ssh.Signer
	for _, path := range server.IdentityFiles {
		if signer, err := loadSigner(path, creds.KeyPassphrases[path]); err == nil {
			signers = append(signers, signer)
		}
	}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}

	// ponytail: only the default SSH_AUTH_SOCK agent is tried, not a custom
	// IdentityAgent path. Add path support if a server config needs it.
	if server.IdentityAgent != "none" {
		if am, ok := agentAuthMethod(); ok {
			methods = append(methods, am)
		}
	}

	if creds.Password != "" {
		methods = append(methods, ssh.Password(creds.Password))
	}

	return methods
}

func agentAuthMethod() (ssh.AuthMethod, bool) {
	conn, err := dialAgent()
	if err != nil {
		return nil, false
	}
	ag := agent.NewClient(conn)
	return ssh.PublicKeysCallback(ag.Signers), true
}

func isEncrypted(path string) bool {
	data, err := readKeyFile(path)
	if err != nil {
		return false
	}
	_, err = ssh.ParsePrivateKey(data)
	var passErr *ssh.PassphraseMissingError
	return errors.As(err, &passErr)
}

func loadSigner(path, passphrase string) (ssh.Signer, error) {
	data, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		return ssh.ParsePrivateKey(data)
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
}

func readKeyFile(path string) ([]byte, error) {
	// #nosec G304 -- identity file paths come from the user's own ssh config
	return os.ReadFile(expandHome(path))
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

func currentUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}
