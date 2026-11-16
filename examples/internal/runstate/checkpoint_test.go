package runstate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

func run17(t *testing.T, ctx context.Context) *runstate.Run {
	t.Helper()
	r, err := runstate.NewRun(ctx, "run-17", "读取 doc-1", 100)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func event17(t *testing.T, r *runstate.Run, e runstate.Event) {
	t.Helper()
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}
}
func response17(id string) *model.Response {
	return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: id, Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}
}
func checkpoint17(t *testing.T, r *runstate.Run, b runstate.CheckpointBoundary) runstate.Checkpoint {
	t.Helper()
	before := r.Snapshot()
	cp, err := r.Checkpoint()
	if err != nil || cp.Boundary != b || !reflect.DeepEqual(before, cp.State) || !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal(cp, err)
	}
	return cp
}
func reject17(t *testing.T, r *runstate.Run) {
	t.Helper()
	before := r.Snapshot()
	cp, err := r.Checkpoint()
	if !errors.Is(err, runstate.ErrCheckpointBoundary) || !reflect.DeepEqual(cp, runstate.Checkpoint{}) || !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal(cp, err)
	}
}
func TestDay17Boundaries(t *testing.T) {
	r := run17(t, context.Background())
	cp := checkpoint17(t, r, runstate.CheckpointReady)
	if cp.State.Step != 0 || len(cp.State.Messages) != 1 || cp.State.MaxSteps != 100 {
		t.Fatal(cp)
	}
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	reject17(t, r)
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response17("call-1")})
	cp = checkpoint17(t, r, runstate.CheckpointBeforeTools)
	if cp.State.Phase != runstate.ToolsPending || cp.State.Pending[0].ID != "call-1" || len(cp.State.Messages) != 2 {
		t.Fatal(cp)
	}
	event17(t, r, runstate.Event{Kind: runstate.StartTools})
	reject17(t, r)
	event17(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "正文"}}})
	cp = checkpoint17(t, r, runstate.CheckpointReady)
	if cp.State.Step != 1 || len(cp.State.Messages) != 3 || len(cp.State.Pending) != 0 {
		t.Fatal(cp)
	}
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "已读取"}}})
	cp = checkpoint17(t, r, runstate.CheckpointTerminal)
	if cp.State.Phase != runstate.Completed || cp.State.Result != "已读取" || cp.State.Step != 2 || len(cp.State.Messages) != 4 {
		t.Fatal(cp)
	}
	if _, err := r.Apply(runstate.Event{Kind: runstate.StartModel}); !errors.Is(err, runstate.ErrTransition) {
		t.Fatal(err)
	}
}
func TestDay17Ownership(t *testing.T) {
	r := run17(t, context.Background())
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	resp := response17("call-1")
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: resp})
	cp := checkpoint17(t, r, runstate.CheckpointBeforeTools)
	original := cp.Clone()
	resp.Message.ToolCalls[0].Arguments[0] = 'x'
	cp.State.Messages[1].ToolCalls[0].Arguments[0] = 'm'
	if cp.State.Pending[0].Arguments[0] != '{' {
		t.Fatal("history and Pending share bytes")
	}
	cp.State.Pending[0].Arguments[0] = 'p'
	cp.State.RunID = "local"
	if !reflect.DeepEqual(original.State, r.Snapshot()) {
		t.Fatal("checkpoint aliases Run")
	}
	if original.State.Pending[0].Arguments[0] != '{' || original.State.Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("Clone aliases source")
	}
	event17(t, r, runstate.Event{Kind: runstate.StartTools})
	if original.State.Phase != runstate.ToolsPending {
		t.Fatal("later update changed checkpoint")
	}
	var zero runstate.Checkpoint
	if !reflect.DeepEqual(zero, zero.Clone()) {
		t.Fatal("zero clone")
	}
}
func TestDay17CancellationAndTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := run17(t, ctx)
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response17("call-1")})
	cancel()
	// Capture is a read; unlike Apply, it does not observe cancellation.
	checkpoint17(t, r, runstate.CheckpointBeforeTools)
	if _, err := r.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cp := checkpoint17(t, r, runstate.CheckpointTerminal)
	if cp.State.Phase != runstate.Cancelled || cp.State.Failure != "cancelled" || len(cp.State.Pending) != 1 {
		t.Fatal(cp)
	}
	// Terminal capture retains unconfirmed work but never reopens the Run.
	for _, kind := range []runstate.EventKind{runstate.Abort, runstate.Cancel} {
		other := run17(t, context.Background())
		event17(t, other, runstate.Event{Kind: runstate.StartModel})
		event17(t, other, runstate.Event{Kind: runstate.ModelReturned, Response: response17("pending")})
		event17(t, other, runstate.Event{Kind: runstate.StartTools})
		event17(t, other, runstate.Event{Kind: kind, Reason: "controlled_failure"})
		terminal := checkpoint17(t, other, runstate.CheckpointTerminal)
		if len(terminal.State.Pending) != 1 || len(terminal.State.Messages) != 2 {
			t.Fatal(terminal)
		}
		if kind == runstate.Abort && (terminal.State.Phase != runstate.Failed || terminal.State.Failure != "controlled_failure") {
			t.Fatal(terminal)
		}
		if kind == runstate.Cancel && terminal.State.Phase != runstate.Cancelled {
			t.Fatal(terminal)
		}
	}
}
func TestDay17HistoricalCandidate(t *testing.T) {
	r, err := runstate.NewRun(context.Background(), "budget-17", "读取 doc-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response17("call-1")})
	candidate := checkpoint17(t, r, runstate.CheckpointBeforeTools)
	// This models a host continuing after capture. No store or pause exists.
	event17(t, r, runstate.Event{Kind: runstate.StartTools})
	reject17(t, r)
	event17(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "accepted"}}})
	if candidate.State.Phase != runstate.ToolsPending || len(candidate.State.Messages) != 2 || r.Snapshot().Phase != runstate.Ready {
		t.Fatal("candidate was not historical")
	}
	// Editing a candidate grants no state update or retry capability.
	candidate.State.Phase = runstate.Completed
	if r.Snapshot().Phase != runstate.Ready {
		t.Fatal("candidate can alter Run")
	}
	if _, err = r.Apply(runstate.Event{Kind: runstate.StartModel}); !errors.Is(err, runstate.ErrMaxSteps) {
		t.Fatal(err)
	}
	checkpoint17(t, r, runstate.CheckpointReady)
}
func TestDay17ConcurrentCapture(t *testing.T) {
	r := run17(t, context.Background())
	start := make(chan struct{})
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 250; i++ {
				cp, err := r.Checkpoint()
				if err != nil {
					if !errors.Is(err, runstate.ErrCheckpointBoundary) {
						t.Error(err)
					}
					runtime.Gosched()
					continue
				}
				switch cp.Boundary {
				case runstate.CheckpointReady:
					if cp.State.Phase != runstate.Ready || len(cp.State.Pending) != 0 || len(cp.State.Messages) != 1+2*cp.State.Step {
						t.Error("mixed Ready snapshot", cp)
					}
				case runstate.CheckpointBeforeTools:
					if cp.State.Phase != runstate.ToolsPending || len(cp.State.Pending) != 1 || len(cp.State.Messages) != 2*cp.State.Step || cp.State.Pending[0].ID != fmt.Sprintf("call-%d", cp.State.Step) {
						t.Error("mixed tools snapshot", cp)
					}
				case runstate.CheckpointTerminal:
					if cp.State.Phase != runstate.Completed || len(cp.State.Messages) != 2*cp.State.Step {
						t.Error("mixed terminal snapshot", cp)
					}
				default:
					t.Error("unknown boundary", cp)
				}
				cp.State.Messages[0].Content = "reader-owned"
				runtime.Gosched()
			}
		}()
	}
	close(start)
	for i := 1; i <= 30; i++ {
		id := fmt.Sprintf("call-%d", i)
		event17(t, r, runstate.Event{Kind: runstate.StartModel})
		event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response17(id)})
		event17(t, r, runstate.Event{Kind: runstate.StartTools})
		event17(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: id, Content: "ok"}}})
	}
	event17(t, r, runstate.Event{Kind: runstate.StartModel})
	event17(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}})
	wg.Wait()
	cp := checkpoint17(t, r, runstate.CheckpointTerminal)
	if cp.State.Step != 31 || cp.State.Messages[0].Content != "读取 doc-1" {
		t.Fatal(cp)
	}
}
