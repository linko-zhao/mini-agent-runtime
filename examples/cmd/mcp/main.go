package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx := context.Background()
	store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Summary: "MCP协议", Content: "发现能力不等于获得执行权限。"}})
	server, err := mcp.NewDemoServer(ctx, &knowledge.GetDocument{Store: store})
	if err != nil {
		return err
	}
	client, err := mcp.NewClient(server)
	if err != nil {
		return err
	}
	initialized, err := client.Initialize(ctx)
	if err != nil {
		return err
	}
	fmt.Println("协议版本:", initialized.ProtocolVersion)
	page, err := client.ListTools(ctx, "")
	if err != nil {
		return err
	}
	fmt.Println("工具数量:", len(page.Tools))
	result, err := client.CallTool(ctx, "get_document", json.RawMessage(`{"id":"doc-1"}`))
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("demo: tool execution failed")
	}
	// This demo knows its fixture returns text. The general client preserves all blocks.
	for _, raw := range result.Content {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return err
		}
		if block.Type == "text" {
			fmt.Println("工具结果:", block.Text)
		}
	}
	return nil
}
