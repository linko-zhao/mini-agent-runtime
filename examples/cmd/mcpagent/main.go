package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/mcptool"
)

type observedModel struct {
	base    model.Model
	history []model.Message
}

func (m *observedModel) Generate(ctx context.Context, messages []model.Message) (*model.Response, error) {
	m.history = append([]model.Message(nil), messages...)
	return m.base.Generate(ctx, messages)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	// Demonstration identity. Production hosts derive this from authentication.
	ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
	store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Summary: "MCP适配", Content: "复用原有工具契约完成远端读取。"}})
	server, err := mcp.NewDemoServer(ctx, &knowledge.GetDocument{Store: store})
	if err != nil {
		return err
	}
	client, err := mcp.NewClient(server)
	if err != nil {
		return err
	}
	if _, err = client.Initialize(ctx); err != nil {
		return err
	}
	remotes, err := mcptool.Discover(ctx, client, "knowledge", 4)
	if err != nil {
		return err
	}
	registry := tool.NewRegistry()
	for _, remote := range remotes {
		if err = registry.Register(ctx, remote); err != nil {
			return err
		}
	}
	tools := registry.Tools()
	for name, target := range tools {
		described, ok := target.(tool.DescribedTool)
		if !ok {
			return fmt.Errorf("tool is not described")
		}
		wrapped, err := tool.WithPolicy(described, tool.Policy{Timeout: time.Second, MaxResultBytes: 4096})
		if err != nil {
			return err
		}
		tools[name] = wrapped
	}
	authorizer, err := tool.NewCapabilityAuthorizer(
		map[string]tool.Capability{"knowledge__get_document": tool.ReadCapability},
		map[string][]tool.Capability{"reader": {tool.ReadCapability}})
	if err != nil {
		return err
	}
	scripted := model.NewMock([]model.Response{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "model-read-1", Name: "knowledge__get_document", Arguments: json.RawMessage(`{"id":"doc-1"}`)}}}},
		{Message: model.Message{Role: "assistant", Content: "MCP工具往返完成"}},
	})
	observer := &observedModel{base: scripted}
	runtime, err := agent.New(observer, tools, 3, agent.WithVisibleTools("knowledge__get_document"), agent.WithAuthorizer(authorizer))
	if err != nil {
		return err
	}
	output, err := runtime.Run(ctx, "读取远端知识文档")
	if err != nil {
		return err
	}
	if len(observer.history) != 3 || observer.history[2].Role != "tool" || observer.history[2].ToolCallID != "model-read-1" {
		return fmt.Errorf("unexpected tool history")
	}
	var result mcptool.ResultEnvelope
	if err = json.Unmarshal([]byte(observer.history[2].Content), &result); err != nil {
		return err
	}
	if len(result.Text) != 1 {
		return fmt.Errorf("missing document text")
	}
	fmt.Println("本地工具: knowledge__get_document")
	fmt.Println("工具消息CallID:", observer.history[2].ToolCallID)
	fmt.Println("远端正文:", result.Text[0])
	fmt.Println("Agent结果:", output)
	return nil
}
