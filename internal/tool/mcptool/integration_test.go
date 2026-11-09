package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool/knowledge"
)

type recordingTransport struct {
	base  mcp.Transport
	calls int
	last  []byte
}

func (r *recordingTransport) Send(ctx context.Context, b []byte) ([]byte, error) {
	var req struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(b, &req)
	if req.Method == "tools/call" {
		r.calls++
		r.last = append([]byte(nil), b...)
	}
	return r.base.Send(ctx, b)
}

type scriptedModel struct {
	calls   int
	defs    []model.ToolDefinition
	history []model.Message
}

func (m *scriptedModel) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	m.defs = defs
	return m, nil
}

func (m *scriptedModel) Generate(_ context.Context, msgs []model.Message) (*model.Response, error) {
	m.calls++
	m.history = append([]model.Message(nil), msgs...)
	if m.calls == 1 {
		return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "model-call-42", Name: "knowledge__get_document", Arguments: json.RawMessage(`{"id":"doc-1"}`)}}}}, nil
	}
	return &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}, nil
}

func TestAgentRemoteRoundTripAndAuthorization(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		label := "denied"
		if allowed {
			label = "allowed"
		}
		t.Run(label, func(t *testing.T) {
			ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
			store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Summary: "MCP", Content: "existing document body"}})
			peer, err := mcp.NewDemoServer(ctx, &knowledge.GetDocument{Store: store})
			if err != nil {
				t.Fatal(err)
			}
			transport := &recordingTransport{base: peer}
			client, err := mcp.NewClient(transport)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			discovered, err := Discover(ctx, client, "knowledge", 4)
			if err != nil {
				t.Fatal(err)
			}
			registry := tool.NewRegistry()
			for _, remote := range discovered {
				if err = registry.Register(ctx, remote); err != nil {
					t.Fatal(err)
				}
			}
			tools := registry.Tools()
			for name, target := range tools {
				described, ok := target.(tool.DescribedTool)
				if !ok {
					t.Fatal("lost description")
				}
				wrapped, err := tool.WithPolicy(described, tool.Policy{Timeout: time.Second, MaxResultBytes: 4096})
				if err != nil {
					t.Fatal(err)
				}
				tools[name] = wrapped
			}
			grants := []tool.Capability{}
			if allowed {
				grants = append(grants, tool.ReadCapability)
			}
			authorizer, err := tool.NewCapabilityAuthorizer(map[string]tool.Capability{"knowledge__get_document": tool.ReadCapability}, map[string][]tool.Capability{"reader": grants})
			if err != nil {
				t.Fatal(err)
			}
			scripted := &scriptedModel{}
			runtime, err := agent.New(scripted, tools, 3, agent.WithVisibleTools("knowledge__get_document"), agent.WithAuthorizer(authorizer))
			if err != nil {
				t.Fatal(err)
			}
			output, err := runtime.Run(ctx, "read a remote document")
			if len(scripted.defs) != 1 || scripted.defs[0].Name != "knowledge__get_document" {
				t.Fatal(scripted.defs)
			}
			if !allowed {
				if !errors.Is(err, tool.ErrForbidden) || transport.calls != 0 || scripted.calls != 1 {
					t.Fatalf("%v remote=%d model=%d", err, transport.calls, scripted.calls)
				}
				return
			}
			if err != nil || output != "done" || transport.calls != 1 || scripted.calls != 2 {
				t.Fatalf("%q %v remote=%d", output, err, transport.calls)
			}
			if len(scripted.history) != 3 || scripted.history[1].Role != "assistant" || scripted.history[2].Role != "tool" || scripted.history[2].ToolCallID != "model-call-42" {
				t.Fatalf("history=%+v", scripted.history)
			}
			var projected ResultEnvelope
			if json.Unmarshal([]byte(scripted.history[2].Content), &projected) != nil || len(projected.Text) != 1 || !strings.Contains(projected.Text[0], "existing document body") {
				t.Fatal(scripted.history[2].Content)
			}
			var request struct {
				ID     json.RawMessage `json:"id"`
				Params struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"params"`
			}
			if err = json.Unmarshal(transport.last, &request); err != nil {
				t.Fatal(err)
			}
			var rpcID uint64
			if json.Unmarshal(request.ID, &rpcID) != nil || rpcID == 0 || request.Params.Name != "get_document" || string(request.Params.Arguments) != `{"id":"doc-1"}` {
				t.Fatalf("wire=%s", transport.last)
			}
			if strings.Contains(string(transport.last), "model-call-42") || strings.Contains(string(transport.last), "reader") {
				t.Fatalf("local call identity leaked into wire arguments: %s", transport.last)
			}
		})
	}
}

func TestRegistryConflictDoesNotReplaceExistingTool(t *testing.T) {
	calls := [3]int{}
	makeRemote := func(prefix string, index int) tool.Invokable {
		caller := stubCaller{call: func(context.Context, string, json.RawMessage) (mcp.ToolResult, error) {
			calls[index]++
			return mcp.ToolResult{Content: []json.RawMessage{text(prefix)}}, nil
		}}
		remote, err := New(caller, prefix, descriptor("read"))
		if err != nil {
			t.Fatal(err)
		}
		return remote
	}
	registry := tool.NewRegistry()
	original := makeRemote("knowledge", 0)
	if err := registry.Register(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), makeRemote("knowledge", 1)); err == nil {
		t.Fatal("duplicate overwritten")
	}
	if err := registry.Register(context.Background(), makeRemote("other", 2)); err != nil {
		t.Fatal(err)
	}
	tools := registry.Tools()
	if tools["knowledge__read"] == nil || tools["other__read"] == nil {
		t.Fatalf("registered names: %v", tools)
	}
	if _, err := tools["knowledge__read"].Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if calls != [3]int{1, 0, 0} {
		t.Fatalf("conflict replaced original or other source ran: %v", calls)
	}
}
