package runstate_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

func candidate18(t *testing.T, id string) runstate.Checkpoint {
	t.Helper()
	r, e := runstate.NewRun(context.Background(), id, "读取 doc-1", 2)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Apply(runstate.Event{Kind: runstate.StartModel}); e != nil {
		t.Fatal(e)
	}
	response := &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}
	if _, e = r.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: response}); e != nil {
		t.Fatal(e)
	}
	cp, e := r.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	return cp
}
func error18(t *testing.T, v runstate.StoredCheckpoint, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(v, runstate.StoredCheckpoint{}) {
		t.Fatal(v, err, want)
	}
}
func TestDay18CreateLoadCAS(t *testing.T) {
	ctx := context.Background()
	var store runstate.CheckpointStore = runstate.NewMemoryCheckpointStore()
	cp := candidate18(t, "r")
	v, e := store.Load(ctx, "r")
	error18(t, v, e, runstate.ErrCheckpointNotFound)
	v, e = store.Save(ctx, cp, 0)
	if e != nil || v.Version != 1 || !reflect.DeepEqual(v.Checkpoint, cp) {
		t.Fatal(v, e)
	}
	v, e = store.Save(ctx, cp, 1)
	if e != nil || v.Version != 2 || v.Checkpoint.State.Step != 1 {
		t.Fatal(v, e)
	}
	latest := v
	for _, old := range []uint64{0, 1, 9} {
		v, e = store.Save(ctx, cp, old)
		error18(t, v, e, runstate.ErrCheckpointConflict)
	}
	loaded, e := store.Load(ctx, "r")
	if e != nil || !reflect.DeepEqual(latest, loaded) {
		t.Fatal(loaded, e)
	}
	cp2 := candidate18(t, "other")
	v, e = store.Save(ctx, cp2, 0)
	if e != nil || v.Version != 1 {
		t.Fatal(v, e)
	}
	// Zero-value store is usable and a new instance has no persisted records.
	var zero runstate.MemoryCheckpointStore
	v, e = zero.Load(ctx, "r")
	error18(t, v, e, runstate.ErrCheckpointNotFound)
	v, e = zero.Save(ctx, cp, 0)
	if e != nil || v.Version != 1 {
		t.Fatal(v, e)
	}
	// RunID is an exact key; nonempty surrounding spaces are not normalized.
	spaced := candidate18(t, " r ")
	v, e = store.Save(ctx, spaced, 0)
	if e != nil || v.Version != 1 {
		t.Fatal(v, e)
	}
	loaded, e = store.Load(ctx, "r")
	if e != nil || loaded.Version != 2 {
		t.Fatal(loaded, e)
	}
	v, e = store.Save(ctx, candidate18(t, "absent"), 4)
	error18(t, v, e, runstate.ErrCheckpointConflict)
	v, e = store.Load(ctx, "absent")
	error18(t, v, e, runstate.ErrCheckpointNotFound)
}
func TestDay18Validation(t *testing.T) {
	ctx := context.Background()
	s := runstate.NewMemoryCheckpointStore()
	cp := candidate18(t, "r")
	if _, e := s.Save(ctx, cp, 0); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*runstate.Checkpoint){
		func(c *runstate.Checkpoint) { c.State.RunID = " " },
		func(c *runstate.Checkpoint) { c.State.MaxSteps = 0 },
		func(c *runstate.Checkpoint) { c.State.Step = -1 },
		func(c *runstate.Checkpoint) { c.State.Step = c.State.MaxSteps + 1 },
		func(c *runstate.Checkpoint) { c.Boundary = "unknown" },
		func(c *runstate.Checkpoint) { c.State.Phase = runstate.ToolsRunning },
		func(c *runstate.Checkpoint) { c.Boundary = runstate.CheckpointReady },
	}
	for _, change := range mutations {
		bad := cp.Clone()
		change(&bad)
		v, e := s.Save(ctx, bad, 1)
		error18(t, v, e, runstate.ErrInvalidCheckpoint)
	}
	v, e := s.Save(ctx, cp, ^uint64(0))
	error18(t, v, e, runstate.ErrInvalidCheckpoint)
	for _, id := range []string{"", "  "} {
		v, e = s.Load(ctx, id)
		error18(t, v, e, runstate.ErrInvalidCheckpoint)
	}
	v, e = s.Load(nil, "r")
	error18(t, v, e, runstate.ErrInvalidCheckpoint)
	v, e = s.Save(nil, cp, 1)
	error18(t, v, e, runstate.ErrInvalidCheckpoint)
	v, e = s.Load(ctx, "r")
	if e != nil || v.Version != 1 || !reflect.DeepEqual(v.Checkpoint, cp) {
		t.Fatal(v, e)
	}
	// All Day17 stable classes are valid store envelopes.
	for _, phase := range []runstate.Phase{runstate.Ready, runstate.Completed, runstate.Failed, runstate.Cancelled} {
		r, e := runstate.NewRun(ctx, string(phase), "", 2)
		if e != nil {
			t.Fatal(e)
		}
		switch phase {
		case runstate.Completed:
			r.Apply(runstate.Event{Kind: runstate.StartModel})
			if _, e = r.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "ok"}}}); e != nil {
				t.Fatal(e)
			}
		case runstate.Failed:
			if _, e = r.Apply(runstate.Event{Kind: runstate.Abort, Reason: "controlled"}); e != nil {
				t.Fatal(e)
			}
		case runstate.Cancelled:
			if _, e = r.Apply(runstate.Event{Kind: runstate.Cancel}); e != nil {
				t.Fatal(e)
			}
		}
		c, e := r.Checkpoint()
		if e != nil {
			t.Fatal(e)
		}
		v, e = s.Save(ctx, c, 0)
		if e != nil || v.Checkpoint.State.Phase != phase {
			t.Fatal(v, e)
		}
	}
}
func TestDay18Cancellation(t *testing.T) {
	s := runstate.NewMemoryCheckpointStore()
	cp := candidate18(t, "r")
	ctx := context.Background()
	if _, e := s.Save(ctx, cp, 0); e != nil {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	expired, finish := context.WithDeadline(ctx, time.Unix(1, 0))
	defer finish()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{canceled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		v, e := s.Load(tc.ctx, "r")
		error18(t, v, e, tc.want)
		v, e = s.Save(tc.ctx, cp, 1)
		error18(t, v, e, tc.want)
		v, e = s.Save(tc.ctx, candidate18(t, "new"), 0)
		error18(t, v, e, tc.want)
	}
	v, e := s.Load(ctx, "r")
	if e != nil || v.Version != 1 {
		t.Fatal(v, e)
	}
	v, e = s.Load(ctx, "new")
	error18(t, v, e, runstate.ErrCheckpointNotFound)
	// A stopped Run can be archived with a separate live storage context.
	runctx, c := context.WithCancel(ctx)
	r, e := runstate.NewRun(runctx, "archive", "", 2)
	if e != nil {
		t.Fatal(e)
	}
	c()
	if _, e = r.Apply(runstate.Event{Kind: runstate.StartModel}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	terminal, e := r.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Save(ctx, terminal, 0)
	if e != nil || v.Checkpoint.State.Phase != runstate.Cancelled {
		t.Fatal(v, e)
	}
}
func TestDay18Ownership(t *testing.T) {
	s := runstate.NewMemoryCheckpointStore()
	cp := candidate18(t, "r")
	want := cp.Clone()
	ctx := context.Background()
	saved, e := s.Save(ctx, cp, 0)
	if e != nil {
		t.Fatal(e)
	}
	cp.State.Pending[0].Arguments[0] = 'p'
	cp.State.Messages[1].ToolCalls[0].Arguments[0] = 'm'
	saved.Checkpoint.State.Pending[0].Arguments[0] = 's'
	saved.Checkpoint.State.Messages[0].Content = "changed"
	loaded, e := s.Load(ctx, "r")
	if e != nil || !reflect.DeepEqual(loaded.Checkpoint, want) {
		t.Fatal(loaded, e)
	}
	loaded.Checkpoint.State.Pending[0].Arguments[0] = 'l'
	if loaded.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("history aliases Pending")
	}
	loaded.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] = 'x'
	second, e := s.Load(ctx, "r")
	if e != nil || !reflect.DeepEqual(second.Checkpoint, want) {
		t.Fatal(second, e)
	}
}
func TestDay18ConcurrentCAS(t *testing.T) {
	ctx := context.Background()
	s := runstate.NewMemoryCheckpointStore()
	cp := candidate18(t, "r")
	for _, expected := range []uint64{0, 1} {
		var wins atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for n := 0; n < 16; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				v, e := s.Save(ctx, cp, expected)
				if e == nil {
					wins.Add(1)
					if v.Version != expected+1 {
						t.Error(v)
					}
				} else if !errors.Is(e, runstate.ErrCheckpointConflict) {
					t.Error(e)
				}
				v, e = s.Load(ctx, "r")
				if e != nil {
					t.Error(e)
					return
				}
				if v.Version != expected+1 || v.Checkpoint.State.RunID != "r" {
					t.Error(v)
				}
				v.Checkpoint.State.Pending[0].Arguments[0] = 'x'
			}()
		}
		close(start)
		wg.Wait()
		v, e := s.Load(ctx, "r")
		if wins.Load() != 1 || e != nil || v.Version != expected+1 || v.Checkpoint.State.Pending[0].Arguments[0] != '{' {
			t.Fatal(wins.Load(), v, e)
		}
	}
}
