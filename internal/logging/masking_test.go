/*
Copyright 2026 Zen Mesh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logging

import (
	"strings"
	"testing"
)

func TestMaskUUID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string stays empty", "", ""},
		{"short string returned as-is", "abc", "abc"},
		{"exactly 8 chars returned as-is", "12345678", "12345678"},
		{"9 chars truncated to 8 + ellipsis", "123456789", "12345678..."},
		{"full uuid masked to first 8", "550e8400-e29b-41d4-a716-446655440000", "550e8400..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskUUID(tt.in); got != tt.want {
				t.Errorf("MaskUUID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string stays empty", "", ""},
		{"3 chars fully hidden", "abc", "***"},
		{"exactly 4 chars fully hidden", "abcd", "***"},
		{"5 chars truncated to 4 + ellipsis", "abcde", "abcd..."},
		{"api key masked to first 4", "sk-live-abcdef1234", "sk-l..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskToken(tt.in); got != tt.want {
				t.Errorf("MaskToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string stays empty", "", ""},
		{"local part shorter than 3 fully hidden", "ab@example.com", "***@example.com"},
		{"local part of 3 fully hidden", "abc@example.com", "***@example.com"},
		{"long local part keeps first 3", "alice@example.com", "ali...@example.com"},
		{"no at sign fully hidden", "not-an-email", "***"},
		{"multiple at signs fully hidden", "a@b@c", "***"},
		{"empty local part", "@example.com", "***@example.com"},
		{"empty domain", "user@", "use...@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskEmail(tt.in); got != tt.want {
				t.Errorf("MaskEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskIP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string stays empty", "", ""},
		{"ipv4 keeps first octet", "192.168.1.100", "192.x.x.x"},
		{"all zero ipv4", "0.0.0.0", "0.x.x.x"},
		{"octet values are not validated", "999.999.999.999", "999.x.x.x"},
		{"three parts fully hidden", "1.2.3", "***"},
		{"five parts fully hidden", "1.2.3.4.5", "***"},
		{"ipv6 fully hidden", "::1", "***"},
		{"garbage fully hidden", "not-an-ip", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskIP(tt.in); got != tt.want {
				t.Errorf("MaskIP(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeSQL(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"empty query stays empty", "", ""},
		{"query without literals unchanged", "SELECT * FROM users", "SELECT * FROM users"},
		{
			"string literal replaced",
			"SELECT * FROM users WHERE name = 'bob'",
			"SELECT * FROM users WHERE name = '?'",
		},
		{
			"numeric literal replaced",
			"SELECT * FROM users WHERE id = 42",
			"SELECT * FROM users WHERE id = ?",
		},
		{
			"quoted uuid hidden by literal rule",
			"SELECT * FROM t WHERE id = '550e8400-e29b-41d4-a716-446655440000'",
			"SELECT * FROM t WHERE id = '?'",
		},
		{
			"unquoted uuid hidden by uuid rule",
			"SELECT * FROM t WHERE id = deadbeef-cafe-babe-face-feedfacefeed",
			"SELECT * FROM t WHERE id = ?",
		},
		{
			"multiple literals replaced",
			"UPDATE t SET a = 1, b = 'x' WHERE c = 2",
			"UPDATE t SET a = ?, b = '?' WHERE c = ?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeSQL(tt.query); got != tt.want {
				t.Errorf("SanitizeSQL(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestSanitizeSQLTruncatesLongQueries(t *testing.T) {
	query := strings.Repeat("a", 250)
	got := SanitizeSQL(query)
	if len(got) != 203 {
		t.Errorf("SanitizeSQL(long) length = %d, want 203", len(got))
	}
	want := strings.Repeat("a", 200) + "..."
	if got != want {
		t.Errorf("SanitizeSQL(long) = %q..., want 200 chars + ellipsis", got[:20])
	}
}

func TestRedactPassword(t *testing.T) {
	for _, in := range []string{"", "hunter2", "p@ssw0rd with spaces"} {
		if got := RedactPassword(in); got != "[REDACTED]" {
			t.Errorf("RedactPassword(%q) = %q, want [REDACTED]", in, got)
		}
	}
}

func TestRedactSecret(t *testing.T) {
	for _, in := range []string{"", "s3cr3t", "token-value-123"} {
		if got := RedactSecret(in); got != "[REDACTED]" {
			t.Errorf("RedactSecret(%q) = %q, want [REDACTED]", in, got)
		}
	}
}
