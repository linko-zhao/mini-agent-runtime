package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Client struct {
	transport      Transport
	gate           chan struct{}
	state          string
	nextID         uint64
	toolsAvailable bool
}

func NewClient(t Transport) (*Client, error) {
	if t == nil {
		return nil, fmt.Errorf("mcp: nil transport")
	}
	return &Client{transport: t, gate: make(chan struct{}, 1), state: "new"}, nil
}

// A session serializes requests. Cancellation while queued sends no message.
func (c *Client) enter(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-c.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *Client) leave() { <-c.gate }
func (c *Client) protocol(message string) error {
	c.state = "failed"
	return fmt.Errorf("%w: %s", ErrProtocol, message)
}

// rpc is called with the session gate held. Transport/protocol failures mark
// the session failed because the peer may already have executed the request.
// A valid JSON-RPC error does not automatically invalidate the session.
func (c *Client) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	c.nextID++
	id := c.nextID
	message, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: raw})
	if err != nil {
		return nil, err
	}
	payload, err := c.transport.Send(ctx, message)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		c.state = "failed"
		return nil, fmt.Errorf("mcp transport: %w", err)
	}
	if len(payload) > MaxResponseBytes {
		return nil, c.protocol("response exceeds byte budget")
	}
	var r response
	if err = json.Unmarshal(payload, &r); err != nil {
		return nil, c.protocol("malformed JSON")
	}
	if r.JSONRPC != "2.0" || string(r.ID) != strconv.FormatUint(id, 10) {
		return nil, c.protocol("version or request ID mismatch")
	}
	if (len(r.Result) == 0) == (len(r.Error) == 0) {
		return nil, c.protocol("expected exactly one of result/error")
	}
	if len(r.Error) > 0 {
		var e RPCError
		if bytes.Equal(bytes.TrimSpace(r.Error), []byte("null")) || json.Unmarshal(r.Error, &e) != nil {
			return nil, c.protocol("invalid RPC error")
		}
		// Required error fields must actually exist, not merely decode as zero values.
		var fields struct {
			Code    *int    `json:"code"`
			Message *string `json:"message"`
		}
		if json.Unmarshal(r.Error, &fields) != nil || fields.Code == nil || fields.Message == nil {
			return nil, c.protocol("missing/invalid RPC error fields")
		}
		return nil, &e
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(r.Result, &object) != nil || object == nil {
		return nil, c.protocol("expected object result")
	}
	return r.Result, nil
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	var result InitializeResult
	if err := c.enter(ctx); err != nil {
		return result, err
	}
	defer c.leave()
	if c.state != "new" {
		return result, ErrSessionState
	}
	// Never replay initialization on an ambiguous, possibly initialized transport.
	c.state = "failed"
	raw, err := c.rpc(ctx, "initialize", struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    struct{}       `json:"capabilities"`
		ClientInfo      Implementation `json:"clientInfo"`
	}{ProtocolVersion: ProtocolVersion, ClientInfo: Implementation{Name: "agent-runtime-examples", Version: "0.1.0"}})
	if err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.ProtocolVersion != ProtocolVersion || result.Capabilities == nil || result.ServerInfo.Name == "" || result.ServerInfo.Version == "" {
		return result, c.protocol("unsupported version or invalid initialize result")
	}
	notification, _ := json.Marshal(request{JSONRPC: "2.0", Method: "notifications/initialized"})
	reply, err := c.transport.Send(ctx, notification)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		return result, fmt.Errorf("mcp initialized notification: %w", err)
	}
	if len(bytes.TrimSpace(reply)) != 0 {
		return result, c.protocol("notification must not receive a response")
	}
	c.toolsAvailable = result.Capabilities.Tools != nil
	c.state = "ready"
	return result, nil
}

func (c *Client) ListTools(ctx context.Context, cursor string) (ToolsPage, error) {
	var page ToolsPage
	if err := c.enter(ctx); err != nil {
		return page, err
	}
	defer c.leave()
	if c.state != "ready" {
		return page, ErrSessionState
	}
	if !c.toolsAvailable {
		return page, ErrToolsUnavailable
	}
	raw, err := c.rpc(ctx, "tools/list", struct {
		Cursor string `json:"cursor,omitempty"`
	}{cursor})
	if err != nil {
		return page, err
	}
	if json.Unmarshal(raw, &page) != nil || page.Tools == nil {
		return page, c.protocol("tools must be an array")
	}
	seen := map[string]bool{}
	for _, d := range page.Tools {
		var schema map[string]any
		if strings.TrimSpace(d.Name) == "" || seen[d.Name] || json.Unmarshal(d.InputSchema, &schema) != nil || schema["type"] != "object" {
			return page, c.protocol("invalid/duplicate tool descriptor")
		}
		seen[d.Name] = true
	}
	return page, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
	var result ToolResult
	if err := c.enter(ctx); err != nil {
		return result, err
	}
	defer c.leave()
	if c.state != "ready" {
		return result, ErrSessionState
	}
	if !c.toolsAvailable {
		return result, ErrToolsUnavailable
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	var object map[string]json.RawMessage
	if strings.TrimSpace(name) == "" || json.Unmarshal(args, &object) != nil || object == nil {
		return result, fmt.Errorf("mcp: nonempty tool name and object arguments required")
	}
	raw, err := c.rpc(ctx, "tools/call", struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}{name, args})
	if err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.Content == nil {
		return result, c.protocol("content must be an array")
	}
	for _, block := range result.Content {
		var header struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(block, &header) != nil || header.Type == "" {
			return result, c.protocol("invalid content block")
		}
	}
	// IsError is a tool execution result, not a JSON-RPC/transport error.
	return result, nil
}
