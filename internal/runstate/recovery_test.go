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

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

type store19 struct {
	load         func(context.Context, string) (runstate.StoredCheckpoint, error)
	loads, saves atomic.Int32
}

func (s *store19) Load(ctx context.Context, id string) (runstate.StoredCheckpoint, error) {
	s.loads.Add(1)
	return s.load(ctx, id)
}
func (s *store19) Save(context.Context, runstate.Checkpoint, uint64) (runstate.StoredCheckpoint, error) {
	s.saves.Add(1)
	return runstate.StoredCheckpoint{}, errors.New("unexpected Save")
}
func apply19(t *testing.T, r *runstate.Run, e runstate.Event) {
	t.Helper()
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}
}
func pending19(t *testing.T, r *runstate.Run) {
	t.Helper()
	apply19(t, r, runstate.Event{Kind: runstate.StartModel})
	apply19(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}})
}
func record19(t *testing.T, phase runstate.Phase) runstate.StoredCheckpoint {
	t.Helper()
	r, e := runstate.NewRun(context.Background(), "r19", "读取 doc-1", 3)
	if e != nil {
		t.Fatal(e)
	}
	switch phase {
	case runstate.ToolsPending:
		pending19(t, r)
	case runstate.Completed:
		apply19(t, r, runstate.Event{Kind: runstate.StartModel})
		apply19(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "已读取"}}})
	case runstate.Failed, runstate.Cancelled:
		pending19(t, r)
		apply19(t, r, runstate.Event{Kind: runstate.StartTools})
		kind := runstate.Abort
		if phase == runstate.Cancelled {
			kind = runstate.Cancel
		}
		apply19(t, r, runstate.Event{Kind: kind, Reason: "controlled"})
	}
	cp, e := r.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	return runstate.StoredCheckpoint{Version: 7, Checkpoint: cp}
}
func fake19(v runstate.StoredCheckpoint) *store19 {
	return &store19{load: func(context.Context, string) (runstate.StoredCheckpoint, error) { return v, nil }}
}
func zero19(t *testing.T, p runstate.RecoveryPlan, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || !reflect.DeepEqual(p, runstate.RecoveryPlan{}) {
		t.Fatal(p, e, want)
	}
}
func TestDay19Positions(t *testing.T) {
	for _, tc := range []struct {
		phase runstate.Phase
		want  runstate.RecoveryPosition
	}{
		{runstate.Ready, runstate.RecoveryReady}, {runstate.ToolsPending, runstate.RecoveryReviewPending},
		{runstate.Completed, runstate.RecoveryTerminal}, {runstate.Failed, runstate.RecoveryTerminal}, {runstate.Cancelled, runstate.RecoveryTerminal},
	} {
		t.Run(string(tc.phase), func(t *testing.T) {
			v := record19(t, tc.phase)
			s := fake19(v)
			p, e := runstate.PlanRecovery(context.Background(), s, "r19")
			if e != nil || p.Position != tc.want || !reflect.DeepEqual(p.Record, v) || s.loads.Load() != 1 || s.saves.Load() != 0 {
				t.Fatal(p, e)
			}
			if tc.phase == runstate.ToolsPending || tc.phase == runstate.Failed || tc.phase == runstate.Cancelled {
				if len(p.Record.Checkpoint.State.Pending) != 1 {
					t.Fatal("lost pending")
				}
			}
		})
	}
	// Real MemoryStore: reading a plan must not add a save or change Version.
	ctx := context.Background()
	r, e := runstate.NewRun(ctx, "r19", "读取 doc-1", 1)
	if e != nil {
		t.Fatal(e)
	}
	pending19(t, r)
	apply19(t, r, runstate.Event{Kind: runstate.StartTools})
	apply19(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "正文"}}})
	cp, e := r.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	s := runstate.NewMemoryCheckpointStore()
	saved, e := s.Save(ctx, cp, 0)
	if e != nil {
		t.Fatal(e)
	}
	p, e := runstate.PlanRecovery(ctx, s, "r19")
	if e != nil || p.Position != runstate.RecoveryExhausted || p.Record.Version != 1 {
		t.Fatal(p, e)
	}
	loaded, e := s.Load(ctx, "r19")
	if e != nil || !reflect.DeepEqual(saved, loaded) || r.Snapshot().Phase != runstate.Ready {
		t.Fatal(loaded, e)
	}
}
func TestDay19InvalidRecords(t *testing.T) {
	base := record19(t, runstate.ToolsPending)
	mutations := []func(*runstate.StoredCheckpoint){
		func(v *runstate.StoredCheckpoint) { v.Version = 0 },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.RunID = "other" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.MaxSteps = 0 },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Step = -1 },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Step = 99 },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.Boundary = runstate.CheckpointReady },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Phase = runstate.ToolsRunning },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Messages = nil },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Messages[0].Role = "system" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending = nil },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending[0].ID = "wrong" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending[0].ID = "" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending[0].Name = " " },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending[0].Name = "other_tool" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending[0].Arguments = []byte("changed") },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Messages[1].Role = "tool" },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Messages[1].ToolCalls = nil },
		func(v *runstate.StoredCheckpoint) {
			s := &v.Checkpoint.State
			s.Pending = append(s.Pending, s.Pending[0])
			s.Messages[1].ToolCalls = append(s.Messages[1].ToolCalls, s.Messages[1].ToolCalls[0])
		},
	}
	for i, mutate := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			v := base
			v.Checkpoint = base.Checkpoint.Clone()
			mutate(&v)
			s := fake19(v)
			p, e := runstate.PlanRecovery(context.Background(), s, "r19")
			zero19(t, p, e, runstate.ErrInvalidCheckpoint)
			if s.saves.Load() != 0 {
				t.Fatal("wrote bad record")
			}
		})
	}
	for _, phase := range []runstate.Phase{runstate.Ready, runstate.Completed} {
		v := record19(t, phase)
		if phase == runstate.Ready {
			v.Checkpoint.State.Pending = base.Checkpoint.State.Pending
		} else {
			v.Checkpoint.State.Result = "wrong"
		}
		p, e := runstate.PlanRecovery(context.Background(), fake19(v), "r19")
		zero19(t, p, e, runstate.ErrInvalidCheckpoint)
	}
	for _, mutate := range []func(*runstate.StoredCheckpoint){
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Pending = base.Checkpoint.State.Pending },
		func(v *runstate.StoredCheckpoint) { v.Checkpoint.State.Messages[1].Role = "tool" },
		func(v *runstate.StoredCheckpoint) {
			v.Checkpoint.State.Messages[1].ToolCalls = base.Checkpoint.State.Pending
		},
	} {
		v := record19(t, runstate.Completed)
		mutate(&v)
		p, e := runstate.PlanRecovery(context.Background(), fake19(v), "r19")
		zero19(t, p, e, runstate.ErrInvalidCheckpoint)
	}
	ready := record19(t, runstate.Ready)
	ready.Checkpoint.State.Messages[0].ToolCalls = base.Checkpoint.State.Pending
	p, e := runstate.PlanRecovery(context.Background(), fake19(ready), "r19")
	zero19(t, p, e, runstate.ErrInvalidCheckpoint)
}

type fault19 struct{}

func (*fault19) Error() string { return "store unavailable" }
func TestDay19InputAndStoreFailures(t *testing.T) {
	ctx := context.Background()
	v := record19(t, runstate.Ready)
	s := fake19(v)
	p, e := runstate.PlanRecovery(nil, s, "r19")
	zero19(t, p, e, runstate.ErrInvalidCheckpoint)
	p, e = runstate.PlanRecovery(ctx, nil, "r19")
	zero19(t, p, e, runstate.ErrInvalidCheckpoint)
	p, e = runstate.PlanRecovery(ctx, s, " ")
	zero19(t, p, e, runstate.ErrInvalidCheckpoint)
	if s.loads.Load() != 0 {
		t.Fatal("invalid input loaded")
	}
	missing := runstate.NewMemoryCheckpointStore()
	p, e = runstate.PlanRecovery(ctx, missing, "r19")
	zero19(t, p, e, runstate.ErrCheckpointNotFound)
	cause := &fault19{}
	s = &store19{load: func(got context.Context, id string) (runstate.StoredCheckpoint, error) {
		if got != ctx || id != "r19" {
			t.Error("wrong downstream input")
		}
		return v, fmt.Errorf("load: %w", cause)
	}}
	p, e = runstate.PlanRecovery(ctx, s, "r19")
	zero19(t, p, e, cause)
	var typed *fault19
	if !errors.As(e, &typed) || typed != cause || s.loads.Load() != 1 || s.saves.Load() != 0 {
		t.Fatal(e)
	}
}
func TestDay19Cancellation(t *testing.T) {
	v := record19(t, runstate.ToolsPending)
	s := fake19(v)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, finish := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer finish()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{canceled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		p, e := runstate.PlanRecovery(tc.ctx, s, "r19")
		zero19(t, p, e, tc.want)
	}
	if s.loads.Load() != 0 {
		t.Fatal("pre-cancel Load")
	}
	ctx, c := context.WithCancel(context.Background())
	defer c()
	entered := make(chan struct{})
	s = &store19{load: func(got context.Context, id string) (runstate.StoredCheckpoint, error) {
		if got != ctx || id != "r19" {
			t.Error("ctx not forwarded")
		}
		close(entered)
		<-got.Done()
		return v, nil
	}}
	type result struct {
		p runstate.RecoveryPlan
		e error
	}
	done := make(chan result, 1)
	go func() { p, e := runstate.PlanRecovery(ctx, s, "r19"); done <- result{p, e} }()
	<-entered
	c()
	got := <-done
	zero19(t, got.p, got.e, context.Canceled)
	if s.loads.Load() != 1 || s.saves.Load() != 0 {
		t.Fatal("retry or Save")
	}
	// Cancellation observed after Load wins over a simultaneous store error.
	ctx2, c2 := context.WithCancel(context.Background())
	defer c2()
	s = &store19{load: func(context.Context, string) (runstate.StoredCheckpoint, error) { c2(); return v, &fault19{} }}
	p, e := runstate.PlanRecovery(ctx2, s, "r19")
	zero19(t, p, e, context.Canceled)
}
func TestDay19OwnershipAndConcurrency(t *testing.T) {
	v := record19(t, runstate.ToolsPending)
	s := fake19(v)
	ctx := context.Background()
	p, e := runstate.PlanRecovery(ctx, s, "r19")
	if e != nil {
		t.Fatal(e)
	}
	p.Record.Checkpoint.State.Pending[0].Arguments[0] = 'p'
	if p.Record.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("history aliases Pending")
	}
	p.Record.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] = 'm'
	if v.Checkpoint.State.Pending[0].Arguments[0] != '{' || v.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("plan aliases Load buffers")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := runstate.PlanRecovery(ctx, s, "r19")
			if e != nil {
				t.Error(e)
				return
			}
			if p.Position != runstate.RecoveryReviewPending || p.Record.Version != 7 {
				t.Error(p)
			}
			p.Record.Checkpoint.State.Pending[0].Arguments[0] = 'x'
		}()
	}
	wg.Wait()
	if s.loads.Load() != 17 || s.saves.Load() != 0 || v.Checkpoint.State.Pending[0].Arguments[0] != '{' {
		t.Fatal("ownership/counts")
	}
}
