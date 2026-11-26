package runstate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

type source27 struct {
	calls  atomic.Int32
	lookup func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error)
}

func (s *source27) Lookup(ctx context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
	s.calls.Add(1)
	return s.lookup(ctx, a)
}
func known27(a runstate.ToolAttempt) runstate.ToolOutcomeEvidence {
	call := a.Call
	call.Arguments = append([]byte(nil), a.Call.Arguments...)
	return runstate.ToolOutcomeEvidence{RunID: a.RunID, Step: a.Step, Evidence: runstate.ReplayEvidence{Call: call, Basis: runstate.BasisResultKnown, Result: &model.Message{Role: "tool", ToolCallID: call.ID, Content: "body " + call.ID}}}
}
func apply27(t *testing.T, g *runstate.PauseGate, e runstate.Event) runstate.State {
	t.Helper()
	s, err := g.Apply(e)
	if err != nil {
		t.Fatal(s, err)
	}
	return s
}
func batch27(t *testing.T, g *runstate.PauseGate, pauseID string) runstate.State {
	t.Helper()
	apply27(t, g, runstate.Event{Kind: runstate.StartModel})
	response := model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}, {ID: "call-2", Name: "get_document", Arguments: []byte(`{"id":"doc-2"}`)}}}}
	apply27(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &response})
	if _, e := g.Pause(pauseID, "review"); e != nil {
		t.Fatal(e)
	}
	if _, e := g.Decide(runstate.ApprovalDecision{RunID: "r27", RequestID: pauseID, Actor: "reviewer", Outcome: runstate.DecisionApprove, Reason: "reviewed"}); e != nil {
		t.Fatal(e)
	}
	s, e := g.ResolveDecision("r27", pauseID)
	if e != nil {
		t.Fatal(s, e)
	}
	return s
}
func gate27(t *testing.T, ctx context.Context) (*runstate.PauseGate, runstate.State) {
	t.Helper()
	r, e := runstate.NewRun(ctx, "r27", "读取 doc-1 和 doc-2", 4)
	if e != nil {
		t.Fatal(e)
	}
	g, e := runstate.NewPauseGate(r)
	if e != nil {
		t.Fatal(e)
	}
	return g, batch27(t, g, "pause-1")
}

type result27 struct {
	state runstate.State
	err   error
}

// A timeout is a deadlock guard only; channels determine ordering, not sleeps.
func wait27[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v
	case <-timer.C:
		t.Fatal("operation blocked while a lookup was pending")
		var zero T
		return zero
	}
}
func TestDay27CommitAndContinue(t *testing.T) {
	g, before := gate27(t, context.Background())
	query := context.WithValue(context.Background(), struct{ scope string }{"query"}, "host")
	source := &source27{lookup: func(ctx context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		if ctx != query {
			t.Fatal("query context replaced")
		}
		// These gate reads must remain usable while external queries run.
		if _, ok := g.CurrentInterrupt(); ok {
			t.Fatal("unexpected pause")
		}
		if _, ok := g.CurrentDecision(); ok {
			t.Fatal("unexpected decision")
		}
		return known27(a), nil
	}}
	s, e := g.ReconcileToolResults(query, source)
	if e != nil || s.Phase != runstate.Ready || s.Pending != nil || s.Step != before.Step || len(s.Messages) != len(before.Messages)+2 || source.calls.Load() != 2 {
		t.Fatal(s, e, source.calls.Load())
	}
	if s.Messages[2].ToolCallID != "call-1" || s.Messages[3].ToolCallID != "call-2" {
		t.Fatal(s)
	}
	s.Messages[0].Content = "caller edit"
	s.Messages[1].ToolCalls[0].Arguments[0] = 'x'
	s.Messages[2].Content = "caller edit"
	if g.Snapshot().Messages[0].Content != before.Messages[0].Content || g.Snapshot().Messages[1].ToolCalls[0].Arguments[0] != '{' || g.Snapshot().Messages[2].Content != "body call-1" {
		t.Fatal("success result aliases run")
	}
	// A later call starts from Ready and fails validation without more queries.
	s, e = g.ReconcileToolResults(query, source)
	if !errors.Is(e, runstate.ErrOutcomeState) || s.Phase != runstate.Ready || source.calls.Load() != 2 {
		t.Fatal(s, e)
	}
	apply27(t, g, runstate.Event{Kind: runstate.StartModel})
	s = apply27(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "摘要"}}})
	if s.Phase != runstate.Completed || s.Result != "摘要" || s.Step != 2 {
		t.Fatal(s)
	}
}
func TestDay27IncompleteDoesNotCommit(t *testing.T) {
	for _, basis := range []runstate.ReplayBasis{runstate.BasisUnknown, runstate.BasisNotStarted} {
		t.Run(string(basis), func(t *testing.T) {
			g, before := gate27(t, context.Background())
			source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				e := known27(a)
				if a.Call.ID == "call-1" {
					e.Evidence.Basis = basis
					e.Evidence.Result = nil
				}
				return e, nil
			}}
			s, e := g.ReconcileToolResults(context.Background(), source)
			if !errors.Is(e, runstate.ErrOutcomeIncomplete) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(g.Snapshot(), before) || source.calls.Load() != 2 {
				t.Fatal(s, e)
			}
		})
	}
}

type outage27 struct{}

func (*outage27) Error() string { return "outcome source unavailable" }
func TestDay27DelegatedFailures(t *testing.T) {
	g, before := gate27(t, context.Background())
	if s, e := g.ReconcileToolResults(nil, nil); !errors.Is(e, runstate.ErrOutcomeState) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, e)
	}
	if s, e := g.ReconcileToolResults(context.Background(), nil); !errors.Is(e, runstate.ErrOutcomeSourceRequired) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, e)
	}
	cause := &outage27{}
	wrapped := fmt.Errorf("lookup: %w", cause)
	source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		if a.Call.ID == "call-2" {
			return known27(a), wrapped
		}
		return known27(a), nil
	}}
	s, e := g.ReconcileToolResults(context.Background(), source)
	var typed *outage27
	if !errors.Is(e, cause) || !errors.As(e, &typed) || typed != cause || !reflect.DeepEqual(s, before) || source.calls.Load() != 2 {
		t.Fatal(s, e)
	}
	source = &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		e := known27(a)
		e.Step++
		return e, nil
	}}
	s, e = g.ReconcileToolResults(context.Background(), source)
	if !errors.Is(e, runstate.ErrReplayEvidence) || !reflect.DeepEqual(s, before) || source.calls.Load() != 1 {
		t.Fatal(s, e)
	}
	s.Pending[0].Arguments[0] = 'x'
	if !reflect.DeepEqual(g.Snapshot(), before) {
		t.Fatal("error state aliases run")
	}
}

type deadline27 struct{ context.Context }

func (d deadline27) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay27QueryCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			g, before := gate27(t, context.Background())
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			var query context.Context = base
			want := error(context.Canceled)
			if deadline {
				query = deadline27{base}
				want = context.DeadlineExceeded
			}
			source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				cancel()
				return known27(a), &outage27{}
			}}
			s, e := g.ReconcileToolResults(query, source)
			if !errors.Is(e, want) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(g.Snapshot(), before) || source.calls.Load() != 1 {
				t.Fatal(s, e)
			}
		})
	}
	// When the same context was already cancelled, no query/commit occurs.
	ctx, cancel := context.WithCancel(context.Background())
	g, before := gate27(t, ctx)
	cancel()
	source := &source27{lookup: func(context.Context, runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		t.Fatal("pre-cancelled query")
		return runstate.ToolOutcomeEvidence{}, nil
	}}
	s, e := g.ReconcileToolResults(ctx, source)
	if !errors.Is(e, context.Canceled) || !reflect.DeepEqual(s, before) || source.calls.Load() != 0 {
		t.Fatal(s, e)
	}
}
func TestDay27OriginalCancellationAtCommit(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			var original context.Context = base
			want := error(context.Canceled)
			if deadline {
				original = deadline27{base}
				want = context.DeadlineExceeded
			}
			g, before := gate27(t, original)
			source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
				cancel()
				return known27(a), nil
			}}
			s, e := g.ReconcileToolResults(context.Background(), source)
			if !errors.Is(e, want) || s.Phase != runstate.Cancelled || s.Failure != "cancelled" || len(s.Pending) != 2 || len(s.Messages) != len(before.Messages) || source.calls.Load() != 2 {
				t.Fatal(s, e)
			}
			if _, ok := g.CurrentInterrupt(); ok {
				t.Fatal("pause retained")
			}
			if _, ok := g.CurrentDecision(); ok {
				t.Fatal("decision retained")
			}
		})
	}
}
func TestDay27QueryOutsideGateLock(t *testing.T) {
	g, before := gate27(t, context.Background())
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		if a.Call.ID == "call-1" {
			entered <- struct{}{}
			<-release
		}
		return known27(a), nil
	}}
	done := make(chan result27, 1)
	go func() { s, e := g.ReconcileToolResults(context.Background(), source); done <- result27{s, e} }()
	wait27(t, entered)
	cancelled := make(chan result27, 1)
	go func() { s, e := g.Apply(runstate.Event{Kind: runstate.Cancel}); cancelled <- result27{s, e} }()
	stop := wait27(t, cancelled)
	if stop.err != nil || stop.state.Phase != runstate.Cancelled {
		t.Fatal(stop)
	}
	once.Do(func() { close(release) })
	got := wait27(t, done)
	if !errors.Is(got.err, runstate.ErrOutcomeConflict) || got.state.Phase != runstate.Cancelled || len(got.state.Messages) != len(before.Messages) || source.calls.Load() != 2 {
		t.Fatal(got)
	}
}
func TestDay27TwoCoordinatorsCommitOnce(t *testing.T) {
	g, before := gate27(t, context.Background())
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		if a.Call.ID == "call-1" {
			entered <- struct{}{}
			<-release
		}
		return known27(a), nil
	}}
	done := make(chan result27, 2)
	for i := 0; i < 2; i++ {
		go func() { s, e := g.ReconcileToolResults(context.Background(), source); done <- result27{s, e} }()
	}
	wait27(t, entered)
	wait27(t, entered)
	once.Do(func() { close(release) })
	results := []result27{wait27(t, done), wait27(t, done)}
	wins, conflicts := 0, 0
	for _, r := range results {
		if r.err == nil {
			wins++
		} else if errors.Is(r.err, runstate.ErrOutcomeConflict) {
			conflicts++
		} else {
			t.Fatal(r)
		}
		if r.state.Phase != runstate.Ready {
			t.Fatal(r)
		}
	}
	if wins != 1 || conflicts != 1 || source.calls.Load() != 4 || len(g.Snapshot().Messages) != len(before.Messages)+2 {
		t.Fatal(wins, conflicts, source.calls.Load(), g.Snapshot())
	}
	for _, r := range results {
		r.state.Messages[1].ToolCalls[0].Arguments[0] = 'x'
		r.state.Messages[2].Content = "edit"
	}
	if g.Snapshot().Messages[1].ToolCalls[0].Arguments[0] != '{' || g.Snapshot().Messages[2].Content != "body call-1" {
		t.Fatal("concurrent returns alias run")
	}
}
func TestDay27OldRoundCannotOverwriteNew(t *testing.T) {
	g, _ := gate27(t, context.Background())
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	source := &source27{lookup: func(_ context.Context, a runstate.ToolAttempt) (runstate.ToolOutcomeEvidence, error) {
		if a.Call.ID == "call-1" {
			entered <- struct{}{}
			<-release
		}
		return known27(a), nil
	}}
	done := make(chan result27, 1)
	go func() { s, e := g.ReconcileToolResults(context.Background(), source); done <- result27{s, e} }()
	wait27(t, entered)
	apply27(t, g, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "original 1"}, {Role: "tool", ToolCallID: "call-2", Content: "original 2"}}})
	current := batch27(t, g, "pause-2") // same call IDs/arguments, a different Step/history
	if current.Step != 2 {
		t.Fatal(current)
	}
	once.Do(func() { close(release) })
	got := wait27(t, done)
	if !errors.Is(got.err, runstate.ErrOutcomeConflict) || !reflect.DeepEqual(got.state, current) || !reflect.DeepEqual(g.Snapshot(), current) || source.calls.Load() != 2 {
		t.Fatal(got)
	}
}
