package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/mcptool"
)

const (
	localName  = "get_document"
	remoteName = "knowledge__get_document"
	localID    = "local-read"
	remoteID   = "remote-read"
)

type countedTool struct {
	target tool.Invokable
	calls  int
}

func (c *countedTool) Meta(ctx context.Context) (tool.Spec, error) { return c.target.Meta(ctx) }
func (c *countedTool) Invoke(ctx context.Context, args json.RawMessage) (string, error) {
	c.calls++
	return c.target.Invoke(ctx, args)
}

type wireProbe struct {
	target              mcp.Transport
	total, toolRequests int
	corrupt, wait       bool
	lastCall            []byte
}

func (p *wireProbe) Send(ctx context.Context, b []byte) ([]byte, error) {
	p.total++
	var req struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return nil, err
	}
	if req.Method != "tools/call" {
		return p.target.Send(ctx, b)
	}
	p.toolRequests++
	p.lastCall = append([]byte(nil), b...)
	if p.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	result, err := p.target.Send(ctx, b)
	if err == nil && p.corrupt {
		// The peer has already executed the operation. A bad response cannot undo it.
		return []byte(`{"jsonrpc":"2.0","id":999,"result":{"content":[{"type":"text","text":"PRIVATE-REMOTE"}]}}`), nil
	}
	return result, err
}

type weekModel struct {
	batch   []model.ToolCall
	calls   int
	defs    []model.ToolDefinition
	history []model.Message
}

func (m *weekModel) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	m.defs = append([]model.ToolDefinition(nil), defs...)
	return m, nil
}
func (m *weekModel) Generate(ctx context.Context, messages []model.Message) (*model.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.calls++
	m.history = append([]model.Message(nil), messages...)
	if m.calls == 1 {
		return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: m.batch}}, nil
	}
	return &model.Response{Message: model.Message{Role: "assistant", Content: "scripted final"}}, nil
}

type eventProbe struct{ events []agent.Event }

func (p *eventProbe) Emit(_ context.Context, e agent.Event) { p.events = append(p.events, e) }

func TestWeek02EndToEndMatrix(t *testing.T) {
	cases := []struct {
		name                                                string
		deny, duplicate, budget, business, corrupt, wait    bool
		localCalls, remoteRequests, remoteCalls, modelCalls int
		terminal                                            bool
		controlMessage                                      string
		sentinel                                            error
	}{
		{name: "mixed-success", localCalls: 1, remoteRequests: 1, remoteCalls: 1, modelCalls: 2},
		{name: "unapproved-remote", deny: true, modelCalls: 1, terminal: true, sentinel: tool.ErrForbidden},
		{name: "duplicate-model-ID", duplicate: true, modelCalls: 1, terminal: true},
		{name: "remote-result-budget", budget: true, localCalls: 1, remoteRequests: 1, remoteCalls: 1, modelCalls: 2, controlMessage: "execution_failed: tool result exceeds byte budget"},
		{name: "remote-business-failure", business: true, localCalls: 1, remoteRequests: 1, remoteCalls: 1, modelCalls: 2, controlMessage: "execution_failed: remote tool execution failed"},
		{name: "corrupt-response-after-execution", corrupt: true, localCalls: 1, remoteRequests: 1, remoteCalls: 1, modelCalls: 2, controlMessage: "execution_failed: remote tool request failed"},
		{name: "remote-timeout", wait: true, localCalls: 1, remoteRequests: 1, modelCalls: 1, terminal: true, sentinel: context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ctx := tool.WithPrincipal(parent, tool.Principal{ID: "reader"})
			local := &countedTool{target: &knowledge.GetDocument{Store: knowledge.NewFakeStore([]knowledge.Document{{ID: "local-1", Content: "local reference"}})}}
			remote := &countedTool{target: &knowledge.GetDocument{Store: knowledge.NewFakeStore([]knowledge.Document{{ID: "remote-1", Content: "remote reference"}})}}
			peer, err := mcp.NewDemoServer(ctx, remote)
			if err != nil {
				t.Fatal(err)
			}
			wire := &wireProbe{target: peer, corrupt: tc.corrupt, wait: tc.wait}
			client, err := mcp.NewClient(wire)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			remoteTools, err := mcptool.Discover(ctx, client, "knowledge", 4)
			if err != nil {
				t.Fatal(err)
			}
			registry := tool.NewRegistry()
			if err = registry.Register(ctx, local); err != nil {
				t.Fatal(err)
			}
			for _, candidate := range remoteTools {
				if err = registry.Register(ctx, candidate); err != nil {
					t.Fatal(err)
				}
			}
			tools := registry.Tools()
			for name, target := range tools {
				described, ok := target.(tool.DescribedTool)
				if !ok {
					t.Fatal("missing metadata")
				}
				policy := tool.Policy{Timeout: time.Second, MaxResultBytes: 4096}
				if name == remoteName && tc.budget {
					policy.MaxResultBytes = 8
				}
				if name == remoteName && tc.wait {
					policy.Timeout = 20 * time.Millisecond
				}
				wrapped, err := tool.WithPolicy(described, policy)
				if err != nil {
					t.Fatal(err)
				}
				tools[name] = wrapped
			}
			rules := map[string]tool.Capability{localName: tool.ReadCapability}
			// A discovered tool is not automatically trusted/classified by the host.
			if !tc.deny {
				rules[remoteName] = tool.ReadCapability
			}
			auth, err := tool.NewCapabilityAuthorizer(rules, map[string][]tool.Capability{"reader": {tool.ReadCapability}})
			if err != nil {
				t.Fatal(err)
			}
			document := "remote-1"
			if tc.business {
				document = "missing"
			}
			args, _ := json.Marshal(map[string]string{"id": document})
			batch := []model.ToolCall{
				{ID: localID, Name: localName, Arguments: json.RawMessage(`{"id":"local-1"}`)},
				{ID: remoteID, Name: remoteName, Arguments: args},
			}
			if tc.duplicate {
				batch[1].ID = localID
			}
			scripted := &weekModel{batch: batch}
			trace := &eventProbe{}
			runtime, err := agent.New(scripted, tools, 3,
				agent.WithVisibleTools(localName, remoteName),
				agent.WithAuthorizer(auth), agent.WithEventSink(trace), agent.WithToolTimeout(2*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			output, runErr := runtime.Run(ctx, "compare known local-1 and remote-1 documents")
			if tc.terminal {
				if runErr == nil || output != "" {
					t.Fatalf("terminal output=%q err=%v", output, runErr)
				}
				if tc.sentinel != nil && !errors.Is(runErr, tc.sentinel) {
					t.Fatalf("lost terminal error: %v", runErr)
				}
			} else if runErr != nil || output != "scripted final" {
				t.Fatalf("%q %v", output, runErr)
			}
			if local.calls != tc.localCalls || wire.toolRequests != tc.remoteRequests || remote.calls != tc.remoteCalls || scripted.calls != tc.modelCalls {
				t.Fatalf("local=%d RPC=%d remoteInvoke=%d model=%d", local.calls, wire.toolRequests, remote.calls, scripted.calls)
			}
			defs := map[string]bool{}
			for _, d := range scripted.defs {
				defs[d.Name] = true
			}
			if len(defs) != 2 || !defs[localName] || !defs[remoteName] {
				t.Fatal(scripted.defs)
			}
			eventCounts := map[agent.EventType]int{}
			toolErrors, toolCancelled := 0, 0
			for _, e := range trace.events {
				eventCounts[e.Type]++
				if e.Type == agent.EventToolAuthorized || e.Type == agent.EventToolDenied {
					if e.PrincipalID != "reader" || e.Step != 1 || e.At.IsZero() {
						t.Fatalf("audit=%+v", e)
					}
				}
				if e.Type == agent.EventToolCompleted {
					if e.Status == "error" {
						toolErrors++
					}
					if e.Status == "cancelled" {
						toolCancelled++
					}
				}
			}
			if eventCounts[agent.EventRunStarted] != 1 || eventCounts[agent.EventRunFinished]+eventCounts[agent.EventRunFailed] != 1 {
				t.Fatalf("invalid run lifecycle: %+v", eventCounts)
			}
			wantFailed := 0
			if tc.terminal {
				wantFailed = 1
			}
			if eventCounts[agent.EventRunFailed] != wantFailed {
				t.Fatalf("terminal=%+v", eventCounts)
			}
			expectedStarted := tc.localCalls
			if tc.remoteRequests > 0 {
				expectedStarted++
			}
			if eventCounts[agent.EventToolStarted] != expectedStarted || eventCounts[agent.EventToolCompleted] != expectedStarted {
				t.Fatalf("unpaired tool events: %+v", eventCounts)
			}
			if tc.deny {
				// An allow decision for the first item is not an execution commitment.
				if eventCounts[agent.EventToolAuthorized] != 1 || eventCounts[agent.EventToolDenied] != 1 || eventCounts[agent.EventToolStarted] != 0 {
					t.Fatalf("preflight=%+v", eventCounts)
				}
			}
			if tc.controlMessage != "" && toolErrors != 1 {
				t.Fatalf("tool errors=%d", toolErrors)
			}
			if tc.wait && toolCancelled != 1 {
				t.Fatalf("cancelled completions=%d", toolCancelled)
			}
			if scripted.calls == 2 {
				ms := scripted.history
				if len(ms) != 4 || ms[1].Role != "assistant" || len(ms[1].ToolCalls) != 2 || ms[1].ToolCalls[1].Name != remoteName {
					t.Fatalf("missing assistant history: %+v", ms)
				}
				if ms[2].Role != "tool" || ms[2].ToolCallID != localID || ms[3].Role != "tool" || ms[3].ToolCallID != remoteID {
					t.Fatalf("message pairing: %+v", ms)
				}
				var localDoc knowledge.Document
				if json.Unmarshal([]byte(ms[2].Content), &localDoc) != nil || localDoc.Content != "local reference" {
					t.Fatal(ms[2].Content)
				}
				if tc.controlMessage != "" {
					if ms[3].Content != tc.controlMessage || strings.Contains(ms[3].Content, "PRIVATE-REMOTE") || strings.Contains(ms[3].Content, "remote reference") {
						t.Fatalf("unsafe error projection: %q", ms[3].Content)
					}
				} else {
					var projected mcptool.ResultEnvelope
					if json.Unmarshal([]byte(ms[3].Content), &projected) != nil || len(projected.Text) != 1 {
						t.Fatal(ms[3].Content)
					}
					var remoteDoc knowledge.Document
					if json.Unmarshal([]byte(projected.Text[0]), &remoteDoc) != nil || remoteDoc.Content != "remote reference" {
						t.Fatal(projected)
					}
				}
			}
			if wire.toolRequests > 0 {
				var request struct {
					ID     json.RawMessage `json:"id"`
					Params struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"params"`
				}
				if json.Unmarshal(wire.lastCall, &request) != nil || request.Params.Name != "get_document" {
					t.Fatalf("wire=%s", wire.lastCall)
				}
				var id uint64
				if json.Unmarshal(request.ID, &id) != nil || id == 0 || strings.Contains(string(wire.lastCall), remoteID) || strings.Contains(string(wire.lastCall), "reader") {
					t.Fatalf("identity domains mixed: %s", wire.lastCall)
				}
			}
			if tc.corrupt || tc.wait {
				before := wire.total
				_, err = client.ListTools(ctx, "")
				if !errors.Is(err, mcp.ErrSessionState) || wire.total != before {
					t.Fatalf("ambiguous session replay: %v total=%d", err, wire.total)
				}
			}
			t.Logf("local=%d tools/call=%d remoteInvoke=%d model=%d finished=%d failed=%d",
				local.calls, wire.toolRequests, remote.calls, scripted.calls, eventCounts[agent.EventRunFinished], eventCounts[agent.EventRunFailed])
		})
	}
}
