package openai

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProtocolAndBinding(t *testing.T) {
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var p ChatCompletionNewParams
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if len(p.Tools) != 1 || p.Messages[0].ToolCallID != "call-1" {
			t.Fatalf("bad request %+v", p)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-2","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)), Header: make(http.Header)}, nil
	})}
	m, err := NewChatModel(&ChatModelConfig{APIKey: "test", Model: "test", BaseURL: "https://example.invalid/chat/completions", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := m.WithTools([]model.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.tools) != 0 {
		t.Fatal("mutated base model")
	}
	r, err := bound.Generate(context.Background(), []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.FinishReason != model.FinishToolCalls || r.Usage.InputTokens != 3 || r.Message.ToolCalls[0].ID != "call-2" {
		t.Fatalf("%+v", r)
	}
}
func TestHTTPErrorAndCancellation(t *testing.T) {
	for _, code := range []int{401, 429, 500} {
		m, _ := NewChatModel(&ChatModelConfig{APIKey: "test", Model: "test", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("private provider error"))}, nil
		})}})
		_, err := m.Generate(context.Background(), nil)
		var e *model.ModelError
		if !errors.As(err, &e) || e.StatusCode != code || e.Retryable != (code == 429 || code >= 500) {
			t.Fatal(err)
		}
	}
	if !errors.Is(wrapTransportError(context.Canceled), context.Canceled) {
		t.Fatal("lost cancellation")
	}
}
