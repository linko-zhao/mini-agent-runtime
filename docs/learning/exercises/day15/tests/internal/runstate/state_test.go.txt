package runstate_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

func day15New(t *testing.T, max int) runstate.State {
	t.Helper()
	s, err := runstate.New("run-1", "read doc-1", max)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func day15Apply(t *testing.T, s runstate.State, e runstate.Event) runstate.State {
	t.Helper()
	next, err := runstate.Advance(s, e)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func day15Response() *model.Response {
	return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "read-1", Name: "get_document", Arguments: json.RawMessage(`{"id":"doc-1"}`)}}}}
}
func day15Pending(t *testing.T) runstate.State {
	s := day15Apply(t, day15New(t, 2), runstate.Event{Kind: runstate.StartModel})
	return day15Apply(t, s, runstate.Event{Kind: runstate.ModelReturned, Response: day15Response()})
}

func TestDay15New(t *testing.T) {
	s := day15New(t, 2)
	if s.RunID != "run-1" || s.Phase != runstate.Ready || s.Step != 0 || s.MaxSteps != 2 || len(s.Messages) != 1 || s.Messages[0].Role != "user" || s.Messages[0].Content != "read doc-1" || len(s.Pending) != 0 || s.Result != "" || s.Failure != "" {
		t.Fatalf("%+v", s)
	}
	for _, tc := range []struct {
		id  string
		max int
	}{{"", 2}, {" \t", 2}, {"x", 0}, {"x", -1}} {
		if _, err := runstate.New(tc.id, "", tc.max); !errors.Is(err, runstate.ErrTransition) {
			t.Fatal(tc, err)
		}
	}
	if _, err := runstate.New("x", "", 1); err != nil {
		t.Fatal("empty user content is allowed", err)
	}
}
func TestDay15RoundTrip(t *testing.T) {
	s := day15New(t, 2)
	s = day15Apply(t, s, runstate.Event{Kind: runstate.StartModel})
	if s.Phase != runstate.ModelRunning || s.Step != 1 {
		t.Fatal(s)
	}
	s = day15Apply(t, s, runstate.Event{Kind: runstate.ModelReturned, Response: day15Response()})
	if s.Phase != runstate.ToolsPending || len(s.Pending) != 1 || len(s.Messages) != 2 {
		t.Fatal(s)
	}
	s = day15Apply(t, s, runstate.Event{Kind: runstate.StartTools})
	if s.Phase != runstate.ToolsRunning || s.Step != 1 {
		t.Fatal(s)
	}
	s = day15Apply(t, s, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "read-1", Content: "body"}}})
	if s.Phase != runstate.Ready || s.Step != 1 || len(s.Pending) != 0 || len(s.Messages) != 3 {
		t.Fatal(s)
	}
	s = day15Apply(t, s, runstate.Event{Kind: runstate.StartModel})
	s = day15Apply(t, s, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "answer"}}})
	if s.Phase != runstate.Completed || s.Step != 2 || s.Result != "answer" || len(s.Messages) != 4 {
		t.Fatal(s)
	}
}
func TestDay15InvalidEvents(t *testing.T) {
	ready := day15New(t, 2)
	running := day15Apply(t, ready, runstate.Event{Kind: runstate.StartModel})
	pending := day15Pending(t)
	tools := day15Apply(t, pending, runstate.Event{Kind: runstate.StartTools})
	for _, tc := range []struct {
		name string
		s    runstate.State
		e    runstate.Event
	}{
		{"start-tools-too-early", ready, runstate.Event{Kind: runstate.StartTools}},
		{"model-twice", running, runstate.Event{Kind: runstate.StartModel}},
		{"nil-response", running, runstate.Event{Kind: runstate.ModelReturned}},
		{"wrong-role", running, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "user"}}}},
		{"results-before-start", pending, runstate.Event{Kind: runstate.ToolsReturned}},
		{"missing-results", tools, runstate.Event{Kind: runstate.ToolsReturned}},
		{"wrong-result-id", tools, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "wrong"}}}},
		{"wrong-result-role", tools, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "assistant", ToolCallID: "read-1"}}}},
		{"empty-reason", ready, runstate.Event{Kind: runstate.Abort}},
		{"unknown-event", ready, runstate.Event{Kind: "unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.s)
			got, err := runstate.Advance(tc.s, tc.e)
			after, _ := json.Marshal(tc.s)
			if !errors.Is(err, runstate.ErrTransition) || !reflect.DeepEqual(got, runstate.State{}) || string(before) != string(after) {
				t.Fatalf("mutation or accepted event: %+v %v", got, err)
			}
		})
	}
	for _, calls := range [][]model.ToolCall{{{Name: "read"}}, {{ID: "a", Name: " "}}, {{ID: "a", Name: "read"}, {ID: "a", Name: "read"}}} {
		r := &model.Response{Message: model.Message{Role: "assistant", ToolCalls: calls}}
		if _, err := runstate.Advance(running, runstate.Event{Kind: runstate.ModelReturned, Response: r}); !errors.Is(err, runstate.ErrTransition) {
			t.Fatal(err)
		}
	}
	// 两条结果必须与原始请求的顺序和 ID 一一对应。
	r := day15Response()
	r.Message.ToolCalls = append(r.Message.ToolCalls, model.ToolCall{ID: "read-2", Name: "read"})
	two := day15Apply(t, running, runstate.Event{Kind: runstate.ModelReturned, Response: r})
	two = day15Apply(t, two, runstate.Event{Kind: runstate.StartTools})
	reversed := []model.Message{{Role: "tool", ToolCallID: "read-2"}, {Role: "tool", ToolCallID: "read-1"}}
	if _, err := runstate.Advance(two, runstate.Event{Kind: runstate.ToolsReturned, Results: reversed}); !errors.Is(err, runstate.ErrTransition) {
		t.Fatal("accepted reversed results", err)
	}
}
func TestDay15TerminalAndBudget(t *testing.T) {
	ready := day15New(t, 1)
	failed := day15Apply(t, ready, runstate.Event{Kind: runstate.Abort, Reason: "safe model failure"})
	cancelled := day15Apply(t, ready, runstate.Event{Kind: runstate.Cancel})
	running := day15Apply(t, ready, runstate.Event{Kind: runstate.StartModel})
	done := day15Apply(t, running, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}})
	if failed.Failure != "safe model failure" || cancelled.Failure != "cancelled" || cancelled.Phase != runstate.Cancelled {
		t.Fatal(failed, cancelled)
	}
	for _, state := range []runstate.State{failed, cancelled, done} {
		for _, kind := range []runstate.EventKind{runstate.StartModel, runstate.Cancel, runstate.Abort, runstate.StartTools, runstate.ModelReturned, runstate.ToolsReturned} {
			if _, err := runstate.Advance(state, runstate.Event{Kind: kind, Reason: "x"}); !errors.Is(err, runstate.ErrTransition) {
				t.Fatal(state.Phase, kind, err)
			}
		}
	}
	pending := day15Apply(t, running, runstate.Event{Kind: runstate.ModelReturned, Response: day15Response()})
	tools := day15Apply(t, pending, runstate.Event{Kind: runstate.StartTools})
	back := day15Apply(t, tools, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "read-1"}}})
	if _, err := runstate.Advance(back, runstate.Event{Kind: runstate.StartModel}); !errors.Is(err, runstate.ErrMaxSteps) {
		t.Fatal(err)
	}
	if back.Phase != runstate.Ready || back.Step != 1 {
		t.Fatal("budget check mutated input")
	}
	for _, state := range []runstate.State{running, pending, tools} {
		if got, err := runstate.Advance(state, runstate.Event{Kind: runstate.Cancel}); err != nil || got.Phase != runstate.Cancelled {
			t.Fatal(got, err)
		}
	}
}
func TestDay15SnapshotsAndConcurrency(t *testing.T) {
	original := day15New(t, 2)
	running := day15Apply(t, original, runstate.Event{Kind: runstate.StartModel})
	r := day15Response()
	pending := day15Apply(t, running, runstate.Event{Kind: runstate.ModelReturned, Response: r})
	r.Message.ToolCalls[0].Arguments[0] = 'x'
	if !json.Valid(pending.Pending[0].Arguments) || !json.Valid(pending.Messages[1].ToolCalls[0].Arguments) {
		t.Fatal("aliases event")
	}
	next := day15Apply(t, pending, runstate.Event{Kind: runstate.StartTools})
	next.Pending[0].Arguments[0] = 'x'
	next.Messages[1].ToolCalls[0].Arguments[0] = 'x'
	if !json.Valid(pending.Pending[0].Arguments) || !json.Valid(pending.Messages[1].ToolCalls[0].Arguments) {
		t.Fatal("aliases old state")
	}
	copied := day15Apply(t, pending, runstate.Event{Kind: runstate.StartTools})
	copied.Pending[0].Arguments[0] = 'y'
	if !json.Valid(copied.Messages[1].ToolCalls[0].Arguments) {
		t.Fatal("Pending aliases Messages")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := runstate.Advance(pending, runstate.Event{Kind: runstate.StartTools})
			if err != nil {
				t.Error(err)
				return
			}
			s.Pending[0].Arguments[0] = 'z'
		}()
	}
	wg.Wait()
	if !json.Valid(pending.Pending[0].Arguments) {
		t.Fatal("concurrent transition changed original")
	}
}
