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

//go:build windows

package sshclient

import (
	"errors"
	"net"
)

// ponytail: no Pageant/named-pipe agent support yet — identity file and
// password auth still work on Windows. Add a named-pipe dial (needs
// golang.org/x/sys/windows) if agent auth on Windows is actually needed.
func dialAgent() (net.Conn, error) {
	return nil, errors.New("ssh agent not supported on windows yet")
}
