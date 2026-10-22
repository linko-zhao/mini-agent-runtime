package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

// 对应 API 返回的完整 JSON 响应
type ChatCompletion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"` // 使用指针，因为某些情况下可能无此字段
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

type Message struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content,omitempty"` // 使用指针，当发生工具调用时 Content 可能为 null
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`   // 回传结果时要用
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串，需要二次解析
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func convertResponse(_ context.Context, resp *http.Response) (*model.Response, error) {
	var completion ChatCompletion
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("openai: decode completions response error: %v", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("openai: response has no choices")
	}

	choice := completion.Choices[0]
	msg := model.Message{Role: choice.Message.Role}
	if choice.Message.Content != nil {
		msg.Content = *choice.Message.Content
	}
	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, model.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	out := &model.Response{Message: msg}
	if choice.FinishReason != nil {
		out.FinishReason = model.FinishReason(*choice.FinishReason)
	}
	if completion.Usage != nil {
		out.Usage = model.Usage{
			InputTokens:  completion.Usage.PromptTokens,
			OutputTokens: completion.Usage.CompletionTokens,
		}
	}
	return out, nil
}
