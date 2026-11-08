package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

type stateTool struct {
	name string
	run  func(context.Context, json.RawMessage) (string, error)
}

func (t stateTool) Name() string { return t.name }
func (t stateTool) Execute(ctx context.Context, b json.RawMessage) (string, error) {
	return t.run(ctx, b)
}

type stateAuth func(context.Context, tool.Call) error

func (f stateAuth) Authorize(ctx context.Context, c tool.Call) error { return f(ctx, c) }

func TestStateTransitionGuards(t *testing.T) {
	record, err := newRunRecord("input")
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(record.transition(RunCompleted), ErrInvalidTransition) || record.data.Status != RunCreated {
		t.Fatal("created completed without start")
	}
	if err = record.transition(RunRunning); err != nil {
		t.Fatal(err)
	}
	base, err := record.plan([]model.ToolCall{{ID: "same", Name: "echo"}, {ID: "same", Name: "echo"}}, 1)
	if err != nil || base != 0 || len(record.data.Calls) != 2 || record.data.Calls[0].Index == record.data.Calls[1].Index {
		t.Fatal("lost invalid duplicate-ID positions")
	}
	if err = record.completeCall(0, &model.Message{Role: "tool", ToolCallID: "same"}, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("completed before start")
	}
	if err = record.startCall(0); err != nil {
		t.Fatal(err)
	}
	if err = record.startCall(0); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("started twice")
	}
	if err = record.completeCall(0, &model.Message{Role: "tool", ToolCallID: "wrong"}, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("accepted wrong result ID")
	}
	if err = record.completeCall(0, &model.Message{Role: "tool", ToolCallID: "same", Content: "ok"}, nil); err != nil {
		t.Fatal(err)
	}
	if err = record.completeCall(0, nil, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("completed twice")
	}
	if err = record.finish(nil); !errors.Is(err, ErrInvalidTransition) || record.data.Status != RunFailed || record.data.Calls[1].Status != CallBlocked {
		t.Fatalf("unfinished run: %+v %v", record.data, err)
	}
	if err = record.transition(RunRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("restarted terminal state")
	}
	for _, terminal := range []RunStatus{RunCompleted, RunFailed, RunCancelled} {
		if CanTransition(terminal, RunRunning) || CanTransition(terminal, terminal) {
			t.Fatal("terminal transition allowed")
		}
	}
	if CanTransition(RunStatus("unknown"), RunRunning) {
		t.Fatal("unknown state accepted")
	}
}

func TestRunStateCompletedWithToolError(t *testing.T) {
	n := 0
	scripted := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
		n++
		if n == 1 {
			return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "bad-args", Name: "echo", Arguments: json.RawMessage("{}")}}}}, nil
		}
		return &model.Response{Message: model.Message{Role: "assistant", Content: "fallback"}}, nil
	})
	target := stateTool{"echo", func(context.Context, json.RawMessage) (string, error) {
		return "", &tool.Error{Kind: tool.ValidationError, Message: "bad input"}
	}}
	a, err := New(scripted, map[string]tool.Tool{"echo": target}, 3)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.RunWithState(context.Background(), "input")
	s := result.Snapshot
	if err != nil || result.Output != "fallback" || s.RunID == "" || s.Status != RunCompleted || s.Step != 2 || s.FailureClass != "" || len(s.Messages) != 4 || len(s.Calls) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	c := s.Calls[0]
	if c.Status != CallErrored || c.Result == nil || c.Result.Content != "invalid_arguments: bad input" || c.Result.ToolCallID != "bad-args" {
		t.Fatalf("%+v", c)
	}
}

func TestRunStatePreflightAndControlErrors(t *testing.T) {
	for _, denied := range []bool{false, true} {
		calls := 0
		batch := []model.ToolCall{{ID: "x", Name: "echo"}, {ID: "x", Name: "echo"}}
		var opts []Option
		if denied {
			batch = batch[:1]
			opts = append(opts, WithAuthorizer(stateAuth(func(context.Context, tool.Call) error { return tool.ErrForbidden })))
		}
		m := model.NewMock([]model.Response{{Message: model.Message{Role: "assistant", ToolCalls: batch}}})
		target := stateTool{"echo", func(context.Context, json.RawMessage) (string, error) { calls++; return "ok", nil }}
		a, err := New(m, map[string]tool.Tool{"echo": target}, 2, opts...)
		if err != nil {
			t.Fatal(err)
		}
		r, err := a.RunWithState(context.Background(), "private input")
		if err == nil || r.Snapshot.Status != RunFailed || calls != 0 || len(r.Snapshot.Calls) != len(batch) {
			t.Fatalf("%+v %v calls=%d", r, err, calls)
		}
		if denied && !errors.Is(err, tool.ErrForbidden) {
			t.Fatal(err)
		}
		for _, c := range r.Snapshot.Calls {
			if c.Status != CallBlocked || c.Result != nil {
				t.Fatal(c)
			}
		}
	}
	failed := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
		return nil, errors.New("PRIVATE-PROVIDER")
	})
	a, _ := New(failed, nil, 2)
	r, err := a.RunWithState(context.Background(), "input")
	if err == nil || r.Snapshot.Status != RunFailed || r.Snapshot.FailureClass != "run_error" {
		t.Fatalf("%+v %v", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err = a.RunWithState(ctx, "input")
	if !errors.Is(err, context.Canceled) || r.Snapshot.Status != RunCancelled || r.Snapshot.FailureClass != "cancelled" {
		t.Fatalf("%+v %v", r, err)
	}
	looping := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
		return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "x", Name: "echo"}}}}, nil
	})
	a, _ = New(looping, map[string]tool.Tool{"echo": stateTool{"echo", func(context.Context, json.RawMessage) (string, error) { return "ok", nil }}}, 1)
	r, err = a.RunWithState(context.Background(), "input")
	if !errors.Is(err, ErrMaxSteps) || r.Snapshot.Status != RunFailed || r.Snapshot.FailureClass != "max_steps" || r.Snapshot.Calls[0].Status != CallReturned {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestRunStatePartialBatchPreservesAcceptedResult(t *testing.T) {
	third := 0
	batch := []model.ToolCall{{ID: "first", Name: "first"}, {ID: "wait", Name: "wait"}, {ID: "third", Name: "third"}}
	m := model.NewMock([]model.Response{{Message: model.Message{Role: "assistant", ToolCalls: batch}}})
	tools := map[string]tool.Tool{
		"first": stateTool{"first", func(context.Context, json.RawMessage) (string, error) { return "accepted result", nil }},
		"wait":  stateTool{"wait", func(ctx context.Context, _ json.RawMessage) (string, error) { <-ctx.Done(); return "late", ctx.Err() }},
		"third": stateTool{"third", func(context.Context, json.RawMessage) (string, error) { third++; return "unexpected", nil }},
	}
	a, err := New(m, tools, 2, WithToolTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.RunWithState(context.Background(), "input")
	s := r.Snapshot
	if !errors.Is(err, context.DeadlineExceeded) || s.Status != RunCancelled || s.FailureClass != "deadline_exceeded" || len(s.Calls) != 3 || third != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if s.Calls[0].Status != CallReturned || s.Calls[0].Result == nil || s.Calls[0].Result.Content != "accepted result" {
		t.Fatal(s.Calls[0])
	}
	if s.Calls[1].Status != CallUnconfirmed || s.Calls[1].Result != nil || s.Calls[2].Status != CallBlocked {
		t.Fatal(s.Calls)
	}
	// Preserve old batch semantics: no partial tool messages were fed back.
	if len(s.Messages) != 2 || s.Messages[1].Role != "assistant" {
		t.Fatal(s.Messages)
	}
}

func TestSnapshotOwnershipAndConcurrentRuns(t *testing.T) {
	shared := []model.ToolCall{{ID: "x", Name: "echo", Arguments: json.RawMessage(`{"x":1}`)}}
	n := 0
	m := modelFunc(func(context.Context, []model.Message) (*model.Response, error) {
		n++
		if n == 1 {
			return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: shared}}, nil
		}
		return &model.Response{Message: model.Message{Role: "assistant", Content: "ok"}}, nil
	})
	tools := map[string]tool.Tool{"echo": stateTool{"echo", func(context.Context, json.RawMessage) (string, error) { return "accepted", nil }}}
	a, _ := New(m, tools, 3)
	r, err := a.RunWithState(context.Background(), "original")
	if err != nil {
		t.Fatal(err)
	}
	shared[0].Arguments[0] = 'x'
	shared[0].Name = "changed"
	copy := r.Snapshot.Clone()
	copy.Messages[1].ToolCalls[0].Arguments[0] = 'z'
	copy.Messages[0].Content = "changed"
	copy.Calls[0].Result.Content = "changed"
	if !json.Valid(r.Snapshot.Messages[1].ToolCalls[0].Arguments) || r.Snapshot.Messages[1].ToolCalls[0].Name != "echo" || r.Snapshot.Messages[0].Content != "original" || r.Snapshot.Calls[0].Result.Content != "accepted" {
		t.Fatal("snapshot aliases another owner")
	}

	// Unlike a cursor-based Mock, this model and tool are safe to share.
	stateless := modelFunc(func(_ context.Context, ms []model.Message) (*model.Response, error) {
		if len(ms) == 1 {
			return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "x", Name: "echo"}}}}, nil
		}
		return &model.Response{Message: model.Message{Role: "assistant", Content: ms[0].Content}}, nil
	})
	a, _ = New(stateless, tools, 3)
	type outcome struct {
		result RunResult
		err    error
		input  string
	}
	done := make(chan outcome, 8)
	for i := 0; i < 8; i++ {
		go func(input string) {
			result, err := a.RunWithState(context.Background(), input)
			done <- outcome{result, err, input}
		}(fmt.Sprintf("user-%d", i))
	}
	ids := map[string]bool{}
	for i := 0; i < 8; i++ {
		got := <-done
		s := got.result.Snapshot
		if got.err != nil || got.result.Output != got.input || s.Messages[0].Content != got.input || s.Status != RunCompleted || s.RunID == "" || ids[s.RunID] {
			t.Fatalf("%+v", got)
		}
		ids[s.RunID] = true
	}
}
