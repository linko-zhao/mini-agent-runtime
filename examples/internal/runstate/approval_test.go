package runstate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

func gate23(t *testing.T, ctx context.Context, id string, paused bool) *runstate.PauseGate {
	t.Helper()
	r, e := runstate.NewRun(ctx, id, "读取 doc-1", 3)
	if e != nil {
		t.Fatal(e)
	}
	g, e := runstate.NewPauseGate(r)
	if e != nil {
		t.Fatal(e)
	}
	if paused {
		if _, e = g.Apply(runstate.Event{Kind: runstate.StartModel}); e != nil {
			t.Fatal(e)
		}
		response := model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}
		if _, e = g.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: &response}); e != nil {
			t.Fatal(e)
		}
		if _, e = g.Pause("pause-1", "waiting-for-host"); e != nil {
			t.Fatal(e)
		}
	}
	return g
}
func decision23(id string, outcome runstate.DecisionOutcome) runstate.ApprovalDecision {
	return runstate.ApprovalDecision{RunID: id, RequestID: "pause-1", Actor: " reviewer-1 ", Outcome: outcome, Reason: " reviewed doc-1 "}
}
func zeroDecision23(t *testing.T, got runstate.ApprovalDecision, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || got != (runstate.ApprovalDecision{}) {
		t.Fatal(got, err, want)
	}
}
func TestDay23RecordKeepsPaused(t *testing.T) {
	for _, outcome := range []runstate.DecisionOutcome{runstate.DecisionApprove, runstate.DecisionReject} {
		t.Run(string(outcome), func(t *testing.T) {
			g := gate23(t, context.Background(), "r23", true)
			before := g.Snapshot()
			pauseBefore, _ := g.CurrentInterrupt()
			if d, ok := g.CurrentDecision(); ok || d != (runstate.ApprovalDecision{}) {
				t.Fatal(d, ok)
			}
			input := decision23("r23", outcome)
			got, e := g.Decide(input)
			if e != nil || got != input {
				t.Fatal(got, e)
			}
			input.Actor = "changed input"
			got.Reason = "changed result"
			current, ok := g.CurrentDecision()
			if !ok || current != decision23("r23", outcome) {
				t.Fatal(current, ok)
			}
			current.Actor = "changed read"
			current, _ = g.CurrentDecision()
			if current != decision23("r23", outcome) {
				t.Fatal(current)
			}
			// Host fixture: no tool effect unless the gate accepts StartTools.
			effects := 0
			for _, event := range []runstate.Event{{Kind: runstate.StartTools}, {Kind: runstate.Cancel}, {Kind: runstate.Abort, Reason: "stop"}} {
				s, err := g.Apply(event)
				if err == nil && event.Kind == runstate.StartTools {
					effects++
				}
				if !errors.Is(err, runstate.ErrPaused) || !reflect.DeepEqual(s, before) {
					t.Fatal(s, err)
				}
			}
			pauseAfter, _ := g.CurrentInterrupt()
			if effects != 0 || !reflect.DeepEqual(before, g.Snapshot()) || !reflect.DeepEqual(pauseBefore, pauseAfter) {
				t.Fatal("decision advanced run or replaced pause")
			}
		})
	}
}
func TestDay23Validation(t *testing.T) {
	invalid := []runstate.ApprovalDecision{}
	for _, field := range []string{"run", "request", "actor", "reason"} {
		for _, value := range []string{"", " \t"} {
			d := decision23("r23", runstate.DecisionApprove)
			switch field {
			case "run":
				d.RunID = value
			case "request":
				d.RequestID = value
			case "actor":
				d.Actor = value
			case "reason":
				d.Reason = value
			}
			invalid = append(invalid, d)
		}
	}
	for _, value := range []runstate.DecisionOutcome{"", "APPROVE", " approve ", "unknown"} {
		d := decision23("r23", value)
		invalid = append(invalid, d)
	}
	g := gate23(t, context.Background(), "r23", true)
	before := g.Snapshot()
	for _, d := range invalid {
		got, e := g.Decide(d)
		zeroDecision23(t, got, e, runstate.ErrInvalidDecision)
	}
	if d, ok := g.CurrentDecision(); ok || d != (runstate.ApprovalDecision{}) {
		t.Fatal(d, ok)
	}
	if !reflect.DeepEqual(before, g.Snapshot()) {
		t.Fatal("invalid decision changed run")
	}
	// Shape validation precedes checking a held pause or prior decision.
	open := gate23(t, context.Background(), "open", false)
	got, e := open.Decide(runstate.ApprovalDecision{})
	zeroDecision23(t, got, e, runstate.ErrInvalidDecision)
	if _, e = g.Decide(decision23("r23", runstate.DecisionApprove)); e != nil {
		t.Fatal(e)
	}
	got, e = g.Decide(runstate.ApprovalDecision{})
	zeroDecision23(t, got, e, runstate.ErrInvalidDecision)
}
func TestDay23BindingAndMissing(t *testing.T) {
	g := gate23(t, context.Background(), "r23", false)
	got, e := g.Decide(decision23("r23", runstate.DecisionApprove))
	zeroDecision23(t, got, e, runstate.ErrNoPause)
	if _, e = g.Apply(runstate.Event{Kind: runstate.Abort, Reason: "finished"}); e != nil {
		t.Fatal(e)
	}
	got, e = g.Decide(decision23("r23", runstate.DecisionApprove))
	zeroDecision23(t, got, e, runstate.ErrNoPause)
	// Two runs can use the same pause ID; binding must include RunID.
	g = gate23(t, context.Background(), "r23", true)
	other := gate23(t, context.Background(), "other", true)
	for _, target := range []*runstate.PauseGate{g, other} {
		d := decision23("r23", runstate.DecisionApprove)
		if target == g {
			d.RequestID = "pause-old"
		}
		got, e = target.Decide(d)
		zeroDecision23(t, got, e, runstate.ErrDecisionMismatch)
		if _, ok := target.CurrentDecision(); ok {
			t.Fatal("mismatch recorded")
		}
	}
	// Matching is exact, not whitespace-normalized, including RunID/RequestID.
	for _, field := range []string{"run", "request"} {
		d := decision23("r23", runstate.DecisionApprove)
		if field == "run" {
			d.RunID = " r23 "
		} else {
			d.RequestID = " pause-1 "
		}
		got, e = g.Decide(d)
		zeroDecision23(t, got, e, runstate.ErrDecisionMismatch)
	}
}
func TestDay23FirstDecisionWins(t *testing.T) {
	g := gate23(t, context.Background(), "r23", true)
	first := decision23("r23", runstate.DecisionReject)
	if _, e := g.Decide(first); e != nil {
		t.Fatal(e)
	}
	for _, d := range []runstate.ApprovalDecision{first, decision23("r23", runstate.DecisionApprove)} {
		got, e := g.Decide(d)
		zeroDecision23(t, got, e, runstate.ErrDecisionRecorded)
	}
	d := decision23("other", runstate.DecisionApprove)
	got, e := g.Decide(d)
	zeroDecision23(t, got, e, runstate.ErrDecisionMismatch)
	current, ok := g.CurrentDecision()
	if !ok || current != first {
		t.Fatal(current, ok)
	}
}

type deadline23 struct{ context.Context }

func (d deadline23) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay23Cancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		for _, recorded := range []bool{false, true} {
			t.Run(fmt.Sprintf("deadline=%v/recorded=%v", deadline, recorded), func(t *testing.T) {
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				var ctx context.Context = base
				want := error(context.Canceled)
				if deadline {
					ctx = deadline23{base}
					want = context.DeadlineExceeded
				}
				g := gate23(t, ctx, "r23", true)
				original := decision23("r23", runstate.DecisionApprove)
				if recorded {
					if _, e := g.Decide(original); e != nil {
						t.Fatal(e)
					}
				}
				cancel()
				got, e := g.Decide(runstate.ApprovalDecision{})
				zeroDecision23(t, got, e, want)
				if d, ok := g.CurrentDecision(); ok != recorded || (recorded && d != original) {
					t.Fatal(d, ok)
				}
				if g.Snapshot().Phase != runstate.ToolsPending {
					t.Fatal("read observed cancellation")
				}
				s, e := g.Apply(runstate.Event{Kind: runstate.StartTools})
				if !errors.Is(e, want) || s.Phase != runstate.Cancelled || len(s.Pending) != 1 {
					t.Fatal(s, e)
				}
				if d, ok := g.CurrentDecision(); ok || d != (runstate.ApprovalDecision{}) {
					t.Fatal(d, ok)
				}
				if _, ok := g.CurrentInterrupt(); ok {
					t.Fatal("pause not cleared")
				}
				got, e = g.Decide(original)
				zeroDecision23(t, got, e, want)
			})
		}
	}
}
func TestDay23ConcurrentFirstWriter(t *testing.T) {
	g := gate23(t, context.Background(), "r23", true)
	before := g.Snapshot()
	var wg sync.WaitGroup
	var wins atomic.Int32
	winner := make(chan runstate.ApprovalDecision, 16)
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			d := decision23("r23", runstate.DecisionApprove)
			if i%2 == 1 {
				d.Outcome = runstate.DecisionReject
			}
			d.Actor = fmt.Sprintf("reviewer-%d", i)
			got, e := g.Decide(d)
			if e == nil {
				wins.Add(1)
				winner <- got
				got.Actor = "caller edit"
			} else if !errors.Is(e, runstate.ErrDecisionRecorded) {
				t.Error(e)
			}
			if current, ok := g.CurrentDecision(); !ok {
				t.Error("missing winner")
			} else {
				current.Reason = "read edit"
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal(wins.Load())
	}
	current, ok := g.CurrentDecision()
	if !ok || current != <-winner {
		t.Fatal(current, ok)
	}
	if !reflect.DeepEqual(before, g.Snapshot()) {
		t.Fatal("decision advanced state")
	}
	if _, e := g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, runstate.ErrPaused) {
		t.Fatal(e)
	}
}
