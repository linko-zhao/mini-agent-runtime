package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool/calculator"
)

// scriptModel replays queued responses; the last response's model sees the
// full accumulated history, which it captures for assertions.
type scriptModel struct {
	t       *testing.T
	respond []model.Message
	calls   [][]model.Message
	finalFn func(msgs []model.Message) model.Message
}

func (s *scriptModel) Generate(_ context.Context, msgs []model.Message) (*model.Response, error) {
	s.calls = append(s.calls, append([]model.Message(nil), msgs...))
	if len(s.respond) > 0 {
		m := s.respond[0]
		s.respond = s.respond[1:]
		return &model.Response{Message: m}, nil
	}
	return &model.Response{Message: s.finalFn(msgs)}, nil
}

func toolCallMsg(id, name, args string) model.Message {
	return model.Message{
		Role:      "assistant",
		ToolCalls: []model.ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(args)}},
	}
}

func runWithScript(t *testing.T, script *scriptModel, tools map[string]tool.Tool) (string, error) {
	t.Helper()
	a, err := New(script, tools, 5)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a.Run(context.Background(), "hi")
}

// 1. 合法工具参数：执行真实操作，工具结果带着 ToolCallID 回到模型历史。
func TestToolCallValidArgs(t *testing.T) {
	script := &scriptModel{
		respond: []model.Message{toolCallMsg("call_1", "calculator", `{"a":1,"b":2}`)},
		finalFn: func(msgs []model.Message) model.Message {
			return model.Message{Role: "assistant", Content: "done"}
		},
	}
	out, err := runWithScript(t, script, map[string]tool.Tool{"calculator": &calculator.Calculator{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "done" {
		t.Fatalf("output = %q", out)
	}
	if len(script.calls) != 2 {
		t.Fatalf("model called %d times", len(script.calls))
	}
	history := script.calls[1]
	last := history[len(history)-1]
	if last.Role != "tool" || last.ToolCallID != "call_1" || last.Content != "3" {
		t.Fatalf("tool message = %+v", last)
	}
}

//  2. 非法 JSON / 未知字段：工具错误作为 Tool Result 返回（run 不失败、不 panic），
//     内容稳定。缺字段当前不校验（零值参与计算），锁定该行为。
func TestToolCallInvalidArgs(t *testing.T) {
	cases := []struct {
		name        string
		args        string
		wantContent string
	}{
		{"invalid json", `{"a":`, "invalid_json: arguments must be a valid JSON object"},
		{"wrong type", `{"a":"x","b":2}`, "invalid_json: arguments must be a valid JSON object"},
		{"unknown field", `{"a":1,"b":2,"c":3}`, "invalid_arguments: arguments has unexpected field"},
		{"missing field", `{"a":1}`, "1"}, // shortcut: no required-field validation today
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := &scriptModel{
				respond: []model.Message{toolCallMsg("call_x", "calculator", tc.args)},
				finalFn: func(msgs []model.Message) model.Message {
					return model.Message{Role: "assistant", Content: "recovered"}
				},
			}
			out, err := runWithScript(t, script, map[string]tool.Tool{"calculator": &calculator.Calculator{}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if out != "recovered" {
				t.Fatalf("output = %q", out)
			}
			last := script.calls[1][len(script.calls[1])-1]
			if last.Content != tc.wantContent {
				t.Fatalf("tool content = %q, want %q", last.Content, tc.wantContent)
			}
			if last.ToolCallID != "call_x" {
				t.Fatalf("ToolCallID = %q", last.ToolCallID)
			}
		})
	}
}

// 3. 未知工具名：Run 报错且包含工具名；历史只追加 assistant 消息，无 tool 消息。
func TestToolCallUnknownTool(t *testing.T) {
	script := &scriptModel{
		respond: []model.Message{toolCallMsg("call_9", "nope", `{}`)},
		finalFn: func(msgs []model.Message) model.Message {
			return model.Message{Role: "assistant", Content: "unreachable"}
		},
	}
	tools := map[string]tool.Tool{"calculator": &calculator.Calculator{}}
	_, err := runWithScript(t, script, tools)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want mention of tool name", err)
	}
	if len(script.calls) != 1 {
		t.Fatalf("model called %d times after failure", len(script.calls))
	}
	for _, m := range script.calls[0] {
		if m.Role == "tool" {
			t.Fatalf("history mutated with tool message: %+v", m)
		}
	}
	if len(tools) != 1 {
		t.Fatalf("registry mutated: %v", tools)
	}
}

//  4. 工具执行错误：当前设计 = Tool Result（错误文本入历史），Run 成功。
//     该测试锁定此行为；若未来改为 Run error 需同步修改。
func TestToolCallExecuteError(t *testing.T) {
	broken := toolFunc(func(context.Context, json.RawMessage) (string, error) {
		return "", errors.New("boom")
	})
	script := &scriptModel{
		respond: []model.Message{toolCallMsg("call_e", "lookup", `{}`)},
		finalFn: func(msgs []model.Message) model.Message {
			return model.Message{Role: "assistant", Content: "handled"}
		},
	}
	out, err := runWithScript(t, script, map[string]tool.Tool{"lookup": broken})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "handled" {
		t.Fatalf("output = %q", out)
	}
	last := script.calls[1][len(script.calls[1])-1]
	if last.Role != "tool" || last.Content != "boom" || last.ToolCallID != "call_e" {
		t.Fatalf("tool message = %+v", last)
	}
}

// 5. 多工具调用：每个 Tool Result 保留对应 ToolCallID，按序配对。
func TestToolCallIDPairing(t *testing.T) {
	script := &scriptModel{
		respond: []model.Message{{
			Role: "assistant",
			ToolCalls: []model.ToolCall{
				{ID: "call_a", Name: "calculator", Arguments: json.RawMessage(`{"a":1,"b":1}`)},
				{ID: "call_b", Name: "calculator", Arguments: json.RawMessage(`{"a":2,"b":2}`)},
			},
		}},
		finalFn: func(msgs []model.Message) model.Message {
			return model.Message{Role: "assistant", Content: "paired"}
		},
	}
	_, err := runWithScript(t, script, map[string]tool.Tool{"calculator": &calculator.Calculator{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	history := script.calls[1]
	toolMsgs := history[len(history)-2:]
	want := []struct{ id, content string }{{"call_a", "2"}, {"call_b", "4"}}
	for i, w := range want {
		if toolMsgs[i].Role != "tool" || toolMsgs[i].ToolCallID != w.id || toolMsgs[i].Content != w.content {
			t.Fatalf("toolMsgs[%d] = %+v, want id=%s content=%s", i, toolMsgs[i], w.id, w.content)
		}
	}
}
