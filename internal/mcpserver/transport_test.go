// Copyright 2026 Zen Mesh. All rights reserved.

package mcpserver

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStdioDefaultDenyRefusesToolsList(t *testing.T) {
	t.Setenv(ModeEnv, "")
	a := New(nil, nil)
	var out bytes.Buffer
	if err := a.RunStdio(strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`), &out); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		ID    int `json:"id"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("the tools/list must refuse typed-denied: %+v", resp)
	}
}

func TestStdioEnabledEnumeratesReadOnlyFamily(t *testing.T) {
	t.Setenv(ModeEnv, MCPModeStdio)
	t.Setenv(ActorEnv, "test")
	a := New(nil, nil)
	var out bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n")
	if err := a.RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("want 2 responses, got %q", out.String())
	}
	var list struct {
		ID     int `json:"id"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	found := false
	for _, line := range lines {
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &resp) == nil && resp.ID == 2 {
			found = true
			if len(resp.Result.Tools) != 2 {
				t.Fatalf("want the 2-tool family, got %v", resp.Result.Tools)
			}
		}
	}
	if !found {
		t.Fatalf("the tools/list response missing; snapshot: %s", snap(t, snap(t, "")))
	}
	_ = snap
	_ = list
}

func snap(t *testing.T, s string) string { t.Helper(); return s }

// The stdio call path with NO provider wired answers the EMPTY body —
// never a panic (the 81fa94e posture; the live drive s121 caught the
// nil dereference this test pins).
func TestStdioCallWithNilProvidersAnswersEmptyBody(t *testing.T) {
	t.Setenv(ModeEnv, "stdio")
	t.Setenv(ActorEnv, "opencode-s121")
	a := New(nil, nil)
	var out bytes.Buffer
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"health_summary","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"entitlement_status","arguments":{}}}`,
		"",
	}, "\n")
	if err := a.RunStdio(strings.NewReader(in), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	responses := map[int]map[string]any{}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("response line: %v (%s)", err, line)
		}
		var body map[string]any
		_ = json.Unmarshal(r.Result, &body)
		responses[r.ID] = body
	}
	for _, id := range []int{2, 3} {
		body, ok := responses[id]
		if !ok {
			t.Fatalf("no response for id %d (a panic truncates the session)", id)
		}
		content, _ := body["content"].([]any)
		if len(content) != 1 {
			t.Fatalf("id %d: content = %v, want one text entry", id, body["content"])
		}
		entry := content[0].(map[string]any)
		if entry["text"] != "{}" {
			t.Fatalf("id %d: body = %v, want the empty {}", id, entry["text"])
		}
	}
}

// A notification (an id-less message) is NEVER answered — the JSON-RPC
// law strict MCP clients rely on for notifications/initialized.
func TestStdioNotificationGetsNoResponse(t *testing.T) {
	t.Setenv(ModeEnv, "stdio")
	t.Setenv(ActorEnv, "opencode-s121")
	a := New(nil, nil)
	var out bytes.Buffer
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		"",
	}, "\n")
	if err := a.RunStdio(strings.NewReader(in), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	lines := nonEmptyLines(t, out.String())
	if len(lines) != 1 {
		t.Fatalf("got %d response lines, want exactly 1 (the tools/list answer; the notification is silent): %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"tools"`) {
		t.Fatalf("the single response is not the tools/list answer: %s", lines[0])
	}
}

func nonEmptyLines(t *testing.T, s string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
