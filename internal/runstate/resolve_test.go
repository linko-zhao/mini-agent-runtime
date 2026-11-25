package runstate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

func gate24(t *testing.T, ctx context.Context, pause bool) *runstate.PauseGate {
	t.Helper()
	r, e := runstate.NewRun(ctx, "r24", "读取 doc-1", 3)
	if e != nil {
		t.Fatal(e)
	}
	g, e := runstate.NewPauseGate(r)
	if e != nil {
		t.Fatal(e)
	}
	if pause {
		pending24(t, g, "call-1", "pause-1")
	}
	return g
}
func apply24(t *testing.T, g *runstate.PauseGate, e runstate.Event) runstate.State {
	t.Helper()
	s, err := g.Apply(e)
	if err != nil {
		t.Fatal(s, err)
	}
	return s
}
func pending24(t *testing.T, g *runstate.PauseGate, callID, pauseID string) {
	t.Helper()
	apply24(t, g, runstate.Event{Kind: runstate.StartModel})
	response := model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: callID, Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}
	apply24(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &response})
	if _, e := g.Pause(pauseID, "waiting-for-host"); e != nil {
		t.Fatal(e)
	}
}
func decide24(t *testing.T, g *runstate.PauseGate, id string, outcome runstate.DecisionOutcome) runstate.ApprovalDecision {
	t.Helper()
	d, e := g.Decide(runstate.ApprovalDecision{RunID: "r24", RequestID: id, Actor: "reviewer-1", Outcome: outcome, Reason: " reviewed doc-1 "})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func cleared24(t *testing.T, g *runstate.PauseGate) {
	t.Helper()
	if p, ok := g.CurrentInterrupt(); ok || !reflect.DeepEqual(p, runstate.PauseRequest{}) {
		t.Fatal(p, ok)
	}
	if d, ok := g.CurrentDecision(); ok || d != (runstate.ApprovalDecision{}) {
		t.Fatal(d, ok)
	}
}

// A host effect is permitted only by this successful resolution's phase.
func effect24(g *runstate.PauseGate, calls *atomic.Int32) (runstate.State, error) {
	s, e := g.ResolveDecision("r24", "pause-1")
	if e == nil && s.Phase == runstate.ToolsRunning {
		calls.Add(1)
	}
	return s, e
}
func TestDay24ApproveContinues(t *testing.T) {
	g := gate24(t, context.Background(), true)
	before := g.Snapshot()
	old, _ := g.CurrentInterrupt()
	decision := decide24(t, g, "pause-1", runstate.DecisionApprove)
	var effects atomic.Int32
	s, e := effect24(g, &effects)
	if e != nil || s.Phase != runstate.ToolsRunning || effects.Load() != 1 {
		t.Fatal(s, e, effects.Load())
	}
	want := before
	want.Phase = runstate.ToolsRunning
	if !reflect.DeepEqual(s, want) {
		t.Fatal("resolution changed payload/step/history", s, want)
	}
	cleared24(t, g)
	if old.Checkpoint.State.Phase != runstate.ToolsPending || decision.Outcome != runstate.DecisionApprove {
		t.Fatal("old values changed")
	}
	s, e = effect24(g, &effects)
	if !errors.Is(e, runstate.ErrNoPause) || s.Phase != runstate.ToolsRunning || effects.Load() != 1 {
		t.Fatal(s, e, effects.Load())
	}
	if _, e = g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, runstate.ErrTransition) {
		t.Fatal(e)
	}
	// Preserve call association and return the tool message to the next model round.
	if _, e = g.Apply(runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "wrong", Content: "doc"}}}); !errors.Is(e, runstate.ErrTransition) {
		t.Fatal(e)
	}
	s = apply24(t, g, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "doc-1 body"}}})
	if s.Phase != runstate.Ready || s.Pending != nil || s.Messages[len(s.Messages)-1].Content != "doc-1 body" {
		t.Fatal(s)
	}
	apply24(t, g, runstate.Event{Kind: runstate.StartModel})
	s = apply24(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "摘要"}}})
	if s.Phase != runstate.Completed || s.Result != "摘要" || s.Step != 2 {
		t.Fatal(s)
	}
}
func TestDay24RejectTerminates(t *testing.T) {
	g := gate24(t, context.Background(), true)
	before := g.Snapshot()
	d := decide24(t, g, "pause-1", runstate.DecisionReject)
	var effects atomic.Int32
	s, e := effect24(g, &effects)
	if e != nil || s.Phase != runstate.Failed || s.Failure != d.Reason || effects.Load() != 0 {
		t.Fatal(s, e, effects.Load())
	}
	want := before
	want.Phase = runstate.Failed
	want.Failure = d.Reason
	if !reflect.DeepEqual(s, want) {
		t.Fatal(s, want)
	}
	cleared24(t, g)
	if _, e = g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, runstate.ErrTransition) {
		t.Fatal(e)
	}
	if s, e = g.ResolveDecision("r24", "pause-1"); !errors.Is(e, runstate.ErrNoPause) || s.Phase != runstate.Failed {
		t.Fatal(s, e)
	}
}
func TestDay24Preconditions(t *testing.T) {
	g := gate24(t, context.Background(), true)
	before := g.Snapshot()
	p, _ := g.CurrentInterrupt()
	rows := []struct {
		run, id string
		want    error
	}{{"", "pause-1", runstate.ErrInvalidDecision}, {"r24", " \t", runstate.ErrInvalidDecision}, {"other", "pause-1", runstate.ErrDecisionMismatch}, {"r24", "old", runstate.ErrDecisionMismatch}, {" r24 ", "pause-1", runstate.ErrDecisionMismatch}, {"r24", "pause-1", runstate.ErrDecisionRequired}}
	for _, row := range rows {
		s, e := g.ResolveDecision(row.run, row.id)
		if !errors.Is(e, row.want) || !reflect.DeepEqual(s, before) {
			t.Fatal(s, e, row.want, row)
		}
		s.Pending[0].Arguments[0] = 'x'
		s.Messages[1].ToolCalls[0].Arguments[0] = 'y'
		if !reflect.DeepEqual(g.Snapshot(), before) {
			t.Fatal("error state aliases run")
		}
	}
	after, _ := g.CurrentInterrupt()
	if !reflect.DeepEqual(p, after) {
		t.Fatal("invalid input consumed request")
	}
	if _, ok := g.CurrentDecision(); ok {
		t.Fatal("created decision")
	}
	decide24(t, g, "pause-1", runstate.DecisionApprove)
	if _, e := g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, runstate.ErrPaused) {
		t.Fatal(e)
	}
	open := gate24(t, context.Background(), false)
	if _, e := open.ResolveDecision("", " "); !errors.Is(e, runstate.ErrInvalidDecision) {
		t.Fatal(e)
	}
	if s, e := open.ResolveDecision("other", "old"); !errors.Is(e, runstate.ErrNoPause) || s.Phase != runstate.Ready {
		t.Fatal(s, e)
	}
}

type deadline24 struct{ context.Context }

func (d deadline24) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay24Cancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		for _, recorded := range []bool{false, true} {
			t.Run(fmt.Sprintf("deadline=%v/recorded=%v", deadline, recorded), func(t *testing.T) {
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				var ctx context.Context = base
				want := error(context.Canceled)
				if deadline {
					ctx = deadline24{base}
					want = context.DeadlineExceeded
				}
				g := gate24(t, ctx, true)
				old, _ := g.CurrentInterrupt()
				if recorded {
					decide24(t, g, "pause-1", runstate.DecisionApprove)
				}
				cancel()
				s, e := g.ResolveDecision("", "") // observed cancellation precedes input checks
				if !errors.Is(e, want) || s.Phase != runstate.Cancelled || s.Failure != "cancelled" || len(s.Pending) != 1 {
					t.Fatal(s, e)
				}
				cleared24(t, g)
				if old.Checkpoint.State.Phase != runstate.ToolsPending {
					t.Fatal("old request mutated")
				}
				if s, e = g.ResolveDecision("r24", "pause-1"); !errors.Is(e, runstate.ErrTransition) || s.Phase != runstate.Cancelled {
					t.Fatal(s, e)
				}
			})
		}
	}
	// Even without a pause, observed cancellation is delegated to Run.Apply.
	ctx, cancel := context.WithCancel(context.Background())
	g := gate24(t, ctx, false)
	cancel()
	if s, e := g.ResolveDecision("", ""); !errors.Is(e, context.Canceled) || s.Phase != runstate.Cancelled {
		t.Fatal(s, e)
	}
	// Terminal-first behavior preserves a preexisting Completed result.
	ctx, cancel = context.WithCancel(context.Background())
	g = gate24(t, ctx, false)
	apply24(t, g, runstate.Event{Kind: runstate.StartModel})
	apply24(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}})
	cancel()
	if s, e := g.ResolveDecision("r24", "pause-1"); !errors.Is(e, runstate.ErrTransition) || s.Phase != runstate.Completed || s.Result != "done" {
		t.Fatal(s, e)
	}
}
func TestDay24ConcurrentConsumption(t *testing.T) {
	g := gate24(t, context.Background(), true)
	decide24(t, g, "pause-1", runstate.DecisionApprove)
	before := g.Snapshot()
	var wg sync.WaitGroup
	var wins, effects atomic.Int32
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, e := effect24(g, &effects)
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, runstate.ErrNoPause) {
				t.Error(e)
			}
			if s.Phase != runstate.ToolsRunning {
				t.Error(s)
			}
			s.Pending[0].Arguments[0] = 'x'
			s.Messages[1].ToolCalls[0].Arguments[0] = 'y'
		}()
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 || effects.Load() != 1 {
		t.Fatal(wins.Load(), effects.Load())
	}
	want := before
	want.Phase = runstate.ToolsRunning
	if !reflect.DeepEqual(want, g.Snapshot()) {
		t.Fatal("returned state aliases run")
	}
	cleared24(t, g)
}
func TestDay24StaleRequest(t *testing.T) {
	g := gate24(t, context.Background(), true)
	decide24(t, g, "pause-1", runstate.DecisionApprove)
	if _, e := g.ResolveDecision("r24", "pause-1"); e != nil {
		t.Fatal(e)
	}
	apply24(t, g, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "doc-1 body"}}})
	pending24(t, g, "call-2", "pause-2")
	decide24(t, g, "pause-2", runstate.DecisionApprove)
	before := g.Snapshot()
	if s, e := g.ResolveDecision("r24", "pause-1"); !errors.Is(e, runstate.ErrDecisionMismatch) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, e)
	}
	if d, ok := g.CurrentDecision(); !ok || d.RequestID != "pause-2" {
		t.Fatal(d, ok)
	}
	if s, e := g.ResolveDecision("r24", "pause-2"); e != nil || s.Phase != runstate.ToolsRunning || s.Pending[0].ID != "call-2" {
		t.Fatal(s, e)
	}
}
func TestDay24ResolveVersusCancellation(t *testing.T) {
	for i := 0; i < 32; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		g := gate24(t, ctx, true)
		decide24(t, g, "pause-1", runstate.DecisionApprove)
		start := make(chan struct{})
		done := make(chan struct{})
		go func() { <-start; cancel(); close(done) }()
		close(start)
		s, e := g.ResolveDecision("r24", "pause-1")
		<-done
		if e == nil {
			if s.Phase != runstate.ToolsRunning {
				t.Fatal(s)
			}
		} else if !errors.Is(e, context.Canceled) || s.Phase != runstate.Cancelled {
			t.Fatal(s, e)
		}
		cleared24(t, g)
		if s.Phase == runstate.ToolsRunning {
			s, e = g.Apply(runstate.Event{Kind: runstate.StartTools})
			if !errors.Is(e, context.Canceled) || s.Phase != runstate.Cancelled {
				t.Fatal(s, e)
			}
		}
	}
}
