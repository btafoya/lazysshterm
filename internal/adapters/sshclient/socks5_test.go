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
	"net"
	"testing"
)

func TestSocks5Handshake(t *testing.T) {
	tests := []struct {
		name    string
		request []byte
		want    string
		wantErr bool
	}{
		{
			// greeting (ver=5, nmethods=1, no-auth) + CONNECT request for a
			// domain name of length 11, port 443 (0x01BB)
			name: "domain name",
			request: append([]byte{0x05, 0x01, 0x00},
				append([]byte{0x05, 0x01, 0x00, 0x03, 11},
					append([]byte("example.com"), 0x01, 0xBB)...)...),
			want: "example.com:443",
		},
		{
			// same greeting + CONNECT request for an IPv4 address, port 8080 (0x1F90)
			name: "ipv4",
			request: append([]byte{0x05, 0x01, 0x00},
				append([]byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1}, 0x1F, 0x90)...),
			want: "127.0.0.1:8080",
		},
		{
			name:    "wrong version rejected",
			request: []byte{0x04, 0x01, 0x00},
			wantErr: true,
		},
		{
			name: "non-connect command rejected",
			request: append([]byte{0x05, 0x01, 0x00},
				[]byte{0x05, 0x02, 0x00, 0x01, 127, 0, 0, 1, 0, 80}...), // BIND, not CONNECT
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			// Write and drain-read run in separate goroutines: the server
			// side interleaves its own reads and writes (greeting reply,
			// then the request), so a single blocking client.Write would
			// deadlock against a single later client.Read.
			go func() { _, _ = client.Write(tt.request) }()
			go func() {
				buf := make([]byte, 32)
				_, _ = client.Read(buf)
			}()

			got, err := socks5Handshake(server)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("socks5Handshake() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("socks5Handshake() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("socks5Handshake() = %q, want %q", got, tt.want)
			}
		})
	}
}
