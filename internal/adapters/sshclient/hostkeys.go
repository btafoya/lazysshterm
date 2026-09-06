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
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyCallback wraps knownhosts.New with trust-on-first-use: an unknown
// host is accepted and recorded, a mismatched key is rejected.
func hostKeyCallback(userKnownHostsFile string) (ssh.HostKeyCallback, error) {
	path := expandHome(userKnownHostsFile)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}

	if err := ensureKnownHostsFile(path); err != nil {
		return nil, err
	}

	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, err
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			return recordHostKey(path, hostname, key)
		}
		return err
	}, nil
}

func ensureKnownHostsFile(path string) error {
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// #nosec G304 -- path is the user's own known_hosts file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func recordHostKey(path, hostname string, key ssh.PublicKey) error {
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	// #nosec G304 -- path is the user's own known_hosts file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("record host key: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(line + "\n")
	return err
}
