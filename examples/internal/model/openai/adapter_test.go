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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProviderMapping(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body requestEnvelope
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tools) != 1 || body.Messages[0].CallID != "id-1" {
			t.Fatalf("request mapping: %+v", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"id-2","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`))}, nil
	})}
	a, err := New(Config{APIKey: "test", Model: "demo", BaseURL: "https://example.invalid", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := a.WithTools([]model.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.definitions) != 0 {
		t.Fatal("binding changed original adapter")
	}
	got, err := bound.Generate(context.Background(), []model.Message{{Role: "tool", ToolCallID: "id-1", Content: "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.ToolCalls[0].ID != "id-2" || got.FinishReason != model.FinishToolCalls || got.Usage.InputTokens != 2 {
		t.Fatalf("response mapping: %+v", got)
	}
}
func TestProviderErrorsAndMalformedCalls(t *testing.T) {
	for _, status := range []int{401, 429, 503} {
		err := httpError(status)
		var typed *model.ModelError
		if !errors.As(err, &typed) || typed.StatusCode != status || typed.Retryable != (status == 429 || status == 503) {
			t.Fatal(err)
		}
	}
	if _, err := decodeResponse(strings.NewReader(`{"choices":[]}`)); err == nil {
		t.Fatal("accepted empty choices")
	}
	if _, err := decodeResponse(strings.NewReader(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"x","function":{"name":"f","arguments":"not-json"}}]}}]}`)); err == nil {
		t.Fatal("accepted malformed arguments")
	}
}
