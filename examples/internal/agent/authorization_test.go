package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

type accessCounter struct {
	name  string
	calls int
	seen  string
}

func (c *accessCounter) Name() string                     { return c.name }
func (c *accessCounter) Definition() model.ToolDefinition { return model.ToolDefinition{Name: c.name} }
func (c *accessCounter) Execute(_ context.Context, args json.RawMessage) (string, error) {
	c.calls++
	c.seen = string(args)
	return "ok", nil
}

type accessModel struct {
	first   []model.ToolCall
	calls   int
	history []model.Message
	defs    []model.ToolDefinition
}

func (m *accessModel) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	m.defs = defs
	return m, nil
}
func (m *accessModel) Generate(_ context.Context, ms []model.Message) (*model.Response, error) {
	m.calls++
	m.history = append([]model.Message(nil), ms...)
	if m.calls == 1 {
		return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: m.first}}, nil
	}
	return &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}, nil
}
func accessPolicy(t *testing.T) *tool.CapabilityAuthorizer {
	t.Helper()
	a, err := tool.NewCapabilityAuthorizer(
		map[string]tool.Capability{"read": tool.ReadCapability, "write": tool.WriteCapability},
		map[string][]tool.Capability{"reader": {tool.ReadCapability}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAuthorizationDeniesWholeBatchBeforeExecution(t *testing.T) {
	read, write := &accessCounter{name: "read"}, &accessCounter{name: "write"}
	m := &accessModel{first: []model.ToolCall{{ID: "r", Name: "read"}, {ID: "w", Name: "write"}}}
	events := &sink{}
	a, err := New(m, map[string]tool.Tool{"read": read, "write": write}, 2, WithVisibleTools(), WithAuthorizer(accessPolicy(t)), WithEventSink(events))
	if err != nil {
		t.Fatal(err)
	}
	ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
	_, err = a.Run(ctx, "model can request hidden tools")
	if !errors.Is(err, tool.ErrForbidden) || read.calls != 0 || write.calls != 0 || m.calls != 1 || len(m.defs) != 0 {
		t.Fatalf("err=%v read=%d write=%d model=%d defs=%v", err, read.calls, write.calls, m.calls, m.defs)
	}
	denied, finished, failed, started := 0, 0, 0, 0
	for _, e := range events.events {
		switch e.Type {
		case EventToolDenied:
			denied++
			if e.ToolCallID != "w" || e.PrincipalID != "reader" || e.Status != "denied" {
				t.Fatalf("%+v", e)
			}
		case EventRunFinished:
			finished++
		case EventRunFailed:
			failed++
		case EventToolStarted:
			started++
		}
	}
	if denied != 1 || finished != 0 || failed != 1 || started != 0 {
		t.Fatalf("events=%+v", events.events)
	}
}

func TestAuthorizationVisibilityIsNotPermission(t *testing.T) {
	read := &accessCounter{name: "read"}
	m := &accessModel{first: []model.ToolCall{{ID: "r", Name: "read", Arguments: json.RawMessage("{}")}}}
	a, err := New(m, map[string]tool.Tool{"read": read}, 2, WithVisibleTools(), WithAuthorizer(accessPolicy(t)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Run(tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"}), "read")
	if err != nil || out != "done" || read.calls != 1 || len(m.defs) != 0 {
		t.Fatalf("%q %v count=%d", out, err, read.calls)
	}
	if len(m.history) != 3 || m.history[2].ToolCallID != "r" || m.history[2].Role != "tool" {
		t.Fatalf("%+v", m.history)
	}
}

type authorizerFunc func(context.Context, tool.Call) error

func (f authorizerFunc) Authorize(ctx context.Context, c tool.Call) error { return f(ctx, c) }

func TestAuthorizationFailureAndArgumentIsolation(t *testing.T) {
	cause := errors.New("private policy backend failure")
	for _, missing := range []bool{false, true} {
		target := &accessCounter{name: "read"}
		m := &accessModel{first: []model.ToolCall{{ID: "r", Name: "read"}}}
		var auth tool.Authorizer = authorizerFunc(func(context.Context, tool.Call) error { return cause })
		if missing {
			auth = accessPolicy(t)
		}
		a, _ := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(auth))
		_, err := a.Run(context.Background(), "")
		if err == nil || target.calls != 0 || m.calls != 1 {
			t.Fatalf("%v %d", err, target.calls)
		}
		if !missing && !errors.Is(err, cause) {
			t.Fatal("lost cause")
		}
		if missing && !errors.Is(err, tool.ErrForbidden) {
			t.Fatal("missing principal not denied")
		}
	}
	target := &accessCounter{name: "read"}
	m := &accessModel{first: []model.ToolCall{{ID: "r", Name: "read", Arguments: json.RawMessage("{}")}}}
	auth := authorizerFunc(func(_ context.Context, c tool.Call) error { c.Arguments[0] = 'x'; return nil })
	a, _ := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(auth))
	if _, err := a.Run(context.Background(), ""); err != nil || target.seen != "{}" {
		t.Fatalf("%v args=%q", err, target.seen)
	}
}

func TestAuthorizationCancellationAndNilOption(t *testing.T) {
	if _, err := New(&accessModel{}, nil, 1, WithAuthorizer(nil)); err == nil {
		t.Fatal("nil authorizer accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := &accessCounter{name: "read"}
	m := &accessModel{first: []model.ToolCall{{ID: "r", Name: "read"}}}
	auth := authorizerFunc(func(context.Context, tool.Call) error { cancel(); return nil })
	a, _ := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(auth))
	_, err := a.Run(ctx, "")
	if !errors.Is(err, context.Canceled) || target.calls != 0 || m.calls != 1 {
		t.Fatalf("%v calls=%d", err, target.calls)
	}
}
