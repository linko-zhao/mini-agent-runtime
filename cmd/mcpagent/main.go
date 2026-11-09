package main

import (
	"context"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool/knowledge"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool/mcptool"
)

func main() {
	
}

func run() error {
	ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
	store := knowledge.NewFakeStore([]knowledge.Document{{
		ID: "doc-1",
		Summary: "MCP适配",
		Content: "复用原有工具契约完成远端读取",
	}})
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
		if err := registry.Register(ctx, remote); err != nil {
			return err
		}
	}
	tools := registry.Tools()
	for name, target := range tools {
		described, ok := target.(tool.DescribedTool)
		if !ok {
			return err
		}
		wrapped, err := tool.WithPolicy(described, tool.Policy{Timeout: time.Second, MaxResultBytes: 4096})
		if err != nil {
			return err
		}
		tools[name] = wrapped
	}
}
