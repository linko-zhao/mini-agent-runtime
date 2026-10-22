package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

func newTestModel(t *testing.T, handler http.HandlerFunc) *ChatModel {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	m, err := NewChatModel(&ChatModelConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	return m
}

func TestGenerateTextResponse(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "chatcmpl-1",
			"object": "chat.completion",
			"created": 1,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "hello world"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	})

	resp, err := m.Generate(context.Background(), []model.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Errorf("Content = %q, want %q", resp.Message.Content, "hello world")
	}
	if resp.FinishReason != model.FinishStop {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, model.FinishStop)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v, want {10 5}", resp.Usage)
	}
}

func TestGenerateToolCalls(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [
						{"id": "call_1", "type": "function",
						 "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\",\"unit\":\"c\"}"}},
						{"id": "call_2", "type": "function",
						 "function": {"name": "get_time", "arguments": "{\"tz\":\"Asia/Shanghai\"}"}}
					]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	})

	resp, err := m.Generate(context.Background(), []model.Message{{Role: "user", Content: "weather?"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.Message.ToolCalls) != 2 {
		t.Fatalf("ToolCalls len = %d, want 2", len(resp.Message.ToolCalls))
	}

	// 顺序与字段无损保留
	first, second := resp.Message.ToolCalls[0], resp.Message.ToolCalls[1]
	if first.ID != "call_1" || first.Name != "get_weather" {
		t.Errorf("first ToolCall = %+v", first)
	}
	if string(first.Arguments) != `{"city":"Beijing","unit":"c"}` {
		t.Errorf("first Arguments = %s", first.Arguments)
	}
	if second.ID != "call_2" || second.Name != "get_time" {
		t.Errorf("second ToolCall = %+v", second)
	}
	if string(second.Arguments) != `{"tz":"Asia/Shanghai"}` {
		t.Errorf("second Arguments = %s", second.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
}

func TestGenerateHTTPErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		wantCode  string
		wantRetry bool
	}{
		{http.StatusUnauthorized, "unauthorized", false},
		{http.StatusTooManyRequests, "rate_limited", true},
		{http.StatusServiceUnavailable, "server_error", true},
	}

	for _, tc := range cases {
		m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"error": "boom"}`))
		})

		_, err := m.Generate(context.Background(), []model.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		var merr *model.ModelError
		if !errors.As(err, &merr) {
			t.Fatalf("status %d: errors.As failed for %T", tc.status, err)
		}
		if merr.StatusCode != tc.status {
			t.Errorf("status %d: StatusCode = %d", tc.status, merr.StatusCode)
		}
		if merr.Code != tc.wantCode {
			t.Errorf("status %d: Code = %q, want %q", tc.status, merr.Code, tc.wantCode)
		}
		if merr.Retryable != tc.wantRetry {
			t.Errorf("status %d: Retryable = %v, want %v", tc.status, merr.Retryable, tc.wantRetry)
		}
	}
}

func TestGenerateEmptyChoices(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id": "x", "choices": []}`))
	})

	resp, err := m.Generate(context.Background(), []model.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatalf("expected error for empty choices, got response %+v", resp)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

func TestGenerateInvalidJSON(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})

	resp, err := m.Generate(context.Background(), []model.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatalf("expected decode error, got response %+v", resp)
	}
}

func TestGenerateContextCanceled(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Generate(ctx, []model.Message{{Role: "user", Content: "hi"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(context.Canceled) = false, err = %v", err)
	}
	var merr *model.ModelError
	if errors.As(err, &merr) && merr.Retryable {
		t.Errorf("canceled request should not be retryable")
	}
}

func TestGenerateContextDeadlineExceeded(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := m.Generate(ctx, []model.Message{{Role: "user", Content: "hi"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(context.DeadlineExceeded) = false, err = %v", err)
	}
}

func TestNewChatModelMissingConfig(t *testing.T) {
	if _, err := NewChatModel(&ChatModelConfig{}); err == nil {
		t.Fatal("expected error for empty config")
	}
}
