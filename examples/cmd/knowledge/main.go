package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
	"log"
"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx := context.Background()
	store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Summary: "Runtime 工具契约", Content: "工具描述供模型选择，调用由 Runtime 执行。"}})
	registry := tool.NewRegistry()
	for _, t := range []tool.Invokable{&knowledge.SearchKnowledge{Store: store}, &knowledge.GetDocument{Store: store}} {
		if err := registry.Register(ctx, t); err != nil {
			return err
		}
	}
	specs, err := registry.Specs([]string{"search_knowledge", "get_document"})
	if err != nil {
		return err
	}
	fmt.Println("本轮模型可见工具：", len(specs))
	m := model.NewMock([]model.Response{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "search-1", Name: "search_knowledge", Arguments: json.RawMessage(`{"query":"Runtime"}`)}}}},
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "read-1", Name: "get_document", Arguments: json.RawMessage(`{"id":"doc-1"}`)}}}},
		{Message: model.Message{Role: "assistant", Content: "工具描述供模型选择，调用由 Runtime 执行。"}},
	})
	tools := registry.Tools()
for name, t := range tools {
 described, ok := t.(tool.DescribedTool)
 if !ok { return fmt.Errorf("tool %s is not described", name) }
 wrapped, err := tool.WithPolicy(described, tool.Policy{Timeout: time.Second, MaxResultBytes:4096})
 if err != nil { return err }; tools[name] = wrapped
}
a, err := agent.New(m, tools, 4, agent.WithVisibleTools("search_knowledge", "get_document"))
	if err != nil {
		return err
	}
	output, err := a.Run(ctx, "查找并阅读 Runtime 工具契约")
	if err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}
