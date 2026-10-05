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
	a := New()
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
	a := New()
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
