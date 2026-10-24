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

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishToolCalls FinishReason = "tool_calls"
	FinishLength    FinishReason = "length"
)

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type Response struct {
	Message      Message
	FinishReason FinishReason
	Usage        Usage
}

type ModelError struct {
	StatusCode int
	Code       string
	Retryable  bool
	Err        error
}

func (e *ModelError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code
}

func (e *ModelError) Unwrap() error {
	return e.Err
}

type Model interface {
	Generate(ctx context.Context, messages []Message) (*Response, error)
}

type ToolDefinition struct {
	Name string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

type ToolBinder interface {
	WithTools(defs []ToolDefinition) (Model, error)
}
