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
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/btafoya/lazysshterm/internal/core/ports"
)

// runShell opens an interactive PTY shell on client and blocks until it
// exits, streaming through sio. Raw terminal mode and resize polling only
// apply when sio.Stdin is an actual terminal.
func runShell(client *ssh.Client, sio ports.SessionIO) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	session.Stdin = sio.Stdin
	session.Stdout = sio.Stdout
	session.Stderr = sio.Stderr

	fd := int(sio.Stdin.Fd())
	raw := term.IsTerminal(fd)

	w, h := 80, 24
	if raw {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("make raw terminal: %w", err)
		}
		defer func() { _ = term.Restore(fd, state) }()
		if cw, ch, err := term.GetSize(fd); err == nil {
			w, h = cw, ch
		}
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
	}
	if err := session.RequestPty(termEnv, h, w, modes); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	done := make(chan struct{})
	if raw {
		go pollResize(session, fd, w, h, done)
	}

	err = session.Wait()
	close(done)

	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitStatus() == 130 {
		return nil // Ctrl-C exit, not a failure.
	}
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	return nil
}

// pollResize watches the local terminal size and forwards changes as SSH
// window-change requests. Windows has no SIGWINCH, so polling is used on
// both platforms for one code path.
func pollResize(session *ssh.Session, fd, lastW, lastH int, done <-chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			w, h, err := term.GetSize(fd)
			if err != nil || (w == lastW && h == lastH) {
				continue
			}
			lastW, lastH = w, h
			_ = session.WindowChange(h, w)
		}
	}
}
