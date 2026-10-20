package model

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Response struct{ Message Message }

type Model interface {
	Generate(ctx context.Context, messages []Message) (*Response, error)
}
