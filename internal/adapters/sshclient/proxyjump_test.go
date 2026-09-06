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

import "testing"

func TestParseProxyJump(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "single hop", input: "bastion", want: []string{"bastion"}},
		{name: "multi hop trims spaces", input: "bastion1, bastion2 ,bastion3", want: []string{"bastion1", "bastion2", "bastion3"}},
		{name: "drops empty segments", input: "bastion1,,bastion2", want: []string{"bastion1", "bastion2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProxyJump(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseProxyJump(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseProxyJump(%q) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestLiteralDestination(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		wantUser string
		wantHost string
		wantPort int
	}{
		{name: "host only", spec: "example.com", wantUser: "", wantHost: "example.com", wantPort: 22},
		{name: "user and host", spec: "root@example.com", wantUser: "root", wantHost: "example.com", wantPort: 22},
		{name: "user host port", spec: "root@example.com:2222", wantUser: "root", wantHost: "example.com", wantPort: 2222},
		{name: "host port no user", spec: "example.com:2222", wantUser: "", wantHost: "example.com", wantPort: 2222},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := literalDestination(tt.spec)
			if got.User != tt.wantUser || got.Host != tt.wantHost || got.Port != tt.wantPort {
				t.Fatalf("literalDestination(%q) = %+v, want user=%q host=%q port=%d",
					tt.spec, got, tt.wantUser, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestEffectivePort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want int
	}{
		{name: "explicit port", port: 2222, want: 2222},
		{name: "zero defaults to 22", port: 0, want: 22},
		{name: "negative defaults to 22", port: -1, want: 22},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := literalDestination("host")
			s.Port = tt.port
			if got := effectivePort(s); got != tt.want {
				t.Fatalf("effectivePort(port=%d) = %d, want %d", tt.port, got, tt.want)
			}
		})
	}
}
