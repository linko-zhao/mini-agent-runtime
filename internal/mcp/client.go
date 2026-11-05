package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	return &Client{transport: t}, nil
}

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
		return nil, c.protocol("malformed JSON")
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
	c.state = "failed"
	raw, err := c.rpc(ctx, "initialize", struct {
		ProtocalVersion string         `json:"protocalVersion"`
		Capabilities    struct{}       `json:"capabilities"`
		ClientInfo      Implementation `json:"clientInfo"`
	}{ProtocalVersion: ProtocolVersion, ClientInfo: Implementation{Name: "agent-runtime-examples", Version: "0.1.0"}})
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
