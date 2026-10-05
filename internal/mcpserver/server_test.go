// Copyright 2026 Zen Mesh. All rights reserved.

package mcpserver

import (
	"encoding/json"
	"testing"
)

func TestDefaultDeny(t *testing.T) {
	t.Setenv(ModeEnv, "")
	t.Setenv(ActorEnv, "")
	a := New()
	if a.Enabled() {
		t.Fatal("the adapter must be dark without the mode env (default-deny)")
	}
	if _, err := a.ListJSON(); err != ErrNotEnabled {
		t.Fatalf("the tools/list must refuse typed-denied: %v", err)
	}
	if len(a.Tools()) != 0 {
		t.Fatal("the tools must not enumerate when denied")
	}
}

func TestEnabledRequiresActorAttribution(t *testing.T) {
	t.Setenv(ModeEnv, MCPModeStdio)
	t.Setenv(ActorEnv, "")
	a := New()
	if !a.Enabled() {
		t.Fatal("the mode env alone enables the transport")
	}
	if a.Actor() != "" {
		t.Fatalf("no actor env = no attribution: %q", a.Actor())
	}
}

func TestEnabledEnumeratesReadOnlyFamily(t *testing.T) {
	t.Setenv(ModeEnv, MCPModeStdio)
	t.Setenv(ActorEnv, "opencode-rotation")
	a := New()
	tools := a.Tools()
	if len(tools) != 2 {
		t.Fatalf("want the 2-tool read-only family, got %v", tools)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
		for _, banned := range []string{"delete", "execute", "start", "run", "trigger"} {
			if containsFold(tl.Name, banned) {
				t.Fatalf("the MCP family must be READ-ONLY: %q carries %q", tl.Name, banned)
			}
		}
	}
	for _, want := range []string{"entitlement_status", "health_summary"} {
		if !names[want] {
			t.Fatalf("missing the %q tool: %v", want, names)
		}
	}
}

func TestListJSONShape(t *testing.T) {
	t.Setenv(ModeEnv, MCPModeStdio)
	t.Setenv(ActorEnv, "test")
	a := New()
	body, err := a.ListJSON()
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("the tools/list body must be valid JSON: %v", err)
	}
	if len(parsed.Tools) != 2 {
		t.Fatalf("want 2 tools, got %d", len(parsed.Tools))
	}
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if toLower(s[i:i+len(sub)]) == toLower(sub) {
				return true
			}
		}
		return false
	})()
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
