package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
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

		r, err := New(m, nil, 5).Run(context.Background(), "Hi")
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

		result, err := New(m, map[string]tool.Tool{"lookup": lookup}, 5).Run(context.Background(), "find")
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

		_, err := New(m, nil, 5).Run(context.Background(), "find")
		require.ErrorContains(t, err, `tool "missing" not found`)
	})

	t.Run("超过 maxSteps 时返回上限错误", func(t *testing.T) {
		m := model.NewMock([]model.Response{{Message: model.Message{
			Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call", Name: "lookup"}},
		}}})
		lookup := toolFunc(func(context.Context, json.RawMessage) (string, error) { return "result", nil })

		_, err := New(m, map[string]tool.Tool{"lookup": lookup}, 1).Run(context.Background(), "find")
		require.ErrorContains(t, err, "arrive max steps")
	})

	t.Run("context.Cancel 后及时退出", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := modelFunc(func(ctx context.Context, _ []model.Message) (*model.Response, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		done := make(chan error, 1)
		go func() {
			_, err := New(m, nil, 5).Run(ctx, "find")
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

		_, err := New(m, nil, 5).Run(context.Background(), "find")
		require.ErrorIs(t, err, wantErr)
	})
}
