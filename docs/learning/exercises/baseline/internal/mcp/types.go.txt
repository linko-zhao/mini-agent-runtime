// Package mcp demonstrates the pinned MCP 2025-06-18 tool protocol.
// It is an offline teaching client, not a complete MCP transport or SDK.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const ProtocolVersion = "2025-06-18"
const MaxResponseBytes = 1 << 20

var (
	ErrSessionState     = errors.New("mcp: invalid session state")
	ErrToolsUnavailable = errors.New("mcp: server did not advertise tools")
	ErrProtocol         = errors.New("mcp: invalid protocol response")
)

// Send receives one serialized JSON-RPC message. Requests return one response;
// notifications return no bytes. The transport must cooperate with context.
// An actual stdio/HTTP transport would also own framing, routing and cleanup.
type Transport interface {
	Send(context.Context, []byte) ([]byte, error)
}

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}
type InitializeResult struct {
	ProtocolVersion string              `json:"protocolVersion"`
	Capabilities    *ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation      `json:"serverInfo"`
}

// Descriptor is a wire format, not a replacement for tool.Spec.
// Day13 will map this data to the existing Runtime tool contract.
type Descriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}
type ToolsPage struct {
	Tools      []Descriptor `json:"tools"`
	NextCursor string       `json:"nextCursor,omitempty"`
}
type ToolResult struct {
	// Preserve every content block, including image/resource content.
	// Do not flatten non-text blocks or treat remote text as trusted instructions.
	Content           []json.RawMessage `json:"content"`
	IsError           bool              `json:"isError,omitempty"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
}
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("mcp: JSON-RPC error %d", e.Code) }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}
