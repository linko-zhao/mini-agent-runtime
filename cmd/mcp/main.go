package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/linko-zhao/mini-agent-runtime/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool/knowledge"
)


func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()
	store := knowledge.NewFakeStore([]knowledge.Document{{
		ID: "doc-1", 
		Summary: "MCP协议", 
		Content: "发现能力不等于执行权限",
	}})
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
	fmt.Println("Protocol Version:", initialized.ProtocolVersion)
	page, err := client.ListTools(ctx, "")
	if err != nil {
		return err
	}
	fmt.Println("Tools Num:", len(page.Tools))
	result, err := client.CallTool(ctx, "get_docment", json.RawMessage(`{"id":"doc-1"}`))
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("demo: tool execution failed")
	}
	for _, raw := range result.Content {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return err
		}
		if block.Type == "text" {
			fmt.Println("Tool Result:", block.Text)
		}
	}
	return nil
}
