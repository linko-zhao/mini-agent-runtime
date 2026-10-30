package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"io"
	"net/http"
	"strings"
)

const responseLimit = 4 << 20

// 独立 wire 结构只描述 Provider 协议；Runtime 类型不会被 JSON 直接解码。
type requestEnvelope struct {
	Model    string           `json:"model"`
	Messages []requestMessage `json:"messages"`
	Tools    []requestTool    `json:"tools,omitempty"`
}
type requestMessage struct {
	Role    string        `json:"role"`
	Content string        `json:"content,omitempty"`
	Calls   []requestCall `json:"tool_calls,omitempty"`
	CallID  string        `json:"tool_call_id,omitempty"`
}
type requestCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function requestFunction `json:"function"`
}
type requestFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type requestTool struct {
	Type     string               `json:"type"`
	Function model.ToolDefinition `json:"function"`
}

func encodeRequest(name string, messages []model.Message, defs []model.ToolDefinition) ([]byte, error) {
	p := requestEnvelope{Model: name}
	for _, m := range messages {
		w := requestMessage{Role: m.Role, Content: m.Content, CallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			if !json.Valid(c.Arguments) {
				return nil, fmt.Errorf("openai adapter: invalid arguments for call %q", c.ID)
			}
			w.Calls = append(w.Calls, requestCall{ID: c.ID, Type: "function", Function: requestFunction{Name: c.Name, Arguments: string(c.Arguments)}})
		}
		p.Messages = append(p.Messages, w)
	}
	for _, d := range defs {
		p.Tools = append(p.Tools, requestTool{Type: "function", Function: d})
	}
	return json.Marshal(p)
}

type responseEnvelope struct {
	Choices []responseChoice `json:"choices"`
	Usage   *usageWire       `json:"usage"`
}
type responseChoice struct {
	Message responseMessage `json:"message"`
	Reason  *string         `json:"finish_reason"`
}
type responseMessage struct {
	Role    string         `json:"role"`
	Content *string        `json:"content"`
	Calls   []responseCall `json:"tool_calls"`
}
type responseCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function requestFunction `json:"function"`
}
type usageWire struct {
	Input  int `json:"prompt_tokens"`
	Output int `json:"completion_tokens"`
}

func decodeResponse(r io.Reader) (*model.Response, error) {
	raw, err := io.ReadAll(io.LimitReader(r, responseLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > responseLimit {
		return nil, fmt.Errorf("openai adapter: response exceeds limit")
	}
	var wire responseEnvelope
	if err = json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("openai adapter: invalid response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, fmt.Errorf("openai adapter: response contains no choices")
	}
	c := wire.Choices[0]
	if c.Message.Role != "assistant" {
		return nil, fmt.Errorf("openai adapter: expected assistant response")
	}
	out := &model.Response{Message: model.Message{Role: c.Message.Role}}
	if c.Message.Content != nil {
		out.Message.Content = *c.Message.Content
	}
	for _, call := range c.Message.Calls {
		if call.ID == "" || call.Function.Name == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return nil, fmt.Errorf("openai adapter: malformed tool call")
		}
		out.Message.ToolCalls = append(out.Message.ToolCalls, model.ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(strings.TrimSpace(call.Function.Arguments))})
	}
	if c.Reason != nil {
		out.FinishReason = model.FinishReason(*c.Reason)
	}
	if wire.Usage != nil {
		out.Usage = model.Usage{InputTokens: wire.Usage.Input, OutputTokens: wire.Usage.Output}
	}
	return out, nil
}

// statusResponse is handy for protocol-focused tests without a live endpoint.
func statusResponse(status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(nil))}
}
