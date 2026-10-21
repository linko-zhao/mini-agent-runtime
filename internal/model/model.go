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
	FinishToolCalls FinishReason = "too_calls"
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

type Model interface {
	Generate(ctx context.Context, messages []Message) (*Response, error)
}
