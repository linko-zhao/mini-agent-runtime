package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

type DemoServer struct {
	mu sync.Mutex
	state string
	spec tool.Spec
	target tool.Invokable
}

func NewDemoServer(ctx context.Context, target tool.Invokable) (*DemoServer, error) {
	if target == nil {
		return nil, fmt.Errorf("mcp demo: nil target")
	}
	spec, err := target.Meta(ctx)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if spec.Name == "" || json.Unmarshal(spec.InputSchema, &schema) != nil || schema["type"] != "object" {
		return nil, fmt.Errorf("mcp demo: invalid descriptor")
	}
	spec.InputSchema = append(json.RawMessage(nil), spec.InputSchema...)
	return &DemoServer{state: "new", spec: spec, target: target}, nil
}

func reply(id uint64, result any, rpcErr *RPCError) ([]byte, error) {
	r := response{JSONRPC: "2.0", ID: json.RawMessage(strconv.FormatUint(id, 10))}
	var err error
	if rpcErr != nil {
		r.Error, err = json.Marshal(rpcErr)
	} else {
		r.Result, err = json.Marshal(result)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(r)
}

func textBlock(s string) json.RawMessage {
	raw, _ := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{"text", s})
	return raw
}

func (s *DemoServer) Send(ctx context.Context, payload []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var r request
	if err := json.Unmarshal(payload, &r); err != nil {
		return nil, err
	}
	if r.JSONRPC != "2.0" {
		return nil, fmt.Errorf("mcp demo: invalid envelope")
	}
	if r.Method == "notifications/initialized" {
		if r.ID != 0 || s.state != "initialized" {
			return nil, fmt.Errorf("mcp demo: unexpected initialized notification")
		}
		s.state = "ready"
		return nil, nil
	}
	if r.ID == 0 {
		return nil, fmt.Errorf("mcp demo: request ID required")
	}
	if r.Method == "initialize" {
		if s.state != "new" {
			return reply(r.ID, nil, &RPCError{Code: -32600, Message: "already initialized"})
		}
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(r.Params, &p) != nil || p.ProtocolVersion == "" {
			return reply(r.ID, nil, &RPCError{Code: -32602, Message: "protocolVersion required"})
		}
		s.state = "initialized"
		return reply(r.ID, InitializeResult{ProtocolVersion: ProtocolVersion, Capabilities: &ServerCapabilities{Tools: &ToolsCapability{}}, ServerInfo: Implementation{Name: "offline-knowledge", Version: "0.1.0"}}, nil)
	}
	if s.state != "ready" {
		return reply(r.ID, nil, &RPCError{Code: -32600, Message: "not initialized"})
	}
	switch r.Method {
	case "tools/list":
		// One page only; this fixture's server declares no pagination.
		return reply(r.ID, ToolsPage{Tools: []Descriptor{{Name: s.spec.Name, Description: s.spec.Description, InputSchema: s.spec.InputSchema}}}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(r.Params, &p) != nil || p.Name != s.spec.Name {
			return reply(r.ID, nil, &RPCError{Code: -32602, Message: "unknown tool or invalid parameters"})
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		output, err := s.target.Invoke(ctx, p.Arguments)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			// Controlled generic result; do not expose internal backend error text.
			return reply(r.ID, ToolResult{Content: []json.RawMessage{textBlock("document tool failed")}, IsError: true}, nil)
		}
		return reply(r.ID, ToolResult{Content: []json.RawMessage{textBlock(output)}}, nil)
	default:
		return reply(r.ID, nil, &RPCError{Code: -32601, Message: "method not found"})
	}
}
