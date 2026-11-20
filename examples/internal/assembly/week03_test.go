package assembly_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/assembly"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

type store21 struct {
	load         func(context.Context, string) (runstate.StoredCheckpoint, error)
	loads, saves atomic.Int32
}

func (s *store21) Load(ctx context.Context, id string) (runstate.StoredCheckpoint, error) {
	s.loads.Add(1)
	return s.load(ctx, id)
}
func (s *store21) Save(context.Context, runstate.Checkpoint, uint64) (runstate.StoredCheckpoint, error) {
	s.saves.Add(1)
	return runstate.StoredCheckpoint{}, errors.New("unexpected Save")
}

type source21 struct {
	evidence func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error)
	calls    atomic.Int32
}

func (s *source21) Evidence(ctx context.Context, v runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
	s.calls.Add(1)
	return s.evidence(ctx, v)
}
func apply21(t *testing.T, r *runstate.Run, e runstate.Event) {
	t.Helper()
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}
}
func fixture21(t *testing.T, kind string) (*runstate.Run, *runstate.MemoryCheckpointStore, runstate.StoredCheckpoint) {
	t.Helper()
	ctx := context.Background()
	budget := 3
	if kind == "exhausted" {
		budget = 1
	}
	r, err := runstate.NewRun(ctx, "r21", "读取 doc-1 / doc-2", budget)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "ready" {
		apply21(t, r, runstate.Event{Kind: runstate.StartModel})
		if kind == "completed" {
			apply21(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "已读取"}}})
		} else {
			calls := []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}, {ID: "call-2", Name: "get_document", Arguments: []byte(`{"id":"doc-2"}`)}}
			apply21(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: calls}}})
			switch kind {
			case "failed", "cancelled":
				apply21(t, r, runstate.Event{Kind: runstate.StartTools})
				event := runstate.Abort
				if kind == "cancelled" {
					event = runstate.Cancel
				}
				apply21(t, r, runstate.Event{Kind: event, Reason: "controlled"})
			case "exhausted":
				apply21(t, r, runstate.Event{Kind: runstate.StartTools})
				apply21(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "one"}, {Role: "tool", ToolCallID: "call-2", Content: "two"}}})
			}
		}
	}
	cp, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	s := runstate.NewMemoryCheckpointStore()
	v, err := s.Save(ctx, cp, 0)
	if err != nil {
		t.Fatal(err)
	}
	return r, s, v
}
func evidence21(v runstate.StoredCheckpoint, bases ...runstate.ReplayBasis) runstate.ReplayEvidenceSet {
	out := runstate.ReplayEvidenceSet{RunID: v.Checkpoint.State.RunID, Version: v.Version, Calls: make([]runstate.ReplayEvidence, len(bases))}
	for i, b := range bases {
		c := v.Checkpoint.State.Pending[i]
		out.Calls[i] = runstate.ReplayEvidence{Call: c, Basis: b}
		if b == runstate.BasisResultKnown {
			out.Calls[i].Result = &model.Message{Role: "tool", ToolCallID: c.ID, Content: "confirmed"}
		}
		if b == runstate.BasisBackendDedup {
			out.Calls[i].IdempotencyKey = fmt.Sprintf("host-key-%d", i+1)
		}
	}
	return out
}
func zero21(t *testing.T, r assembly.Week03Report, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(r, assembly.Week03Report{}) {
		t.Fatal(r, err, want)
	}
}
func TestDay21Week03Matrix(t *testing.T) {
	rows := []struct {
		name, kind string
		position   runstate.RecoveryPosition
		bases      []runstate.ReplayBasis
		blocked    bool
	}{
		{"ready", "ready", runstate.RecoveryReady, nil, false},
		{"budget", "exhausted", runstate.RecoveryExhausted, nil, false},
		{"completed", "completed", runstate.RecoveryTerminal, nil, false},
		{"failed_with_pending", "failed", runstate.RecoveryTerminal, nil, false},
		{"cancelled_with_pending", "cancelled", runstate.RecoveryTerminal, nil, false},
		{"unknown_blocks", "pending", runstate.RecoveryReviewPending, []runstate.ReplayBasis{runstate.BasisUnknown, runstate.BasisResultKnown}, true},
		{"result_and_dedup", "pending", runstate.RecoveryReviewPending, []runstate.ReplayBasis{runstate.BasisResultKnown, runstate.BasisBackendDedup}, false},
		{"not_started", "pending", runstate.RecoveryReviewPending, []runstate.ReplayBasis{runstate.BasisNotStarted, runstate.BasisNotStarted}, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r, memory, before := fixture21(t, row.kind)
			runBefore := r.Snapshot()
			ctx := context.Background()
			store := &store21{load: memory.Load}
			source := &source21{evidence: func(got context.Context, v runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
				if got != ctx || !reflect.DeepEqual(v, before) {
					t.Error("source input")
				}
				return evidence21(v, row.bases...), nil
			}}
			report, err := assembly.ReviewWeek03(ctx, store, source, "r21")
			if err != nil || report.Recovery.Position != row.position || !reflect.DeepEqual(report.Recovery.Record, before) {
				t.Fatal(report, err)
			}
			if row.bases == nil {
				if report.Replay != nil || source.calls.Load() != 0 {
					t.Fatal("unexpected evidence/replay")
				}
				// nil source is valid when no pending review is required.
				if _, err := assembly.ReviewWeek03(ctx, memory, nil, "r21"); err != nil {
					t.Fatal(err)
				}
			} else {
				if report.Replay == nil || report.Replay.Blocked != row.blocked || source.calls.Load() != 1 || len(report.Replay.Steps) != 2 {
					t.Fatal(report)
				}
				if row.name == "result_and_dedup" && (report.Replay.Steps[0].Action != runstate.ActionReuse || report.Replay.Steps[0].Result.Content != "confirmed" || report.Replay.Steps[1].IdempotencyKey != "host-key-2") {
					t.Fatal(report)
				}
			}
			after, err := memory.Load(ctx, "r21")
			if err != nil || store.loads.Load() != 1 || store.saves.Load() != 0 || !reflect.DeepEqual(after, before) || !reflect.DeepEqual(runBefore, r.Snapshot()) {
				t.Fatal(after, err)
			}
		})
	}
}

type fault21 struct{}

func (*fault21) Error() string { return "dependency unavailable" }
func TestDay21DependencyFailures(t *testing.T) {
	_, memory, before := fixture21(t, "pending")
	ctx := context.Background()
	source := &source21{evidence: func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		return evidence21(before, runstate.BasisUnknown, runstate.BasisUnknown), nil
	}}
	cause := &fault21{}
	store := &store21{load: func(context.Context, string) (runstate.StoredCheckpoint, error) {
		return before, fmt.Errorf("load: %w", cause)
	}}
	report, err := assembly.ReviewWeek03(ctx, store, source, "r21")
	zero21(t, report, err, cause)
	var typed *fault21
	if !errors.As(err, &typed) || typed != cause || source.calls.Load() != 0 || store.loads.Load() != 1 {
		t.Fatal(err)
	}
	report, err = assembly.ReviewWeek03(ctx, runstate.NewMemoryCheckpointStore(), source, "missing")
	zero21(t, report, err, runstate.ErrCheckpointNotFound)
	report, err = assembly.ReviewWeek03(ctx, memory, nil, "r21")
	zero21(t, report, err, assembly.ErrEvidenceSourceRequired)
	source = &source21{evidence: func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		return evidence21(before, runstate.BasisUnknown, runstate.BasisUnknown), fmt.Errorf("evidence: %w", cause)
	}}
	report, err = assembly.ReviewWeek03(ctx, memory, source, "r21")
	zero21(t, report, err, cause)
	if !errors.As(err, &typed) || typed != cause || source.calls.Load() != 1 {
		t.Fatal(err)
	}
}
func TestDay21MalformedAndStale(t *testing.T) {
	_, memory, v1 := fixture21(t, "pending")
	ctx := context.Background()
	old := evidence21(v1, runstate.BasisUnknown, runstate.BasisUnknown)
	if _, err := memory.Save(ctx, v1.Checkpoint, 1); err != nil {
		t.Fatal(err)
	}
	source := &source21{evidence: func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) { return old, nil }}
	report, err := assembly.ReviewWeek03(ctx, memory, source, "r21")
	zero21(t, report, err, runstate.ErrReplayEvidence)
	current, err := memory.Load(ctx, "r21")
	if err != nil || current.Version != 2 {
		t.Fatal(current, err)
	}
	bad := v1
	bad.Checkpoint = v1.Checkpoint.Clone()
	bad.Checkpoint.State.Phase = runstate.ToolsRunning
	store := &store21{load: func(context.Context, string) (runstate.StoredCheckpoint, error) { return bad, nil }}
	source.calls.Store(0)
	report, err = assembly.ReviewWeek03(ctx, store, source, "r21")
	zero21(t, report, err, runstate.ErrInvalidCheckpoint)
	if source.calls.Load() != 0 || store.saves.Load() != 0 {
		t.Fatal("invalid input reached source")
	}
	fresh := evidence21(current, runstate.BasisResultKnown, runstate.BasisBackendDedup)
	fresh.Calls[1].Call.Arguments = []byte("wrong")
	source = &source21{evidence: func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		return fresh, nil
	}}
	report, err = assembly.ReviewWeek03(ctx, memory, source, "r21")
	zero21(t, report, err, runstate.ErrReplayEvidence)
}
func TestDay21Cancellation(t *testing.T) {
	_, memory, v := fixture21(t, "pending")
	store := &store21{load: memory.Load}
	source := &source21{evidence: func(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		return evidence21(v, runstate.BasisUnknown, runstate.BasisUnknown), nil
	}}
	report, err := assembly.ReviewWeek03(nil, store, source, "r21")
	zero21(t, report, err, runstate.ErrInvalidCheckpoint)
	report, err = assembly.ReviewWeek03(context.Background(), nil, source, "r21")
	zero21(t, report, err, runstate.ErrInvalidCheckpoint)
	report, err = assembly.ReviewWeek03(context.Background(), store, source, " ")
	zero21(t, report, err, runstate.ErrInvalidCheckpoint)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, finish := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer finish()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{canceled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		report, err = assembly.ReviewWeek03(tc.ctx, store, source, "r21")
		zero21(t, report, err, tc.want)
	}
	if store.loads.Load() != 0 || source.calls.Load() != 0 {
		t.Fatal("pre-cancel dependencies")
	}
	// Cancellation during Load prevents Evidence.
	ctx, c := context.WithCancel(context.Background())
	entered := make(chan struct{})
	load := &store21{load: func(got context.Context, id string) (runstate.StoredCheckpoint, error) {
		close(entered)
		<-got.Done()
		return v, nil
	}}
	type result struct {
		report assembly.Week03Report
		err    error
	}
	done := make(chan result, 1)
	go func() { r, e := assembly.ReviewWeek03(ctx, load, source, "r21"); done <- result{r, e} }()
	<-entered
	c()
	got := <-done
	zero21(t, got.report, got.err, context.Canceled)
	if source.calls.Load() != 0 {
		t.Fatal("cancelled load reached Evidence")
	}
	// Cancellation during Evidence rejects both late evidence and its error.
	ctx, c = context.WithCancel(context.Background())
	entered = make(chan struct{})
	late := &source21{evidence: func(got context.Context, record runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		if got != ctx || record.Version != 1 {
			t.Error("source context/record")
		}
		close(entered)
		<-got.Done()
		return evidence21(v, runstate.BasisNotStarted, runstate.BasisNotStarted), &fault21{}
	}}
	done = make(chan result, 1)
	go func() { r, e := assembly.ReviewWeek03(ctx, store, late, "r21"); done <- result{r, e} }()
	<-entered
	c()
	got = <-done
	zero21(t, got.report, got.err, context.Canceled)
	if late.calls.Load() != 1 || store.saves.Load() != 0 {
		t.Fatal("retry/write")
	}
}
func TestDay21OwnershipAndConcurrency(t *testing.T) {
	_, memory, v := fixture21(t, "pending")
	ctx := context.Background()
	prepared := evidence21(v, runstate.BasisResultKnown, runstate.BasisBackendDedup)
	source := &source21{evidence: func(_ context.Context, input runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error) {
		input.Checkpoint.State.Pending[0].Arguments[0] = 'x'
		input.Version = 999
		return prepared, nil
	}}
	report, err := assembly.ReviewWeek03(ctx, memory, source, "r21")
	if err != nil || report.Recovery.Record.Version != 1 || report.Recovery.Record.Checkpoint.State.Pending[0].Arguments[0] != '{' {
		t.Fatal(report, err)
	}
	report.Recovery.Record.Checkpoint.State.Pending[0].Arguments[0] = 'r'
	if report.Replay.Record.Checkpoint.State.Pending[0].Arguments[0] != '{' {
		t.Fatal("report fields alias")
	}
	report.Replay.Steps[0].Result.Content = "caller"
	if prepared.Calls[0].Result.Content != "confirmed" {
		t.Fatal("source result aliases report")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := assembly.ReviewWeek03(ctx, memory, source, "r21")
			if e != nil {
				t.Error(e)
				return
			}
			if r.Replay == nil || r.Replay.Blocked || r.Recovery.Record.Version != 1 {
				t.Error(r)
				return
			}
			r.Replay.Steps[0].Call.Arguments[0] = 'z'
			r.Replay.Steps[0].Result.Content = "owned"
		}()
	}
	wg.Wait()
	after, err := memory.Load(ctx, "r21")
	if err != nil || !reflect.DeepEqual(v, after) || prepared.Calls[0].Result.Content != "confirmed" || source.calls.Load() != 17 {
		t.Fatal(after, err)
	}
}
