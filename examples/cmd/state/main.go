package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
	registry := tool.NewRegistry()
	store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Content: "Run status and call outcome are separate."}})
	if err := registry.Register(ctx, &knowledge.GetDocument{Store: store}); err != nil {
		return err
	}
	auth, err := tool.NewCapabilityAuthorizer(map[string]tool.Capability{"get_document": tool.ReadCapability}, map[string][]tool.Capability{"reader": {tool.ReadCapability}})
	if err != nil {
		return err
	}
	for _, id := range []string{"doc-1", "missing"} {
		args, _ := json.Marshal(map[string]string{"id": id})
		m := model.NewMock([]model.Response{
			{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "read-1", Name: "get_document", Arguments: args}}}},
			{Message: model.Message{Role: "assistant", Content: "scripted final"}},
		})
		a, err := agent.New(m, registry.Tools(), 3, agent.WithAuthorizer(auth))
		if err != nil {
			return err
		}
		result, err := a.RunWithState(ctx, "read "+id)
		if err != nil {
			return err
		}
		if len(result.Snapshot.Calls) != 1 {
			return fmt.Errorf("unexpected call count")
		}
		fmt.Printf("%s: run=%s call=%s messages=%d\n", id, result.Snapshot.Status, result.Snapshot.Calls[0].Status, len(result.Snapshot.Messages))
	}
	return nil
}
