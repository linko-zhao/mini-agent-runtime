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

type source26 struct {
	calls  atomic.Int32
	lookup func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error)
}

func (s *source26) Lookup(ctx context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
	s.calls.Add(1)
	return s.lookup(ctx, a)
}
func known26(a runstate.ToolAttempt) runstate.ToolOutcomeEvidence {
	call := a.Call
	call.Arguments = append([]byte(nil), a.Call.Arguments...)
	return runstate.ToolOutcomeEvidence{RunID: a.RunID, Step: a.Step, Evidence: runstate.ReplayEvidence{Call: call, Basis: runstate.BasisResultKnown, Result: &model.Message{Role: "tool", ToolCallID: call.ID, Content: "body for " + call.ID}}}
}
func running26(t *testing.T) (*runstate.PauseGate, runstate.State) {
	t.Helper()
	r, e := runstate.NewRun(context.Background(), "r26", "读取 doc-1 和 doc-2", 3)
	if e != nil {
		t.Fatal(e)
	}
	g, e := runstate.NewPauseGate(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.Apply(runstate.Event{Kind: runstate.StartModel}); e != nil {
		t.Fatal(e)
	}
	response := model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte("{ \"id\": \"doc-1\" }")}, {ID: "call-2", Name: "get_document", Arguments: []byte(`{"id":"doc-2"}`)}}}}
	if _, e = g.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: &response}); e != nil {
		t.Fatal(e)
	}
	if _, e = g.Pause("pause-1", "review"); e != nil {
		t.Fatal(e)
	}
	if _, e = g.Decide(runstate.ApprovalDecision{RunID: "r26", RequestID: "pause-1", Actor: "reviewer", Outcome: runstate.DecisionApprove, Reason: "reviewed"}); e != nil {
		t.Fatal(e)
	}
	s, e := g.ResolveDecision("r26", "pause-1")
	if e != nil {
		t.Fatal(e)
	}
	return g, s
}
func zeroReport26(t *testing.T, r runstate.ToolOutcomeReport, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || !reflect.DeepEqual(r, runstate.ToolOutcomeReport{}) {
		t.Fatal(r, e, want)
	}
}
func TestDay26OutcomeMatrix(t *testing.T) {
	for _, basis := range []runstate.ReplayBasis{runstate.BasisUnknown, runstate.BasisNotStarted, runstate.BasisResultKnown} {
		t.Run(string(basis), func(t *testing.T) {
			g, before := running26(t)
			ctx := context.WithValue(context.Background(), struct{ key string }{"scope"}, "host-scope")
			order := []string{}
			source := &source26{lookup: func(got context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				if got != ctx || a.RunID != "r26" || a.Step != 1 {
					t.Fatal(got, a)
				}
				order = append(order, a.Call.ID)
				e := known26(a)
				if a.Call.ID == "call-1" && basis != runstate.BasisResultKnown {
					e.Evidence.Basis = basis
					e.Evidence.Result = nil
				}
				return e, nil
			}}
			report, e := runstate.InspectToolOutcomes(ctx, source, before)
			all := basis == runstate.BasisResultKnown
			if e != nil || report.AllResultsKnown != all || len(report.Evidence) != 2 || source.calls.Load() != 2 || !reflect.DeepEqual(order, []string{"call-1", "call-2"}) || !reflect.DeepEqual(report.Observed, before) {
				t.Fatal(report, e, order)
			}
			if !reflect.DeepEqual(g.Snapshot(), before) {
				t.Fatal("inspection applied a transition")
			}
			if !all {
				if report.Results != nil || report.Evidence[0].Evidence.Basis != basis || report.Evidence[1].Evidence.Result == nil {
					t.Fatal(report)
				}
			} else {
				if len(report.Results) != 2 || report.Results[0].ToolCallID != "call-1" || report.Results[1].ToolCallID != "call-2" {
					t.Fatal(report)
				}
				// This fixture still owns the current Run; only the HOST applies results.
				s, e := g.Apply(runstate.Event{Kind: runstate.ToolsReturned, Results: report.Results})
				if e != nil || s.Phase != runstate.Ready || s.Pending != nil || s.Messages[len(s.Messages)-1].ToolCallID != "call-2" {
					t.Fatal(s, e)
				}
			}
		})
	}
}
func TestDay26InputValidation(t *testing.T) {
	rows := []struct {
		name   string
		change func(*runstate.State)
	}{
		{"run", func(s *runstate.State) { s.RunID = " " }}, {"phase", func(s *runstate.State) { s.Phase = runstate.ToolsPending }},
		{"step", func(s *runstate.State) { s.Step = 0 }}, {"budget", func(s *runstate.State) { s.MaxSteps = 0 }},
		{"result", func(s *runstate.State) { s.Result = "done" }}, {"failure", func(s *runstate.State) { s.Failure = "failed" }},
		{"messages", func(s *runstate.State) { s.Messages = nil }}, {"first_role", func(s *runstate.State) { s.Messages[0].Role = "tool" }},
		{"pending", func(s *runstate.State) { s.Pending = nil }}, {"last_role", func(s *runstate.State) { s.Messages[1].Role = "user" }},
		{"last_calls", func(s *runstate.State) { s.Messages[1].ToolCalls = nil }}, {"call_id", func(s *runstate.State) { s.Pending[0].ID = "" }},
		{"name", func(s *runstate.State) { s.Pending[0].Name = " " }}, {"args", func(s *runstate.State) { s.Pending[0].Arguments = []byte("changed") }},
		{"duplicate", func(s *runstate.State) {
			s.Pending[1] = s.Pending[0]
			s.Messages[1].ToolCalls[1] = s.Messages[1].ToolCalls[0]
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, s := running26(t)
			row.change(&s)
			source := &source26{lookup: func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				t.Fatal("invalid state queried")
				return runstate.ToolOutcomeEvidence{}, nil
			}}
			r, e := runstate.InspectToolOutcomes(context.Background(), source, s)
			zeroReport26(t, r, e, runstate.ErrOutcomeState)
			if source.calls.Load() != 0 {
				t.Fatal(source.calls.Load())
			}
		})
	}
	_, s := running26(t)
	r, e := runstate.InspectToolOutcomes(nil, nil, s)
	zeroReport26(t, r, e, runstate.ErrOutcomeState)
	r, e = runstate.InspectToolOutcomes(context.Background(), nil, s)
	zeroReport26(t, r, e, runstate.ErrOutcomeSourceRequired)
}
func TestDay26EvidenceBinding(t *testing.T) {
	rows := []struct {
		name   string
		change func(*runstate.ToolOutcomeEvidence)
	}{
		{"run", func(e *runstate.ToolOutcomeEvidence) { e.RunID = "other" }}, {"step", func(e *runstate.ToolOutcomeEvidence) { e.Step = 0 }},
		{"call_id", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Call.ID = "old" }}, {"name", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Call.Name = "other" }},
		{"args", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Call.Arguments = []byte(`{"id":"doc-1"}`) }},
		{"key", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.IdempotencyKey = "key" }},
		{"result_nil", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Result = nil }},
		{"role", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Result.Role = "assistant" }},
		{"result_id", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Result.ToolCallID = "other" }},
		{"nested_call", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Result.ToolCalls = []model.ToolCall{{ID: "x"}} }},
		{"unknown_with_result", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Basis = runstate.BasisUnknown }},
		{"not_started_with_result", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Basis = runstate.BasisNotStarted }},
		{"dedup", func(e *runstate.ToolOutcomeEvidence) {
			e.Evidence.Basis = runstate.BasisBackendDedup
			e.Evidence.Result = nil
			e.Evidence.IdempotencyKey = "key"
		}},
		{"unknown_basis", func(e *runstate.ToolOutcomeEvidence) { e.Evidence.Basis = "other" }},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, s := running26(t)
			source := &source26{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				e := known26(a)
				row.change(&e)
				return e, nil
			}}
			r, e := runstate.InspectToolOutcomes(context.Background(), source, s)
			zeroReport26(t, r, e, runstate.ErrReplayEvidence)
			if source.calls.Load() != 1 {
				t.Fatal(source.calls.Load())
			}
		})
	}
}

type outage26 struct{}

func (*outage26) Error() string { return "receipt source unavailable" }
func TestDay26SourceFailure(t *testing.T) {
	for _, failedCall := range []string{"call-1", "call-2"} {
		t.Run(failedCall, func(t *testing.T) {
			g, s := running26(t)
			cause := &outage26{}
			wrapped := fmt.Errorf("lookup failed: %w", cause)
			source := &source26{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				if a.Call.ID == failedCall {
					return known26(a), wrapped
				}
				return known26(a), nil
			}}
			r, e := runstate.InspectToolOutcomes(context.Background(), source, s)
			zeroReport26(t, r, e, cause)
			var typed *outage26
			if !errors.As(e, &typed) || typed != cause {
				t.Fatal(e)
			}
			want := int32(1)
			if failedCall == "call-2" {
				want = 2
			}
			if source.calls.Load() != want || !reflect.DeepEqual(g.Snapshot(), s) {
				t.Fatal(source.calls.Load())
			}
		})
	}
}

type deadline26 struct{ context.Context }

func (d deadline26) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay26Cancellation(t *testing.T) {
	_, s := running26(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, e := runstate.InspectToolOutcomes(ctx, nil, runstate.State{})
	zeroReport26(t, r, e, context.Canceled)
	for _, deadline := range []bool{false, true} {
		for _, withError := range []bool{false, true} {
			t.Run(fmt.Sprintf("deadline=%v/error=%v", deadline, withError), func(t *testing.T) {
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				var ctx context.Context = base
				want := error(context.Canceled)
				if deadline {
					ctx = deadline26{base}
					want = context.DeadlineExceeded
				}
				source := &source26{lookup: func(got context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
					if got != ctx {
						t.Fatal("context replaced")
					}
					cancel()
					if withError {
						return known26(a), &outage26{}
					}
					return known26(a), nil
				}}
				r, e := runstate.InspectToolOutcomes(ctx, source, s)
				zeroReport26(t, r, e, want)
				if source.calls.Load() != 1 {
					t.Fatal(source.calls.Load())
				}
			})
		}
	}
}
func TestDay26Ownership(t *testing.T) {
	g, s := running26(t)
	before := g.Snapshot()
	var results []*model.Message
	source := &source26{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		e := known26(a)
		results = append(results, e.Evidence.Result)
		a.Call.Arguments[0] = 'x'
		return e, nil
	}}
	r, e := runstate.InspectToolOutcomes(context.Background(), source, s)
	if e != nil {
		t.Fatal(e)
	}
	for _, msg := range results {
		msg.Content = "source edit"
	}
	if r.Results[0].Content != "body for call-1" || r.Evidence[0].Evidence.Result.Content != "body for call-1" {
		t.Fatal(r)
	}
	r.Results[0].Content = "results edit"
	r.Evidence[0].Evidence.Call.Arguments[0] = 'e'
	r.Observed.Pending[0].Arguments[0] = 'o'
	if r.Evidence[0].Evidence.Result.Content != "body for call-1" || r.Observed.Messages[1].ToolCalls[0].Arguments[0] != '{' || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(g.Snapshot(), before) {
		t.Fatal("ownership boundary failed")
	}
}
func TestDay26ConcurrentReports(t *testing.T) {
	g, s := running26(t)
	source := &source26{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		return known26(a), nil
	}}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := runstate.InspectToolOutcomes(context.Background(), source, s)
			if e != nil || !r.AllResultsKnown || len(r.Results) != 2 {
				t.Error(r, e)
				return
			}
			r.Observed.Pending[0].Arguments[0] = 'x'
			r.Evidence[0].Evidence.Call.Arguments[0] = 'e'
			r.Results[0].Content = "edit"
		}()
	}
	wg.Wait()
	if source.calls.Load() != 32 || !reflect.DeepEqual(g.Snapshot(), s) {
		t.Fatal(source.calls.Load())
	}
}
