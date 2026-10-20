package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

// calculator 最小实现：args {"a":1,"b":1}，返回 a+b。
type calculator struct{}

func (calculator) Name() string { return "calculator" }

func (calculator) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		A, B int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
	}
	return fmt.Sprint(p.A + p.B), nil
}

func main() {
	m := model.NewMock([]model.Response{
		// 第一轮：要求执行 calculator
		{Message: model.Message{
			Role: "assistant",
			ToolCalls: []model.ToolCall{{
				ID:        "call-1",
				Name:      "calculator",
				Arguments: json.RawMessage(`{"a":1,"b":1}`),
			}},
		}},
		// 第二轮：返回最终答案
		{Message: model.Message{Role: "assistant", Content: "计算结果为 2"}},
	})

	a := agent.New(m, map[string]tool.Tool{"calculator": calculator{}}, 5)

	out, err := a.Run(context.Background(), "1+1 等于几？")
	if err != nil {
		panic(err)
	}
	fmt.Println(out)
}
