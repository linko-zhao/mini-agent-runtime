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

func gate22(t *testing.T, ctx context.Context) *runstate.PauseGate {
	t.Helper()
	r, e := runstate.NewRun(ctx, "r22", "读取 doc-1", 3)
	if e != nil {
		t.Fatal(e)
	}
	g, e := runstate.NewPauseGate(r)
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func apply22(t *testing.T, g *runstate.PauseGate, e runstate.Event) runstate.State {
	t.Helper()
	s, err := g.Apply(e)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func pending22(t *testing.T, g *runstate.PauseGate) {
	t.Helper()
	apply22(t, g, runstate.Event{Kind: runstate.StartModel})
	apply22(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte(`{"id":"doc-1"}`)}}}}})
}
func zeroPause22(t *testing.T, r runstate.PauseRequest, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || !reflect.DeepEqual(r, runstate.PauseRequest{}) {
		t.Fatal(r, e, want)
	}
}

// Host-side fixture: the mock effect happens only after an accepted StartTools.
func effect22(g *runstate.PauseGate, calls *atomic.Int32) error {
	if _, e := g.Apply(runstate.Event{Kind: runstate.StartTools}); e != nil {
		return e
	}
	calls.Add(1)
	return nil
}
func TestDay22OpenGate(t *testing.T) {
	if g, e := runstate.NewPauseGate(nil); g != nil || !errors.Is(e, runstate.ErrInvalidPause) {
		t.Fatal(g, e)
	}
	g := gate22(t, context.Background())
	initial := g.Snapshot()
	if v, ok := g.CurrentInterrupt(); ok || !reflect.DeepEqual(v, runstate.PauseRequest{}) {
		t.Fatal(v, ok)
	}
	if initial.Phase != runstate.Ready || initial.Step != 0 {
		t.Fatal(initial)
	}
	s, e := g.Apply(runstate.Event{Kind: runstate.StartTools})
	if !errors.Is(e, runstate.ErrTransition) || !reflect.DeepEqual(s, initial) {
		t.Fatal(s, e)
	}
	s = apply22(t, g, runstate.Event{Kind: runstate.StartModel})
	if s.Step != 1 {
		t.Fatal(s)
	}
	s = apply22(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "完成"}}})
	if s.Phase != runstate.Completed || s.Result != "完成" {
		t.Fatal(s)
	}
}
func TestDay22PauseBlocksEffects(t *testing.T) {
	g := gate22(t, context.Background())
	pending22(t, g)
	before := g.Snapshot()
	// A preexisting saved record is not modified by an in-memory pause.
	cp := runstate.Checkpoint{Boundary: runstate.CheckpointBeforeTools, State: before}
	store := runstate.NewMemoryCheckpointStore()
	saved, e := store.Save(context.Background(), cp, 0)
	if e != nil {
		t.Fatal(e)
	}
	request, e := g.Pause(" pause-1 ", " waiting-for-host ")
	if e != nil || request.ID != " pause-1 " || request.Reason != " waiting-for-host " || request.Checkpoint.Boundary != runstate.CheckpointBeforeTools || !reflect.DeepEqual(request.Checkpoint.State, before) {
		t.Fatal(request, e)
	}
	var effects atomic.Int32
	if e = effect22(g, &effects); !errors.Is(e, runstate.ErrPaused) || effects.Load() != 0 {
		t.Fatal(e, effects.Load())
	}
	for _, event := range []runstate.Event{{Kind: runstate.StartModel}, {Kind: runstate.StartTools}, {Kind: runstate.Abort, Reason: "stop"}, {Kind: runstate.Cancel}} {
		s, e := g.Apply(event)
		if !errors.Is(e, runstate.ErrPaused) || !reflect.DeepEqual(s, before) {
			t.Fatal(s, e)
		}
	}
	duplicate, e := g.Pause("other", "reason")
	zeroPause22(t, duplicate, e, runstate.ErrPaused)
	after, e := store.Load(context.Background(), "r22")
	if e != nil || !reflect.DeepEqual(after, saved) || !reflect.DeepEqual(g.Snapshot(), before) {
		t.Fatal(after, e)
	}
	current, ok := g.CurrentInterrupt()
	if !ok || current.ID != request.ID {
		t.Fatal(current, ok)
	}
}
func TestDay22MetadataAndBoundaries(t *testing.T) {
	for _, row := range []struct{ id, reason string }{{"", "reason"}, {" ", "reason"}, {"id", ""}, {"id", " "}} {
		g := gate22(t, context.Background())
		pending22(t, g)
		before := g.Snapshot()
		v, e := g.Pause(row.id, row.reason)
		zeroPause22(t, v, e, runstate.ErrInvalidPause)
		if _, ok := g.CurrentInterrupt(); ok || !reflect.DeepEqual(before, g.Snapshot()) {
			t.Fatal("invalid request mutated gate")
		}
	}
	for _, phase := range []runstate.Phase{runstate.Ready, runstate.ModelRunning, runstate.ToolsRunning, runstate.Completed, runstate.Failed, runstate.Cancelled} {
		t.Run(string(phase), func(t *testing.T) {
			g := gate22(t, context.Background())
			switch phase {
			case runstate.ModelRunning:
				apply22(t, g, runstate.Event{Kind: runstate.StartModel})
			case runstate.ToolsRunning:
				pending22(t, g)
				apply22(t, g, runstate.Event{Kind: runstate.StartTools})
			case runstate.Completed:
				apply22(t, g, runstate.Event{Kind: runstate.StartModel})
				apply22(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}})
			case runstate.Failed:
				apply22(t, g, runstate.Event{Kind: runstate.Abort, Reason: "controlled"})
			case runstate.Cancelled:
				apply22(t, g, runstate.Event{Kind: runstate.Cancel})
			}
			before := g.Snapshot()
			v, e := g.Pause("p", "reason")
			zeroPause22(t, v, e, runstate.ErrPauseBoundary)
			if !reflect.DeepEqual(before, g.Snapshot()) {
				t.Fatal("boundary rejection changed state")
			}
			if _, ok := g.CurrentInterrupt(); ok {
				t.Fatal("request created at wrong boundary")
			}
		})
	}
}

// Controlled deadline error after cancellation; no wall-clock sleeps.
type deadline22 struct{ context.Context }

func (d deadline22) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay22Cancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			var ctx context.Context = base
			want := error(context.Canceled)
			if deadline {
				ctx = deadline22{base}
				want = context.DeadlineExceeded
			}
			g := gate22(t, ctx)
			pending22(t, g)
			request, e := g.Pause("p", "wait")
			if e != nil {
				t.Fatal(e)
			}
			cancel()
			if g.Snapshot().Phase != runstate.ToolsPending {
				t.Fatal("snapshot changed state")
			}
			if _, ok := g.CurrentInterrupt(); !ok {
				t.Fatal("read unexpectedly consumed pause")
			}
			v, e := g.Pause("", "")
			zeroPause22(t, v, e, want) // context wins before metadata checks
			s, e := g.Apply(runstate.Event{Kind: runstate.StartTools})
			if !errors.Is(e, want) || s.Phase != runstate.Cancelled || s.Failure != "cancelled" || len(s.Pending) != 1 {
				t.Fatal(s, e)
			}
			if v, ok := g.CurrentInterrupt(); ok || !reflect.DeepEqual(v, runstate.PauseRequest{}) {
				t.Fatal(v, ok)
			}
			if request.Checkpoint.State.Phase != runstate.ToolsPending {
				t.Fatal("old owned request changed")
			}
			s, e = g.Apply(runstate.Event{Kind: runstate.StartTools})
			if !errors.Is(e, runstate.ErrTransition) || s.Phase != runstate.Cancelled {
				t.Fatal(s, e)
			}
		})
	}
	// Cancellation before the first Pause creates no request.
	ctx, cancel := context.WithCancel(context.Background())
	g := gate22(t, ctx)
	pending22(t, g)
	cancel()
	v, e := g.Pause("p", "reason")
	zeroPause22(t, v, e, context.Canceled)
	if _, ok := g.CurrentInterrupt(); ok {
		t.Fatal("created a cancelled pause")
	}
	// Existing terminal states keep Run.Apply's terminal-first behavior.
	ctx, cancel = context.WithCancel(context.Background())
	g = gate22(t, ctx)
	apply22(t, g, runstate.Event{Kind: runstate.StartModel})
	apply22(t, g, runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}})
	cancel()
	s, e := g.Apply(runstate.Event{Kind: runstate.Cancel})
	if !errors.Is(e, runstate.ErrTransition) || s.Phase != runstate.Completed {
		t.Fatal(s, e)
	}
}
func TestDay22OwnershipAndConcurrency(t *testing.T) {
	g := gate22(t, context.Background())
	pending22(t, g)
	before := g.Snapshot()
	request, e := g.Pause("p", "review")
	if e != nil {
		t.Fatal(e)
	}
	request.Checkpoint.State.Pending[0].Arguments[0] = 'p'
	request.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] = 'h'
	current, ok := g.CurrentInterrupt()
	if !ok || !reflect.DeepEqual(current.Checkpoint.State, before) {
		t.Fatal(current, ok)
	}
	current.Checkpoint.State.Pending[0].Arguments[0] = 'c'
	snapshot := g.Snapshot()
	snapshot.Messages[1].ToolCalls[0].Arguments[0] = 's'
	if !reflect.DeepEqual(before, g.Snapshot()) {
		t.Fatal("owned buffers alias gate")
	}
	other := gate22(t, context.Background())
	pending22(t, other)
	var wg sync.WaitGroup
	var wins atomic.Int32
	winner := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, e := other.Pause(fmt.Sprintf("p-%d", i), "review")
			if e == nil {
				wins.Add(1)
				winner <- v.ID
				v.Checkpoint.State.Pending[0].Arguments[0] = 'x'
			} else if !errors.Is(e, runstate.ErrPaused) {
				t.Error(e)
			}
			v, ok := other.CurrentInterrupt()
			if !ok {
				t.Error("missing request")
				return
			}
			v.Checkpoint.State.Pending[0].Arguments[0] = 'z'
			s := other.Snapshot()
			s.Messages[1].ToolCalls[0].Arguments[0] = 'r'
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal(wins.Load())
	}
	got, ok := other.CurrentInterrupt()
	if !ok || got.ID != <-winner || got.Checkpoint.State.Pending[0].Arguments[0] != '{' {
		t.Fatal(got)
	}
}
func TestDay22PauseVersusStart(t *testing.T) {
	for i := 0; i < 32; i++ {
		g := gate22(t, context.Background())
		pending22(t, g)
		start := make(chan struct{})
		paused := make(chan error, 1)
		started := make(chan error, 1)
		var effects atomic.Int32
		go func() { <-start; _, e := g.Pause("p", "review"); paused <- e }()
		go func() { <-start; started <- effect22(g, &effects) }()
		close(start)
		pauseErr, startErr := <-paused, <-started
		if pauseErr == nil {
			if !errors.Is(startErr, runstate.ErrPaused) || effects.Load() != 0 || g.Snapshot().Phase != runstate.ToolsPending {
				t.Fatal(pauseErr, startErr, effects.Load())
			}
		} else {
			if !errors.Is(pauseErr, runstate.ErrPauseBoundary) || startErr != nil || effects.Load() != 1 || g.Snapshot().Phase != runstate.ToolsRunning {
				t.Fatal(pauseErr, startErr, effects.Load())
			}
		}
	}
}
