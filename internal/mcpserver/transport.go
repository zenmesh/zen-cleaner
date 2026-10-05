// Copyright 2026 Zen Mesh. All rights reserved.

// The stdio transport (R051 N3 continuation): the JSON-RPC lines over
// the process stdio — the initialize handshake, tools/list, and the
// tools/call for the read-only family. DEFAULT-DENY: without the mode
// env the transport refuses every call with the typed denial (the
// tools neither enumerate nor answer).
package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// rpcRequest is one JSON-RPC 2.0 request.
type rpcRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is one JSON-RPC 2.0 response.
type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RunStdio serves the JSON-RPC transport on the process stdio (blocks
// until the input closes). The calls are typed: initialize (the
// handshake), tools/list (the read-only family or the typed denial),
// tools/call (the read-only tools only).
func (a *Adapter) RunStdio(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue // not JSON: skip the line (the transport's tolerance)
		}
		switch req.Method {
		case "initialize":
			res := map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]interface{}{"tools": map[string]bool{"listChanged": false}},
				"serverInfo":      map[string]string{"name": "zen-cleaner", "version": "0.1.0"},
			}
			b, _ := json.Marshal(res)
			_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(b)})
		case "tools/list":
			tools := a.Tools()
			if tools == nil {
				_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID,
					"error": rpcError{Code: -32601, Message: "not enabled (default-deny)"}})
				continue
			}
			b, _ := json.Marshal(map[string]interface{}{"tools": tools})
			_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(b)})
		case "tools/call":
			if !a.enabled {
				_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID,
					"error": rpcError{Code: -32601, Message: ErrNotEnabled.Error()}})
				continue
			}
			var call struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(req.Params, &call)
			var res string
			switch call.Name {
			case "entitlement_status", "health_summary":
				res = "{}" // the read-only summaries' live bodies ride the next slice
			default:
				res = "{}"
			}
			b, _ := json.Marshal(map[string]interface{}{"content": []map[string]string{{"type": "text", "text": res}}})
			_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(b)})
		default:
			_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID,
				"error": rpcError{Code: -32601, Message: fmt.Sprintf("unknown method %q", req.Method)}})
		}
	}
	return scanner.Err()
}

var _ = fmt.Sprintf
