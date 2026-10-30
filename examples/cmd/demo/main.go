package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/calculator"
	"log"
)

func main() {
	m := model.NewMock([]model.Response{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "calculator", Arguments: json.RawMessage(`{"a":1,"b":1}`)}}}, FinishReason: model.FinishToolCalls},
		{Message: model.Message{Role: "assistant", Content: "计算结果为 2"}, FinishReason: model.FinishStop},
	})
	a, err := agent.New(m, map[string]tool.Tool{"calculator": &calculator.Calculator{}}, 4)
	if err != nil {
		log.Fatal(err)
	}
	out, err := a.Run(context.Background(), "1 + 1 等于多少？")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
}
