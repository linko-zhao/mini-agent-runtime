package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/calculator"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/knowledge"
	"testing"
	"time"
)

type modelFunc func(context.Context, []model.Message) (*model.Response, error)

func (f modelFunc) Generate(c context.Context, m []model.Message) (*model.Response, error) {
	return f(c, m)
}

type sink struct{ events []Event }

func (s *sink) Emit(_ context.Context, e Event) { s.events = append(s.events, e) }
func TestRoundTrip(t *testing.T) {
	n := 0
	s := &sink{}
	m := modelFunc(func(_ context.Context, ms []model.Message) (*model.Response, error) {
		n++
		if n == 1 {
			return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "a", Name: "calculator", Arguments: json.RawMessage(`{"a":0,"b":2}`)}, {ID: "b", Name: "calculator", Arguments: json.RawMessage(`{"a":3,"b":4}`)}}}}, nil
		}
		if len(ms) != 4 || ms[2].ToolCallID != "a" || ms[2].Content != "2" || ms[3].ToolCallID != "b" || ms[3].Content != "7" {
			t.Fatalf("history: %+v", ms)
		}
		return &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}, nil
	})
	a, err := New(m, map[string]tool.Tool{"calculator": &calculator.Calculator{}}, 2, WithEventSink(s))
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Run(context.Background(), "input")
	if err != nil || out != "done" {
		t.Fatalf("%q %v", out, err)
	}
	if s.events[0].Type != EventRunStarted || s.events[len(s.events)-1].Type != EventRunFinished {
		t.Fatal(s.events)
	}
}

type waitTool struct{ calls int }

func (*waitTool) Name() string { return "wait" }
func (w *waitTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	w.calls++
	<-ctx.Done()
	return "", ctx.Err()
}
func TestTimeoutAndBatchValidation(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		w := &waitTool{}
		calls := []model.ToolCall{{ID: "a", Name: "wait"}}
		if unknown {
			calls = append(calls, model.ToolCall{ID: "b", Name: "missing"})
		}
		m := model.NewMock([]model.Response{{Message: model.Message{Role: "assistant", ToolCalls: calls}}})
		a, err := New(m, map[string]tool.Tool{"wait": w}, 2, WithToolTimeout(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.Run(context.Background(), "")
		if unknown {
			if err == nil || w.calls != 0 {
				t.Fatalf("partial execution: %v %d", err, w.calls)
			}
		} else if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	}
}
func TestTermination(t *testing.T) {
	m := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
		return nil, errors.New("provider failure")
	})
	a, _ := New(m, nil, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Run(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := a.Run(context.Background(), ""); err == nil {
		t.Fatal("missing model error")
	}
}

type bindingModel struct{ defs []model.ToolDefinition }

func (m *bindingModel) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	m.defs = defs
	return m, nil
}
func (m *bindingModel) Generate(context.Context, []model.Message) (*model.Response, error) {
	return &model.Response{Message: model.Message{Role: "assistant", Content: "ok"}}, nil
}
func TestVisibleToolsOnlyFiltersDefinitions(t *testing.T) {
	registry := tool.NewRegistry()
	ctx := context.Background()
	store := knowledge.NewFakeStore(nil)
	if err := registry.Register(ctx, &knowledge.SearchKnowledge{Store: store}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, &knowledge.GetDocument{Store: store}); err != nil {
		t.Fatal(err)
	}
	m := &bindingModel{}
	a, err := New(m, registry.Tools(), 2, WithVisibleTools("search_knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.defs) != 1 || m.defs[0].Name != "search_knowledge" {
		t.Fatal(m.defs)
	}
	if _, ok := a.tools["get_document"]; !ok {
		t.Fatal("visibility removed registered capability")
	}
	if _, err := New(m, registry.Tools(), 2, WithVisibleTools()); err != nil || len(m.defs) != 0 {
		t.Fatalf("empty visibility: %+v %v", m.defs, err)
	}
	if _, err := New(m, registry.Tools(), 2, WithVisibleTools("missing")); err == nil {
		t.Fatal("unknown visible tool accepted")
	}
}
