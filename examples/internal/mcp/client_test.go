package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
)

type transportFunc func(context.Context, []byte) ([]byte, error)

func (f transportFunc) Send(ctx context.Context, b []byte) ([]byte, error) { return f(ctx, b) }
func fixture(t *testing.T) *DemoServer {
	t.Helper()
	store := knowledge.NewFakeStore([]knowledge.Document{{ID: "doc-1", Summary: "MCP", Content: "existing knowledge body"}})
	s, err := NewDemoServer(context.Background(), &knowledge.GetDocument{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func ready(t *testing.T, transport Transport) *Client {
	t.Helper()
	c, err := NewClient(transport)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}
func decodeRequest(t *testing.T, b []byte) request {
	t.Helper()
	var r request
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLifecycleAndToolErrors(t *testing.T) {
	ctx := context.Background()
	server := fixture(t)
	var methods []string
	var ids []uint64
	transport := transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		req := decodeRequest(t, b)
		methods = append(methods, req.Method)
		ids = append(ids, req.ID)
		if req.Method == "notifications/initialized" {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(b, &fields)
			if _, ok := fields["id"]; ok {
				t.Fatal("notification has an ID")
			}
		}
		return server.Send(ctx, b)
	})
	c, _ := NewClient(transport)
	if _, err := c.ListTools(ctx, ""); !errors.Is(err, ErrSessionState) || len(methods) != 0 {
		t.Fatalf("preinit: %v %v", err, methods)
	}
	init, err := c.Initialize(ctx)
	if err != nil || init.ProtocolVersion != ProtocolVersion || init.Capabilities.Tools == nil {
		t.Fatalf("%+v %v", init, err)
	}
	page, err := c.ListTools(ctx, "")
	if err != nil || len(page.Tools) != 1 || page.Tools[0].Name != "get_document" {
		t.Fatalf("%+v %v", page, err)
	}
	result, err := c.CallTool(ctx, "get_document", json.RawMessage(`{"id":"doc-1"}`))
	if err != nil || result.IsError || len(result.Content) != 1 || !strings.Contains(string(result.Content[0]), "existing knowledge body") {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = c.CallTool(ctx, "get_document", json.RawMessage(`{"id":"missing"}`))
	if err != nil || !result.IsError {
		t.Fatalf("business error: %+v %v", result, err)
	}
	_, err = c.CallTool(ctx, "unknown", nil)
	var rpc *RPCError
	if !errors.As(err, &rpc) || rpc.Code != -32602 {
		t.Fatalf("RPC error: %v", err)
	}
	if _, err = c.ListTools(ctx, ""); err != nil {
		t.Fatalf("valid RPC error killed session: %v", err)
	}
	if _, err = c.Initialize(ctx); !errors.Is(err, ErrSessionState) {
		t.Fatal("reinitialized session")
	}
	if strings.Join(methods[:4], ",") != "initialize,notifications/initialized,tools/list,tools/call" || ids[0] != 1 || ids[1] != 0 || ids[2] != 2 || ids[3] != 3 {
		t.Fatalf("methods=%v IDs=%v", methods, ids)
	}
}

func TestInitializeFailureNeverReplays(t *testing.T) {
	failure := errors.New("notification transport failed")
	for _, mode := range []string{"version", "capabilities", "server", "notification-fails", "notification-response"} {
		t.Run(mode, func(t *testing.T) {
			server := fixture(t)
			sends := 0
			c, _ := NewClient(transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
				sends++
				req := decodeRequest(t, b)
				if req.Method == "notifications/initialized" {
					if mode == "notification-fails" {
						return nil, failure
					}
					if mode == "notification-response" {
						return []byte("{}"), nil
					}
				}
				data, err := server.Send(ctx, b)
				if err != nil || req.Method != "initialize" {
					return data, err
				}
				var r response
				_ = json.Unmarshal(data, &r)
				var result InitializeResult
				_ = json.Unmarshal(r.Result, &result)
				switch mode {
				case "version":
					result.ProtocolVersion = "unsupported"
				case "capabilities":
					result.Capabilities = nil
				case "server":
					result.ServerInfo.Name = ""
				}
				return reply(req.ID, result, nil)
			}))
			_, err := c.Initialize(context.Background())
			if mode == "notification-fails" {
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrProtocol) {
				t.Fatal(err)
			}
			before := sends
			if _, err = c.Initialize(context.Background()); !errors.Is(err, ErrSessionState) {
				t.Fatal(err)
			}
			if _, err = c.ListTools(context.Background(), ""); !errors.Is(err, ErrSessionState) || sends != before {
				t.Fatalf("replay: %v sends=%d", err, sends)
			}
		})
	}
}

func TestNoToolsCapability(t *testing.T) {
	server := fixture(t)
	sends := 0
	c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		sends++
		req := decodeRequest(t, b)
		if req.Method == "initialize" {
			return reply(req.ID, InitializeResult{ProtocolVersion: ProtocolVersion, Capabilities: &ServerCapabilities{}, ServerInfo: Implementation{Name: "no-tools", Version: "1"}}, nil)
		}
		// Notification still must be sent, even when tools are not advertised.
		if req.Method == "notifications/initialized" {
			return nil, nil
		}
		return server.Send(ctx, b)
	}))
	if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrToolsUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.CallTool(context.Background(), "x", nil); !errors.Is(err, ErrToolsUnavailable) || sends != 2 {
		t.Fatalf("%v sends=%d", err, sends)
	}
}

func TestMalformedResponsesFailSession(t *testing.T) {
	for _, mode := range []string{"id", "version", "both", "neither", "null-error", "null-result", "missing-tools", "bad-json", "oversize", "null-code"} {
		t.Run(mode, func(t *testing.T) {
			server := fixture(t)
			lists := 0
			c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
				req := decodeRequest(t, b)
				if req.Method != "tools/list" {
					return server.Send(ctx, b)
				}
				lists++
				switch mode {
				case "bad-json":
					return []byte("{"), nil
				case "oversize":
					return []byte(strings.Repeat(" ", MaxResponseBytes+1)), nil
				}
				var r response
				r.JSONRPC = "2.0"
				r.ID = json.RawMessage("2")
				r.Result = json.RawMessage(`{"tools":[]}`)
				switch mode {
				case "id":
					r.ID = json.RawMessage("99")
				case "version":
					r.JSONRPC = "1.0"
				case "both":
					r.Error = json.RawMessage(`{"code":-1,"message":"error"}`)
				case "neither":
					r.Result = nil
				case "null-error":
					r.Result = nil
					r.Error = json.RawMessage("null")
				case "null-result":
					r.Result = json.RawMessage("null")
				case "missing-tools":
					r.Result = json.RawMessage("{}")
				case "null-code":
					r.Result = nil
					r.Error = json.RawMessage(`{"code":null,"message":"bad"}`)
				}
				return json.Marshal(r)
			}))
			if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrProtocol) {
				t.Fatal(err)
			}
			if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrSessionState) || lists != 1 {
				t.Fatalf("%v lists=%d", err, lists)
			}
		})
	}
}

func TestPaginationAndStructuredContentPreserved(t *testing.T) {
	server := fixture(t)
	lists := 0
	c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		req := decodeRequest(t, b)
		switch req.Method {
		case "tools/list":
			var p struct {
				Cursor string `json:"cursor"`
			}
			_ = json.Unmarshal(req.Params, &p)
			lists++
			if lists == 1 {
				if p.Cursor != "" {
					t.Fatal("first cursor")
				}
				return reply(req.ID, ToolsPage{Tools: []Descriptor{}, NextCursor: "opaque/next"}, nil)
			}
			if p.Cursor != "opaque/next" {
				t.Fatalf("cursor %q", p.Cursor)
			}
			return reply(req.ID, ToolsPage{Tools: []Descriptor{}}, nil)
		case "tools/call":
			return reply(req.ID, ToolResult{Content: []json.RawMessage{json.RawMessage(`{"type":"image","data":"AA==","mimeType":"image/png"}`)}, StructuredContent: json.RawMessage(`{"document":"doc-1"}`)}, nil)
		}
		return server.Send(ctx, b)
	}))
	page, err := c.ListTools(context.Background(), "")
	if err != nil || page.NextCursor != "opaque/next" || lists != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	page, err = c.ListTools(context.Background(), page.NextCursor)
	if err != nil || page.NextCursor != "" || lists != 2 {
		t.Fatalf("%+v %v", page, err)
	}
	r, err := c.CallTool(context.Background(), "get_document", nil)
	if err != nil || !strings.Contains(string(r.Content[0]), "image/png") || string(r.StructuredContent) != `{"document":"doc-1"}` {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestCancellationAndQueuedCall(t *testing.T) {
	server := fixture(t)
	var calls atomic.Int32
	started := make(chan struct{})
	c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		req := decodeRequest(t, b)
		if req.Method != "tools/call" {
			return server.Send(ctx, b)
		}
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.CallTool(ctx, "get_document", nil); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("call did not start")
	}
	queued, stop := context.WithCancel(context.Background())
	defer stop()
	queuedDone := make(chan error, 1)
	queuedStarted := make(chan struct{})
	go func() { close(queuedStarted); _, err := c.ListTools(queued, ""); queuedDone <- err }()
	<-queuedStarted
	select {
	case err := <-queuedDone:
		t.Fatalf("queued request returned before cancellation: %v", err)
	case <-time.After(5 * time.Millisecond):
	}
	stop()
	select {
	case err := <-queuedDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued cancellation blocked")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	if calls.Load() != 1 {
		t.Fatal("cancelled request sent")
	}
	if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrSessionState) {
		t.Fatal("ambiguous cancelled session reused")
	}
}

func TestCallInputAndContentValidation(t *testing.T) {
	server := fixture(t)
	calls := 0
	c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		if decodeRequest(t, b).Method == "tools/call" {
			calls++
		}
		return server.Send(ctx, b)
	}))
	for _, args := range []string{"null", "[]", "{"} {
		if _, err := c.CallTool(context.Background(), "get_document", json.RawMessage(args)); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
	if _, err := c.CallTool(context.Background(), "", nil); err == nil || calls != 0 {
		t.Fatal("invalid local call was sent")
	}
	for _, raw := range []string{`{}`, `{"content":null}`, `{"content":[{}]}`, `{"content":[null]}`} {
		t.Run(raw, func(t *testing.T) {
			peer := fixture(t)
			client := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
				req := decodeRequest(t, b)
				if req.Method == "tools/call" {
					return reply(req.ID, json.RawMessage(raw), nil)
				}
				return peer.Send(ctx, b)
			}))
			if _, err := client.CallTool(context.Background(), "get_document", nil); !errors.Is(err, ErrProtocol) {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidDescriptorsAndTransportFailure(t *testing.T) {
	for _, raw := range []string{
		`{"tools":[{"name":"x","inputSchema":{"type":"string"}}]}`,
		`{"tools":[{"name":"","inputSchema":{"type":"object"}}]}`,
		`{"tools":[{"name":"x","inputSchema":{"type":"object"}},{"name":"x","inputSchema":{"type":"object"}}]}`,
	} {
		peer := fixture(t)
		c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
			req := decodeRequest(t, b)
			if req.Method == "tools/list" {
				return reply(req.ID, json.RawMessage(raw), nil)
			}
			return peer.Send(ctx, b)
		}))
		if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrProtocol) {
			t.Fatal(err)
		}
	}
	failure := errors.New("transport lost response")
	peer := fixture(t)
	c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		if decodeRequest(t, b).Method == "tools/call" {
			return nil, failure
		}
		return peer.Send(ctx, b)
	}))
	if _, err := c.CallTool(context.Background(), "get_document", nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrSessionState) {
		t.Fatal("lost response session reused")
	}
}
