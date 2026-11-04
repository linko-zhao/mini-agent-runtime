package mcp

import (
	"context"
	"errors"
)

var (
	ErrSessionState = errors.New("mcp: invalid session state")
	ErrToolsUnavailable = errors.New("mcp: server did not advertise tools")
	ErrProtocol = errors.New("mcp: invalid protocol response")
)

type Transport interface {
	Send(context.Context, []byte) ([]byte, error)
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type Implementation struct {
	Name string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities	*ServerCapabilities `json:"capabilities"`
	ServerInfo		Implementation `json:"serverInfo"`
}
