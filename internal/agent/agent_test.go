package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/model/openai"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelFunc func(context.Context, []model.Message) (*model.Response, error)

func (f modelFunc) Generate(ctx context.Context, messages []model.Message) (*model.Response, error) {
	return f(ctx, messages)
}

type toolFunc func(context.Context, json.RawMessage) (string, error)

func (f toolFunc) Name() string { return "lookup" }

func (f toolFunc) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return f(ctx, args)
}

func TestAgentRun(t *testing.T) {
	t.Run("模型直接返回文本", func(t *testing.T) {
		m := model.NewMock([]model.Response{
			{Message: model.Message{Role: "assistant", Content: "Hi, I'm an AI assistant"}},
		})

		a, err := New(m, nil, 5)
		require.NoError(t, err)
		r, err := a.Run(context.Background(), "Hi")
		require.NoError(t, err)
		assert.Equal(t, "Hi, I'm an AI assistant", r)
	})

	t.Run("模型请求工具后继续推理，工具返回值与 ToolCall ID 正确写入消息历史", func(t *testing.T) {
		var secondRequest []model.Message
		calls := 0
		m := modelFunc(func(_ context.Context, messages []model.Message) (*model.Response, error) {
			calls++
			if calls == 1 {
				return &model.Response{Message: model.Message{
					Role: "assistant",
					ToolCalls: []model.ToolCall{{
						ID: "call-123", Name: "lookup", Arguments: json.RawMessage(`{"key":"value"}`),
					}},
				}}, nil
			}
			secondRequest = append([]model.Message(nil), messages...)
			return &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}, nil
		})
		lookup := toolFunc(func(_ context.Context, args json.RawMessage) (string, error) {
			assert.JSONEq(t, `{"key":"value"}`, string(args))
			return "tool result", nil
		})

		a, err := New(m, map[string]tool.Tool{"lookup": lookup}, 5)
		require.NoError(t, err)
		result, err := a.Run(context.Background(), "find")
		require.NoError(t, err)
		assert.Equal(t, "done", result)
		require.Len(t, secondRequest, 3)
		assert.Equal(t, model.Message{Role: "user", Content: "find"}, secondRequest[0])
		assert.Equal(t, "call-123", secondRequest[1].ToolCalls[0].ID)
		assert.Equal(t, model.Message{Role: "tool", Content: "tool result", ToolCallID: "call-123"}, secondRequest[2])
	})

	t.Run("工具不存在时返回错误", func(t *testing.T) {
		m := model.NewMock([]model.Response{{Message: model.Message{
			Role:      "assistant",
			ToolCalls: []model.ToolCall{{ID: "missing-id", Name: "missing"}},
		}}})

		a, err := New(m, nil, 5)
		require.NoError(t, err)
		_, err = a.Run(context.Background(), "find")
		require.ErrorContains(t, err, `tool "missing" not found`)
	})

	t.Run("超过 maxSteps 时返回上限错误", func(t *testing.T) {
		m := model.NewMock([]model.Response{{Message: model.Message{
			Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call", Name: "lookup"}},
		}}})
		lookup := toolFunc(func(context.Context, json.RawMessage) (string, error) { return "result", nil })

		a, err := New(m, map[string]tool.Tool{"lookup": lookup}, 1)
		_, err = a.Run(context.Background(), "find")
		require.ErrorContains(t, err, "arrive max steps")
	})

	t.Run("context.Cancel 后及时退出", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := modelFunc(func(ctx context.Context, _ []model.Message) (*model.Response, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		done := make(chan error, 1)
		a, err := New(m, nil, 5)
		require.NoError(t, err)
		go func() {
			_, err := a.Run(ctx, "find")
			done <- err
		}()
		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(time.Second):
			t.Fatal("Run did not return after context cancellation")
		}
	})

	t.Run("模型调用失败时错误向上传递", func(t *testing.T) {
		wantErr := errors.New("model unavailable")
		m := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
			return nil, wantErr
		})

		a, err := New(m, nil, 5)
		_, err = a.Run(context.Background(), "find")
		require.ErrorIs(t, err, wantErr)
	})
}

type describedTool struct {
	def model.ToolDefinition
	run func(context.Context, json.RawMessage) (string, error)
}

func (d describedTool) Name() string { return d.def.Name }

func (d describedTool) Definition() model.ToolDefinition { return d.def }

func (d describedTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return d.run(ctx, args)
}

type capturedRequest struct {
	Tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"messages"`
}

func newHTTPModel(t *testing.T, handler http.HandlerFunc) model.Model {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	m, err := openai.NewChatModel(&openai.ChatModelConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: srv.URL,
	})
	require.NoError(t, err)
	return m
}

func TestAgentRunWithHTTPProvider(t *testing.T) {
	var requests []capturedRequest
	m := newHTTPModel(t, func(w http.ResponseWriter, r *http.Request) {
		var req capturedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"calculator","arguments":"{\"a\":1,\"b\":1}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"结果是 2"},"finish_reason":"stop"}]}`))
	})

	calc := describedTool{
		def: (&tool.Calculator{}).Definition(),
		run: (&tool.Calculator{}).Execute,
	}
	a, err := New(m, map[string]tool.Tool{"calculator": calc}, 5)
	require.NoError(t, err)
	result, err := a.Run(context.Background(), "1+1 等于几？")
	require.NoError(t, err)
	assert.Equal(t, "结果是 2", result)

	require.Len(t, requests, 2)

	// 首次请求携带工具定义，parameters 为 JSON 对象
	require.Len(t, requests[0].Tools, 1)
	assert.Equal(t, "function", requests[0].Tools[0].Type)
	assert.Equal(t, "calculator", requests[0].Tools[0].Function.Name)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(requests[0].Tools[0].Function.Parameters, &schema))
	assert.Equal(t, "object", schema["type"])

	// 第二轮请求回填 assistant tool_calls 与 role=tool 消息
	msgs := requests[1].Messages
	require.Len(t, msgs, 3)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Equal(t, "call-1", msgs[1].ToolCalls[0].ID)
	assert.Equal(t, "calculator", msgs[1].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"a":1,"b":1}`, msgs[1].ToolCalls[0].Function.Arguments)
	assert.Equal(t, "tool", msgs[2].Role)
	assert.Equal(t, "call-1", msgs[2].ToolCallID)
	assert.Equal(t, "2", msgs[2].Content)
}

func TestAgentRunInvalidToolDefinition(t *testing.T) {
	requestCount := 0
	m := newHTTPModel(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
	})

	dup := model.ToolDefinition{Name: "calculator", Parameters: json.RawMessage(`{"type":"object"}`)}
	badSchema := model.ToolDefinition{Name: "broken", Parameters: json.RawMessage(`{"type":"array"}`)}

	t.Run("重复工具名称", func(t *testing.T) {
		tools := map[string]tool.Tool{
			"a": describedTool{def: dup, run: func(context.Context, json.RawMessage) (string, error) { return "", nil }},
			"b": describedTool{def: dup, run: func(context.Context, json.RawMessage) (string, error) { return "", nil }},
		}
		_, err := New(m, tools, 5)
		require.ErrorContains(t, err, "duplicate tool name")
	})

	t.Run("非法 Schema", func(t *testing.T) {
		tools := map[string]tool.Tool{
			"broken": describedTool{def: badSchema, run: func(context.Context, json.RawMessage) (string, error) { return "", nil }},
		}
		_, err := New(m, tools, 5)
		require.ErrorContains(t, err, "object")
	})

	assert.Equal(t, 0, requestCount, "初始化失败时不应向 Provider 发出 HTTP 请求")
}
