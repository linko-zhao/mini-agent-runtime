package openai

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

const maxCompressed = 1 * 1024 * 1024    // 1 MiB 压缩后
const maxDecompressed = 10 * 1024 * 1024 // 10 MiB 解压后

func convertRequest(_ context.Context, cm *ChatModel, messages []model.Message) (*ChatCompletionNewParams, error) {
	p := ChatCompletionNewParams{
		Model: cm.config.Model,
		Tools: cm.tools,
	}

	for _, msg := range messages {
		ct := ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		for _, t := range msg.ToolCalls {
			ct.ToolCalls = append(ct.ToolCalls, ToolCall{
				ID:   t.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      t.Name,
					Arguments: string(t.Arguments),
				},
			})
		}
		p.Messages = append(p.Messages, ct)
	}
	return &p, nil
}

func convertResponse(_ context.Context, resp *http.Response) (*model.Response, error) {
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxCompressed))
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(gz, maxDecompressed))
	if err != nil {
		return nil, err
	}
	var completion ChatCompletion
	if err := json.Unmarshal(body, &completion); err != nil {
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
