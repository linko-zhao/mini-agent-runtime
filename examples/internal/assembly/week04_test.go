package assembly_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/assembly"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

type source28 struct {
	calls  atomic.Int32
	lookup func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error)
}

func (s *source28) Lookup(ctx context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
	s.calls.Add(1)
	return s.lookup(ctx, a)
}
func known28(a runstate.ToolAttempt) runstate.ToolOutcomeEvidence {
	c := a.Call
	c.Arguments = append([]byte(nil), c.Arguments...)
	return runstate.ToolOutcomeEvidence{RunID: a.RunID, Step: a.Step, Evidence: runstate.ReplayEvidence{Call: c, Basis: runstate.BasisResultKnown, Result: &model.Message{Role: "tool", ToolCallID: c.ID, Content: "body " + c.ID}}}
}
func results28(calls []model.ToolCall) []model.Message {
	out := make([]model.Message, len(calls))
	for i, c := range calls {
		out[i] = model.Message{Role: "tool", ToolCallID: c.ID, Content: "body " + c.ID}
	}
	return out
}
func decision28(outcome runstate.DecisionOutcome) runstate.ApprovalDecision {
	return runstate.ApprovalDecision{RunID: "r28", RequestID: "pause-1", Actor: "reviewer", Outcome: outcome, Reason: "reviewed"}
}
func apply28(t *testing.T, g *runstate.PauseGate, e runstate.Event) runstate.State {
	t.Helper()
	s, err := g.Apply(e)
	if err != nil {
		t.Fatal(s, err)
	}
	return s
}
func host28(t *testing.T, ctx context.Context, execute assembly.ToolBatchExecutor) *assembly.Week04Host {
	t.Helper()
	h, e := assembly.BuildWeek04(ctx, "r28", "读取 doc-1 和 doc-2", 4, execute)
	if e != nil {
		t.Fatal(e)
	}
	g := h.Gate()
	apply28(t, g, runstate.Event{Kind: runstate.StartModel})
	apply28(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}, {ID: "call-2", Name: "get_document", Arguments: []byte(`{"id":"doc-2"}`)}}}}})
	if _, e = g.Pause("pause-1", "review"); e != nil {
		t.Fatal(e)
	}
	return h
}
func TestDay28Week04Matrix(t *testing.T) {
	executionErr := errors.New("response missing")
	sourceErr := errors.New("lookup unavailable")
	rows := []struct {
		name                                           string
		reject, executionError                         bool
		basis                                          runstate.ReplayBasis
		sourceError, queryCancel, badResults, noSource bool
		phase                                          runstate.Phase
		want                                           error
		lookups                                        int32
		reconciled                                     bool
	}{
		{name: "reject", reject: true, phase: runstate.Failed},
		{name: "normal", phase: runstate.Ready},
		{name: "known_after_error", executionError: true, basis: runstate.BasisResultKnown, phase: runstate.Ready, lookups: 2, reconciled: true},
		{name: "unknown", executionError: true, basis: runstate.BasisUnknown, phase: runstate.ToolsRunning, want: runstate.ErrOutcomeIncomplete, lookups: 2},
		{name: "not_started", executionError: true, basis: runstate.BasisNotStarted, phase: runstate.ToolsRunning, want: runstate.ErrOutcomeIncomplete, lookups: 2},
		{name: "source_error", executionError: true, sourceError: true, phase: runstate.ToolsRunning, want: sourceErr, lookups: 1},
		{name: "query_cancel", executionError: true, queryCancel: true, phase: runstate.ToolsRunning, want: context.Canceled, lookups: 1},
		{name: "bad_results", badResults: true, phase: runstate.ToolsRunning, want: runstate.ErrTransition},
		{name: "missing_source", executionError: true, noSource: true, phase: runstate.ToolsRunning, want: runstate.ErrOutcomeSourceRequired},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			execCtx := context.WithValue(context.Background(), struct{ key string }{"exec"}, "scope")
			query, cancel := context.WithCancel(context.Background())
			defer cancel()
			var executions atomic.Int32
			h := host28(t, execCtx, func(ctx context.Context, calls []model.ToolCall) ([]model.Message, error) {
				executions.Add(1)
				if ctx != execCtx || len(calls) != 2 {
					t.Fatal(ctx, calls)
				}
				out := results28(calls)
				if row.badResults {
					out[0].ToolCallID = "wrong"
				}
				if row.executionError {
					return out, executionErr
				}
				return out, nil
			})
			source := &source28{lookup: func(ctx context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				if ctx != query {
					t.Fatal("query context replaced")
				}
				if row.queryCancel {
					cancel()
				}
				if row.sourceError {
					return runstate.ToolOutcomeEvidence{}, sourceErr
				}
				e := known28(a)
				if a.Call.ID == "call-1" && row.basis != runstate.BasisResultKnown {
					e.Evidence.Basis = row.basis
					e.Evidence.Result = nil
				}
				return e, nil
			}}
			var port runstate.ToolOutcomeSource = source
			if row.noSource {
				port = nil
			}
			outcome := runstate.DecisionApprove
			if row.reject {
				outcome = runstate.DecisionReject
			}
			report, e := h.Continue(decision28(outcome), query, port)
			if !errors.Is(e, row.want) || report.State.Phase != row.phase || report.Reconciled != row.reconciled || source.calls.Load() != row.lookups {
				t.Fatal(report, e, source.calls.Load())
			}
			wantExec := int32(1)
			if row.reject {
				wantExec = 0
			}
			if executions.Load() != wantExec {
				t.Fatal(executions.Load())
			}
			if row.executionError && !errors.Is(report.ExecutionErr, executionErr) {
				t.Fatal(report.ExecutionErr)
			}
			if !row.executionError && report.ExecutionErr != nil {
				t.Fatal(report.ExecutionErr)
			}
			decoded, e := runstate.DecodePauseSnapshot(report.Payload)
			if e != nil || !reflect.DeepEqual(decoded, report.Pause) || decoded.Decision.Outcome != outcome || decoded.Request.Checkpoint.State.Phase != runstate.ToolsPending {
				t.Fatal(decoded, e)
			}
			if !reflect.DeepEqual(report.State, h.Gate().Snapshot()) || report.State.Step != 1 {
				t.Fatal(report.State)
			}
			if row.phase == runstate.Ready {
				if len(report.State.Messages) != 4 || report.State.Pending != nil {
					t.Fatal(report.State)
				}
				apply28(t, h.Gate(), runstate.Event{Kind: runstate.StartModel})
				s := apply28(t, h.Gate(), runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "摘要"}}})
				if s.Phase != runstate.Completed || s.Step != 2 {
					t.Fatal(s)
				}
			} else if len(report.State.Pending) != 2 || len(report.State.Messages) != 2 {
				t.Fatal(report.State)
			}
		})
	}
}
func TestDay28BuildAndEarlyFailure(t *testing.T) {
	execute := assembly.ToolBatchExecutor(func(context.Context, []model.ToolCall) ([]model.Message, error) {
		t.Fatal("must not execute")
		return nil, nil
	})
	if h, e := assembly.BuildWeek04(nil, "r28", "input", 3, execute); h != nil || !errors.Is(e, runstate.ErrTransition) {
		t.Fatal(h, e)
	}
	if h, e := assembly.BuildWeek04(context.Background(), "r28", "input", 3, nil); h != nil || !errors.Is(e, assembly.ErrWeek04ExecutorRequired) {
		t.Fatal(h, e)
	}
	h := host28(t, context.Background(), execute)
	if h.Gate() != h.Gate() {
		t.Fatal("gate must be stable")
	}
	before := h.Gate().Snapshot()
	d := decision28(runstate.DecisionApprove)
	d.RequestID = "other"
	r, e := h.Continue(d, nil, nil)
	if !errors.Is(e, runstate.ErrDecisionMismatch) || !reflect.DeepEqual(r.State, before) || r.Payload != nil || r.Pause.SchemaVersion != 0 {
		t.Fatal(r, e)
	}
}
func TestDay28CodecFailureKeepsPause(t *testing.T) {
	h, e := assembly.BuildWeek04(context.Background(), "r28", strings.Repeat("x", runstate.MaxPauseSnapshotBytes), 3, func(context.Context, []model.ToolCall) ([]model.Message, error) {
		t.Fatal("codec failure executed tools")
		return nil, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	g := h.Gate()
	apply28(t, g, runstate.Event{Kind: runstate.StartModel})
	apply28(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "c", Name: "get_document", Arguments: []byte(`{}`)}}}}})
	if _, e = g.Pause("pause-1", "review"); e != nil {
		t.Fatal(e)
	}
	r, e := h.Continue(decision28(runstate.DecisionApprove), nil, nil)
	if !errors.Is(e, runstate.ErrInvalidPauseSnapshot) || r.Payload != nil || r.Pause.SchemaVersion != 1 || r.State.Phase != runstate.ToolsPending {
		t.Fatal(e, r.State.Phase)
	}
	if _, ok := g.CurrentDecision(); !ok {
		t.Fatal("recorded decision was rolled back")
	}
	if _, ok := g.CurrentInterrupt(); !ok {
		t.Fatal("pause consumed")
	}
}
func TestDay28ExecutionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	execErr := errors.New("late response")
	var calls atomic.Int32
	h := host28(t, ctx, func(got context.Context, c []model.ToolCall) ([]model.Message, error) {
		if got != ctx {
			t.Fatal("wrong execution context")
		}
		cancel()
		return results28(c), execErr
	})
	source := &source28{lookup: func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		calls.Add(1)
		return runstate.ToolOutcomeEvidence{}, nil
	}}
	r, e := h.Continue(decision28(runstate.DecisionApprove), context.Background(), source)
	if !errors.Is(e, context.Canceled) || !errors.Is(r.ExecutionErr, execErr) || r.State.Phase != runstate.Cancelled || len(r.State.Messages) != 2 || len(r.State.Pending) != 2 || calls.Load() != 0 || r.Reconciled {
		t.Fatal(r, e)
	}
}
func TestDay28OwnershipAndIndependentHosts(t *testing.T) {
	var total atomic.Int32
	execute := assembly.ToolBatchExecutor(func(_ context.Context, c []model.ToolCall) ([]model.Message, error) {
		total.Add(1)
		out := results28(c)
		c[0].Arguments[0] = 'x'
		return out, nil
	})
	h := host28(t, context.Background(), execute)
	r, e := h.Continue(decision28(runstate.DecisionApprove), nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Pause.Request.Checkpoint.State.Pending[0].Arguments[0] = 'p'
	r.Pause.Decision.Actor = "edit"
	r.State.Messages[1].ToolCalls[0].Arguments[0] = 's'
	decoded, e := runstate.DecodePauseSnapshot(r.Payload)
	if e != nil || decoded.Request.Checkpoint.State.Pending[0].Arguments[0] != '{' || h.Gate().Snapshot().Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("report aliases host/payload", e)
	}
	for i := range r.Payload {
		r.Payload[i] = 0
	}
	if h.Gate().Snapshot().Phase != runstate.Ready {
		t.Fatal("payload mutated host")
	}
	hosts := make([]*assembly.Week04Host, 8)
	for i := range hosts {
		hosts[i] = host28(t, context.Background(), execute)
	}
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h *assembly.Week04Host) {
			defer wg.Done()
			r, e := h.Continue(decision28(runstate.DecisionApprove), nil, nil)
			if e != nil || r.State.Phase != runstate.Ready {
				t.Error(r, e)
			}
		}(host)
	}
	wg.Wait()
	if total.Load() != 9 {
		t.Fatal(total.Load())
	}
}
