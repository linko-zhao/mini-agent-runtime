package runstate_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/runstate"
)

func mustRun16(t *testing.T, ctx context.Context, id string, budget int) *runstate.Run {
	t.Helper()
	r, err := runstate.NewRun(ctx, id, "读取 doc-1", budget)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func apply16(t *testing.T, r *runstate.Run, e runstate.Event) runstate.State {
	t.Helper()
	s, err := r.Apply(e)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func response16() *model.Response {
	return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}
}
func TestDay16NewAndContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), struct{}{}, "trusted"))
	defer cancel()
	r := mustRun16(t, ctx, "r16", 2)
	if r.Context() != ctx {
		t.Fatal("context was replaced")
	}
	s := r.Snapshot()
	if s.RunID != "r16" || s.Step != 0 || s.MaxSteps != 2 || s.Phase != runstate.Ready || s.Messages[0].Content != "读取 doc-1" {
		t.Fatal(s)
	}
	for _, tc := range []struct {
		ctx    context.Context
		id     string
		budget int
		want   error
	}{
		{nil, "r", 2, runstate.ErrTransition},
		{context.Background(), " ", 2, runstate.ErrTransition},
		{context.Background(), "r", 0, runstate.ErrTransition},
	} {
		r, e := runstate.NewRun(tc.ctx, tc.id, "", tc.budget)
		if r != nil || !errors.Is(e, tc.want) {
			t.Fatal(r, e)
		}
	}
	cancel()
	if r, e := runstate.NewRun(ctx, "r", "", 2); r != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(r, e)
	}
}

// Empty input remains legal, as in Day15; no new input restriction.
func TestDay16RoundTrip(t *testing.T) {
	r, err := runstate.NewRun(context.Background(), "r", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	start := apply16(t, r, runstate.Event{Kind: runstate.StartModel})
	if start.Step != 1 || start.Phase != runstate.ModelRunning {
		t.Fatal(start)
	}
	pending := apply16(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response16()})
	if pending.Phase != runstate.ToolsPending || pending.Pending[0].ID != "call-1" {
		t.Fatal(pending)
	}
	apply16(t, r, runstate.Event{Kind: runstate.StartTools})
	ready := apply16(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "正文"}}})
	if ready.Phase != runstate.Ready || len(ready.Pending) != 0 || len(ready.Messages) != 3 {
		t.Fatal(ready)
	}
	apply16(t, r, runstate.Event{Kind: runstate.StartModel})
	done := apply16(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "已读取"}}})
	if done.Phase != runstate.Completed || done.Step != 2 || done.Result != "已读取" || len(done.Messages) != 4 {
		t.Fatal(done)
	}
	before := r.Snapshot()
	got, e := r.Apply(runstate.Event{Kind: runstate.Cancel})
	if !errors.Is(e, runstate.ErrTransition) || !reflect.DeepEqual(before, got) {
		t.Fatal(got, e)
	}
}
func TestDay16RejectedAndOwned(t *testing.T) {
	r := mustRun16(t, context.Background(), "r", 1)
	before := r.Snapshot()
	got, e := r.Apply(runstate.Event{Kind: runstate.StartTools})
	if !errors.Is(e, runstate.ErrTransition) || !reflect.DeepEqual(before, got) || !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal(got, e)
	}
	apply16(t, r, runstate.Event{Kind: runstate.StartModel})
	response := response16()
	returned := apply16(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response})
	response.Message.ToolCalls[0].Arguments[0] = 'x'
	returned.Pending[0].Arguments[0] = 'y'
	returned.Messages[1].ToolCalls[0].Arguments[0] = 'z'
	snap := r.Snapshot()
	snap.Pending[0].Arguments[0] = '!'
	if r.Snapshot().Pending[0].Arguments[0] != '{' || r.Snapshot().Messages[1].ToolCalls[0].Arguments[0] != '{' {
		t.Fatal("alias")
	}
	apply16(t, r, runstate.Event{Kind: runstate.StartTools})
	before = r.Snapshot()
	got, e = r.Apply(runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "wrong"}}})
	if !errors.Is(e, runstate.ErrTransition) || !reflect.DeepEqual(before, got) {
		t.Fatal(got, e)
	}
	apply16(t, r, runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1"}}})
	before = r.Snapshot()
	got, e = r.Apply(runstate.Event{Kind: runstate.StartModel})
	if !errors.Is(e, runstate.ErrMaxSteps) || !reflect.DeepEqual(before, got) {
		t.Fatal(got, e)
	}
	failed := apply16(t, r, runstate.Event{Kind: runstate.Abort, Reason: "budget"})
	if failed.Phase != runstate.Failed || failed.Failure != "budget" {
		t.Fatal(failed)
	}
}
func TestDay16Cancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			var ctx context.Context = base
			want := error(context.Canceled)
			if deadline {
				ctx = deadline16{base}
				want = context.DeadlineExceeded
			}
			defer cancel()
			r := mustRun16(t, ctx, "cancel", 2)
			apply16(t, r, runstate.Event{Kind: runstate.StartModel})
			cancel()
			if r.Snapshot().Phase != runstate.ModelRunning {
				t.Fatal("read must not transition")
			}
			got, e := r.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: response16()})
			if !errors.Is(e, want) || got.Phase != runstate.Cancelled || len(got.Messages) != 1 || got.Failure != "cancelled" {
				t.Fatal(got, e)
			}
			if _, e = r.Apply(runstate.Event{Kind: runstate.Cancel}); !errors.Is(e, runstate.ErrTransition) {
				t.Fatal(e)
			}
		})
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	if r, e := runstate.NewRun(expired, "r", "", 2); r != nil || !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(r, e)
	}
	ctx, c := context.WithCancel(context.Background())
	r := mustRun16(t, ctx, "terminal", 1)
	apply16(t, r, runstate.Event{Kind: runstate.StartModel})
	done := apply16(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "ok"}}})
	c()
	got, e := r.Apply(runstate.Event{Kind: runstate.Cancel})
	if !errors.Is(e, runstate.ErrTransition) || !reflect.DeepEqual(done, got) {
		t.Fatal(got, e)
	}
}

// Controlled deadline fixture: Err switches only after the underlying cancel.
// It tests deadline propagation without wall-clock sleeps.
type deadline16 struct{ context.Context }

func (d deadline16) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay16ConcurrentUpdates(t *testing.T) {
	r := mustRun16(t, context.Background(), "shared", 2)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := r.Apply(runstate.Event{Kind: runstate.StartModel})
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, runstate.ErrTransition) {
				t.Error(e)
			}
			s.Messages[0].Content = "private"
			snap := r.Snapshot()
			snap.Messages[0].Content = "reader"
		}()
	}
	wg.Wait()
	s := r.Snapshot()
	if wins.Load() != 1 || s.Step != 1 || s.Phase != runstate.ModelRunning || s.Messages[0].Content != "读取 doc-1" {
		t.Fatal(wins.Load(), s)
	}
	other := mustRun16(t, context.Background(), "other", 2)
	if other.Snapshot().Phase != runstate.Ready {
		t.Fatal("Run isolation")
	}
}

func TestDay16ToolsAndExplicitCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := mustRun16(t, ctx, "tools", 2)
	apply16(t, r, runstate.Event{Kind: runstate.StartModel})
	apply16(t, r, runstate.Event{Kind: runstate.ModelReturned, Response: response16()})
	apply16(t, r, runstate.Event{Kind: runstate.StartTools})
	// The host has entered I/O. Cancellation cannot undo external effects.
	seen := r.Context()
	if seen != ctx {
		t.Fatal("downstream must receive original context")
	}
	cancel()
	<-seen.Done()
	got, e := r.Apply(runstate.Event{Kind: runstate.ToolsReturned, Results: []model.Message{{Role: "tool", ToolCallID: "call-1", Content: "late"}}})
	if !errors.Is(e, context.Canceled) || got.Phase != runstate.Cancelled || len(got.Messages) != 2 || len(got.Pending) != 1 {
		t.Fatal(got, e)
	}
	// Explicit cancellation of a live Run is a state event, not context cancellation.
	live := mustRun16(t, context.Background(), "explicit", 1)
	s := apply16(t, live, runstate.Event{Kind: runstate.Cancel})
	if s.Phase != runstate.Cancelled || live.Context().Err() != nil {
		t.Fatal(s)
	}
}
