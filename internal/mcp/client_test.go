package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type transportFunc func(context.Context, []byte) ([]byte, error)

func (f transportFunc) Send(ctx context.Context, b []byte) ([]byte, error) { return f(ctx, b) }

func decodeRequest(t *testing.T, b []byte) request {
	t.Helper()
	var r request
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func initializeResult() InitializeResult {
	return InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    &ServerCapabilities{Tools: &ToolsCapability{}},
		ServerInfo:      Implementation{Name: "test-server", Version: "1"},
	}
}

func ready(t *testing.T, transport Transport) *Client {
	t.Helper()
	// Isolate response tests from negotiation defects, which the lifecycle test checks.
	c, err := NewClient(transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
		r := decodeRequest(t, b)
		switch r.Method {
		case "initialize":
			return reply(r.ID, initializeResult(), nil)
		case "notifications/initialized":
			return nil, nil
		default:
			return transport.Send(ctx, b)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}

func assertFailedSession(t *testing.T, c *Client) {
	t.Helper()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); !errors.Is(err, ErrSessionState) {
		t.Errorf("reinitialize failed session: %v", err)
	}
	if _, err := c.ListTools(ctx, ""); !errors.Is(err, ErrSessionState) {
		t.Errorf("list on failed session: %v", err)
	}
	if _, err := c.CallTool(ctx, "lookup", nil); !errors.Is(err, ErrSessionState) {
		t.Errorf("call on failed session: %v", err)
	}
}

func TestLifecycleAndToolErrors(t *testing.T) {
	ctx := context.Background()
	var methods []string
	var ids []uint64
	calls := 0
	c, err := NewClient(transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
		r := decodeRequest(t, b)
		methods = append(methods, r.Method)
		ids = append(ids, r.ID)
		if r.JSONRPC != "2.0" {
			t.Errorf("request version = %q", r.JSONRPC)
		}
		switch r.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(r.Params, &params); err != nil || params.ProtocolVersion != ProtocolVersion {
				t.Errorf("initialize must negotiate protocolVersion %q: params=%s err=%v", ProtocolVersion, r.Params, err)
			}
			return reply(r.ID, initializeResult(), nil)
		case "notifications/initialized":
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(b, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["id"]; ok {
				t.Error("notification has an ID")
			}
			return nil, nil
		case "tools/list":
			return reply(r.ID, ToolsPage{Tools: []Descriptor{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}}, nil)
		case "tools/call":
			calls++
			switch calls {
			case 1:
				return reply(r.ID, ToolResult{Content: []json.RawMessage{json.RawMessage(`{"type":"text","text":"found"}`)}}, nil)
			case 2:
				return reply(r.ID, ToolResult{Content: []json.RawMessage{json.RawMessage(`{"type":"text","text":"not found"}`)}, IsError: true}, nil)
			default:
				return reply(r.ID, nil, &RPCError{Code: -32602, Message: "unknown tool", Data: json.RawMessage(`{"name":"unknown"}`)})
			}
		default:
			t.Fatalf("unexpected method %q", r.Method)
			return nil, nil
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListTools(ctx, ""); !errors.Is(err, ErrSessionState) {
		t.Fatalf("list before initialization: %v", err)
	}
	if _, err := c.CallTool(ctx, "lookup", nil); !errors.Is(err, ErrSessionState) || len(methods) != 0 {
		t.Fatalf("tool operation before initialization: %v methods=%v", err, methods)
	}
	init, err := c.Initialize(ctx)
	if err != nil || init.ProtocolVersion != ProtocolVersion || init.Capabilities == nil || init.Capabilities.Tools == nil {
		t.Fatalf("initialize: %+v %v", init, err)
	}
	page, err := c.ListTools(ctx, "")
	if err != nil || len(page.Tools) != 1 || page.Tools[0].Name != "lookup" {
		t.Fatalf("list: %+v %v", page, err)
	}
	result, err := c.CallTool(ctx, "lookup", json.RawMessage(`{"id":"doc-1"}`))
	if err != nil || result.IsError || len(result.Content) != 1 || string(result.Content[0]) != `{"type":"text","text":"found"}` {
		t.Fatalf("success result: %+v %v", result, err)
	}
	result, err = c.CallTool(ctx, "lookup", json.RawMessage(`{"id":"missing"}`))
	if err != nil || !result.IsError || len(result.Content) != 1 || string(result.Content[0]) != `{"type":"text","text":"not found"}` {
		t.Fatalf("business error must remain a result: %+v %v", result, err)
	}
	_, err = c.CallTool(ctx, "unknown", nil)
	var rpc *RPCError
	if !errors.As(err, &rpc) || rpc.Code != -32602 || rpc.Message != "unknown tool" || string(rpc.Data) != `{"name":"unknown"}` || errors.Is(err, ErrProtocol) {
		t.Fatalf("RPC error: %v", err)
	}
	if _, err := c.ListTools(ctx, ""); err != nil {
		t.Fatalf("valid RPC error killed session: %v", err)
	}
	if _, err := c.Initialize(ctx); !errors.Is(err, ErrSessionState) {
		t.Fatalf("reinitialized ready session: %v", err)
	}
	wantMethods := []string{"initialize", "notifications/initialized", "tools/list", "tools/call", "tools/call", "tools/call", "tools/list"}
	if !reflect.DeepEqual(methods, wantMethods) || !reflect.DeepEqual(ids, []uint64{1, 0, 2, 3, 4, 5, 6}) {
		t.Fatalf("wire lifecycle: methods=%v IDs=%v", methods, ids)
	}
}

func TestInitializeFailureNeverReplays(t *testing.T) {
	failure := errors.New("notification transport failed")
	for _, mode := range []string{"version", "capabilities", "server-name", "server-version", "notification-fails", "notification-response"} {
		t.Run(mode, func(t *testing.T) {
			sends := 0
			c, err := NewClient(transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
				sends++
				r := decodeRequest(t, b)
				if r.Method == "notifications/initialized" {
					if mode == "notification-fails" {
						return nil, failure
					}
					return []byte(`{}`), nil
				}
				if r.Method != "initialize" {
					t.Fatalf("unexpected method %q", r.Method)
				}
				result := initializeResult()
				switch mode {
				case "version":
					result.ProtocolVersion = "unsupported"
				case "capabilities":
					result.Capabilities = nil
				case "server-name":
					result.ServerInfo.Name = ""
				case "server-version":
					result.ServerInfo.Version = ""
				}
				return reply(r.ID, result, nil)
			}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Initialize(context.Background())
			wantErr := ErrProtocol
			wantSends := 1
			if strings.HasPrefix(mode, "notification-") {
				wantSends = 2
			}
			if mode == "notification-fails" {
				wantErr = failure
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("initialize error = %v, want %v", err, wantErr)
			}
			assertFailedSession(t, c)
			if sends != wantSends {
				t.Fatalf("failed initialization replayed: sends=%d want=%d", sends, wantSends)
			}
		})
	}
}

func TestNoToolsCapability(t *testing.T) {
	var methods []string
	c, err := NewClient(transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
		r := decodeRequest(t, b)
		methods = append(methods, r.Method)
		if r.Method == "initialize" {
			result := initializeResult()
			result.Capabilities = &ServerCapabilities{}
			return reply(r.ID, result, nil)
		}
		if r.Method != "notifications/initialized" {
			t.Fatalf("tool operation without capability: %q", r.Method)
		}
		return nil, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrToolsUnavailable) {
		t.Fatal(err)
	}
	if _, err := c.CallTool(context.Background(), "lookup", nil); !errors.Is(err, ErrToolsUnavailable) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"initialize", "notifications/initialized"}) {
		t.Fatalf("notification missing or tools sent: %v", methods)
	}
}

func TestMalformedResponsesFailSession(t *testing.T) {
	valid := `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`
	for _, tc := range []struct {
		name, payload string
	}{
		{"id", `{"jsonrpc":"2.0","id":99,"result":{"tools":[]}}`},
		{"string-id", `{"jsonrpc":"2.0","id":"2","result":{"tools":[]}}`},
		{"missing-id", `{"jsonrpc":"2.0","result":{"tools":[]}}`},
		{"version", `{"jsonrpc":"1.0","id":2,"result":{"tools":[]}}`},
		{"both", `{"jsonrpc":"2.0","id":2,"result":{"tools":[]},"error":{"code":-1,"message":"error"}}`},
		{"neither", `{"jsonrpc":"2.0","id":2}`},
		{"null-error", `{"jsonrpc":"2.0","id":2,"error":null}`},
		{"missing-code", `{"jsonrpc":"2.0","id":2,"error":{"message":"error"}}`},
		{"null-code", `{"jsonrpc":"2.0","id":2,"error":{"code":null,"message":"error"}}`},
		{"missing-message", `{"jsonrpc":"2.0","id":2,"error":{"code":-1}}`},
		{"invalid-message", `{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":42}}`},
		{"null-result", `{"jsonrpc":"2.0","id":2,"result":null}`},
		{"array-result", `{"jsonrpc":"2.0","id":2,"result":[]}`},
		{"empty-result", `{"jsonrpc":"2.0","id":2,"result":{}}`},
		{"null-tools", `{"jsonrpc":"2.0","id":2,"result":{"tools":null}}`},
		{"non-array-tools", `{"jsonrpc":"2.0","id":2,"result":{"tools":{}}}`},
		{"bad-json", `{`},
		{"empty-response", ""},
		{"oversize", valid + strings.Repeat(" ", MaxResponseBytes+1-len(valid))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			c := ready(t, transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
				sends++
				if r := decodeRequest(t, b); r.Method != "tools/list" {
					t.Errorf("unexpected method %q", r.Method)
				}
				return []byte(tc.payload), nil
			}))
			if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrProtocol) {
				t.Fatalf("malformed response accepted: %v", err)
			}
			assertFailedSession(t, c)
			if sends != 1 {
				t.Fatalf("failed session sent %d operations", sends)
			}
		})
	}
	t.Run("response-at-limit", func(t *testing.T) {
		c := ready(t, transportFunc(func(context.Context, []byte) ([]byte, error) {
			return []byte(valid + strings.Repeat(" ", MaxResponseBytes-len(valid))), nil
		}))
		page, err := c.ListTools(context.Background(), "")
		if err != nil || page.Tools == nil || len(page.Tools) != 0 {
			t.Fatalf("valid response at byte limit rejected: %+v %v", page, err)
		}
	})
}

func TestPaginationAndStructuredContentPreserved(t *testing.T) {
	cursor := " opaque/+==?游标\x00\n"
	descriptor := Descriptor{
		Name:        "lookup",
		Description: "read a document",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		Annotations: json.RawMessage(`{"readOnlyHint":true,"custom":{"scope":"docs"}}`),
	}
	blocks := []json.RawMessage{
		json.RawMessage(`{"type":"text","text":"caption","annotations":{"audience":["user"]}}`),
		json.RawMessage(`{"type":"image","data":"AA==","mimeType":"image/png"}`),
		json.RawMessage(`{"type":"resource","resource":{"uri":"doc://1","mimeType":"text/plain","text":"body"}}`),
	}
	structured := json.RawMessage(`{"document":"doc-1","nested":{"items":[1,true,null,{"text":"原文"}]}}`)
	lists := 0
	c := ready(t, transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
		r := decodeRequest(t, b)
		switch r.Method {
		case "tools/list":
			lists++
			var params map[string]string
			if err := json.Unmarshal(r.Params, &params); err != nil {
				t.Fatal(err)
			}
			if lists == 1 {
				if _, exists := params["cursor"]; exists {
					t.Error("first request must omit cursor")
				}
				return reply(r.ID, ToolsPage{Tools: []Descriptor{descriptor}, NextCursor: cursor}, nil)
			}
			if lists != 2 || params["cursor"] != cursor {
				t.Fatalf("cursor changed or client auto-paginated: lists=%d params=%q", lists, params)
			}
			return reply(r.ID, ToolsPage{Tools: []Descriptor{}}, nil)
		case "tools/call":
			return reply(r.ID, ToolResult{Content: blocks, StructuredContent: structured}, nil)
		default:
			t.Fatalf("unexpected method %q", r.Method)
			return nil, nil
		}
	}))
	page, err := c.ListTools(context.Background(), "")
	if err != nil || page.NextCursor != cursor || lists != 1 || !reflect.DeepEqual(page.Tools, []Descriptor{descriptor}) {
		t.Fatalf("first page lost metadata or cursor: %+v %v lists=%d", page, err, lists)
	}
	page, err = c.ListTools(context.Background(), page.NextCursor)
	if err != nil || page.NextCursor != "" || page.Tools == nil || len(page.Tools) != 0 || lists != 2 {
		t.Fatalf("last page: %+v %v lists=%d", page, err, lists)
	}
	result, err := c.CallTool(context.Background(), "lookup", nil)
	if err != nil || result.IsError || !reflect.DeepEqual(result.Content, blocks) || !reflect.DeepEqual(result.StructuredContent, structured) {
		t.Fatalf("content was flattened or changed: %+v %v", result, err)
	}
}

func TestCancellationAndQueuedCall(t *testing.T) {
	for _, mode := range []string{"cancel-with-transport-error", "deadline-with-late-success"} {
		t.Run(mode, func(t *testing.T) {
			failure := errors.New("transport replaced context error")
			var sends atomic.Int32
			started := make(chan struct{})
			c := ready(t, transportFunc(func(ctx context.Context, b []byte) ([]byte, error) {
				if sends.Add(1) != 1 {
					return nil, failure
				}
				r := decodeRequest(t, b)
				close(started)
				<-ctx.Done()
				if mode == "deadline-with-late-success" {
					return reply(r.ID, ToolResult{Content: []json.RawMessage{}}, nil)
				}
				return nil, failure
			}))
			var ctx context.Context
			var cancel context.CancelFunc
			wantErr := context.Canceled
			if mode == "deadline-with-late-success" {
				ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
				wantErr = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.CallTool(ctx, "lookup", nil)
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("call did not reach transport")
			}
			queued, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer stop()
			queuedDone := make(chan error, 1)
			go func() {
				_, err := c.ListTools(queued, "")
				queuedDone <- err
			}()
			select {
			case err := <-queuedDone:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("queued cancellation: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("queued cancellation blocked")
			}
			if sends.Load() != 1 {
				t.Fatalf("queued operation reached transport: sends=%d", sends.Load())
			}
			if mode == "cancel-with-transport-error" {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, wantErr) || errors.Is(err, failure) {
					t.Fatalf("transport cancellation lost context error: got=%v want=%v", err, wantErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("transport cancellation blocked")
			}
			assertFailedSession(t, c)
			if sends.Load() != 1 {
				t.Fatalf("uncertain session reused: sends=%d", sends.Load())
			}
		})
	}
}

func TestCallInputAndContentValidation(t *testing.T) {
	t.Run("local-input", func(t *testing.T) {
		sends := 0
		c := ready(t, transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
			sends++
			r := decodeRequest(t, b)
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(r.Params, &params); err != nil || r.Method != "tools/call" || params.Name != "lookup" || string(params.Arguments) != "{}" {
				t.Errorf("valid call params: %s %v", b, err)
			}
			if r.ID != uint64(sends+1) {
				t.Errorf("local rejection consumed request ID: got=%d sends=%d", r.ID, sends)
			}
			return reply(r.ID, ToolResult{Content: []json.RawMessage{}}, nil)
		}))
		for _, tc := range []struct{ name, args string }{
			{"lookup", "null"},
			{"lookup", "[]"},
			{"lookup", "1"},
			{"lookup", `"text"`},
			{"lookup", "{"},
			{"", "{}"},
			{" \t\n", "{}"},
		} {
			if _, err := c.CallTool(context.Background(), tc.name, json.RawMessage(tc.args)); err == nil {
				t.Errorf("accepted invalid input: name=%q args=%q", tc.name, tc.args)
			}
			if sends != 0 {
				t.Fatalf("invalid local input reached transport: sends=%d", sends)
			}
		}
		for _, args := range []json.RawMessage{nil, json.RawMessage(`{}`)} {
			if _, err := c.CallTool(context.Background(), "lookup", args); err != nil {
				t.Fatalf("local rejection damaged session: %v", err)
			}
		}
		if sends != 2 {
			t.Fatalf("valid calls sent %d times", sends)
		}
	})
	for _, tc := range []struct{ name, result string }{
		{"missing-content", `{}`},
		{"null-content", `{"content":null}`},
		{"non-array-content", `{"content":{}}`},
		{"missing-type", `{"content":[{}]}`},
		{"null-block", `{"content":[null]}`},
		{"non-object-block", `{"content":["text"]}`},
		{"empty-type", `{"content":[{"type":""}]}`},
		{"non-string-type", `{"content":[{"type":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			c := ready(t, transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
				sends++
				return reply(decodeRequest(t, b).ID, json.RawMessage(tc.result), nil)
			}))
			if _, err := c.CallTool(context.Background(), "lookup", nil); !errors.Is(err, ErrProtocol) {
				t.Fatalf("invalid content accepted: %v", err)
			}
			assertFailedSession(t, c)
			if sends != 1 {
				t.Fatalf("invalid content session reused: sends=%d", sends)
			}
		})
	}
}

func TestInvalidDescriptorsAndTransportFailure(t *testing.T) {
	for _, tc := range []struct{ name, result string }{
		{"non-object-schema", `{"tools":[{"name":"x","inputSchema":{"type":"string"}}]}`},
		{"missing-schema", `{"tools":[{"name":"x"}]}`},
		{"null-schema", `{"tools":[{"name":"x","inputSchema":null}]}`},
		{"array-schema", `{"tools":[{"name":"x","inputSchema":[]}]}`},
		{"empty-name", `{"tools":[{"name":"","inputSchema":{"type":"object"}}]}`},
		{"blank-name", `{"tools":[{"name":" \t","inputSchema":{"type":"object"}}]}`},
		{"duplicate-name", `{"tools":[{"name":"x","inputSchema":{"type":"object"}},{"name":"x","inputSchema":{"type":"object"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			c := ready(t, transportFunc(func(_ context.Context, b []byte) ([]byte, error) {
				sends++
				return reply(decodeRequest(t, b).ID, json.RawMessage(tc.result), nil)
			}))
			if _, err := c.ListTools(context.Background(), ""); !errors.Is(err, ErrProtocol) {
				t.Fatalf("invalid descriptor accepted: %v", err)
			}
			assertFailedSession(t, c)
			if sends != 1 {
				t.Fatalf("invalid descriptor session reused: sends=%d", sends)
			}
		})
	}
	t.Run("lost-response", func(t *testing.T) {
		failure := errors.New("transport lost response")
		sends := 0
		c := ready(t, transportFunc(func(context.Context, []byte) ([]byte, error) {
			sends++
			return nil, failure
		}))
		if _, err := c.CallTool(context.Background(), "lookup", nil); !errors.Is(err, failure) {
			t.Fatalf("transport cause lost: %v", err)
		}
		assertFailedSession(t, c)
		if sends != 1 {
			t.Fatalf("lost response retried: sends=%d", sends)
		}
	})
}
