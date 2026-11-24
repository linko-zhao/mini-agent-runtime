package runstate_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

func gate25(t *testing.T, ctx context.Context, paused bool) *runstate.PauseGate {
	t.Helper()
	r, e := runstate.NewRun(ctx, "r25", "读取 doc-1", 3)
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
		response := model.Response{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_document", Arguments: []byte("{ \"id\" : \"doc-1\" }\n")}}}}
		if _, e = g.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: &response}); e != nil {
			t.Fatal(e)
		}
		if _, e = g.Pause(" pause-1 ", " waiting-for-host "); e != nil {
			t.Fatal(e)
		}
	}
	return g
}
func decide25(t *testing.T, g *runstate.PauseGate, outcome runstate.DecisionOutcome) {
	t.Helper()
	_, e := g.Decide(runstate.ApprovalDecision{RunID: "r25", RequestID: " pause-1 ", Actor: " reviewer-1 ", Outcome: outcome, Reason: " reviewed doc-1 "})
	if e != nil {
		t.Fatal(e)
	}
}
func snapshot25(t *testing.T, g *runstate.PauseGate) runstate.PauseSnapshot {
	t.Helper()
	s, e := g.PauseSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func raw25(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	if e := gob.NewEncoder(&buf).Encode(v); e != nil {
		t.Fatal(e)
	}
	return buf.Bytes()
}
func zeroSnapshot25(t *testing.T, s runstate.PauseSnapshot, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || !reflect.DeepEqual(s, runstate.PauseSnapshot{}) {
		t.Fatal(s, e, want)
	}
}
func TestDay25CaptureDoesNotAdvance(t *testing.T) {
	for _, outcome := range []runstate.DecisionOutcome{"", runstate.DecisionApprove, runstate.DecisionReject} {
		t.Run(string(outcome), func(t *testing.T) {
			g := gate25(t, context.Background(), true)
			if outcome != "" {
				decide25(t, g, outcome)
			}
			before := g.Snapshot()
			request, _ := g.CurrentInterrupt()
			decision, recorded := g.CurrentDecision()
			s := snapshot25(t, g)
			if s.SchemaVersion != runstate.PauseSnapshotSchema || !reflect.DeepEqual(s.Request, request) || (s.Decision != nil) != recorded {
				t.Fatal(s)
			}
			if recorded && *s.Decision != decision {
				t.Fatal(s.Decision)
			}
			if !reflect.DeepEqual(before, g.Snapshot()) {
				t.Fatal("capture advanced run")
			}
			if _, e := g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, runstate.ErrPaused) {
				t.Fatal(e)
			}
		})
	}
}
func TestDay25Ownership(t *testing.T) {
	g := gate25(t, context.Background(), true)
	decide25(t, g, runstate.DecisionApprove)
	s := snapshot25(t, g)
	original := s.Clone()
	clone := s.Clone()
	clone.Request.Checkpoint.State.Pending[0].Arguments[0] = 'p'
	clone.Request.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] = 'h'
	clone.Decision.Actor = "changed clone"
	if !reflect.DeepEqual(s, original) {
		t.Fatal("Clone aliases input")
	}
	s.Decision.Reason = "changed capture"
	s.Request.Checkpoint.State.Pending[0].Arguments[0] = 'c'
	if got := snapshot25(t, g); !reflect.DeepEqual(got, original) {
		t.Fatal("capture aliases gate")
	}
	if copy := (runstate.PauseSnapshot{}).Clone(); !reflect.DeepEqual(copy, runstate.PauseSnapshot{}) {
		t.Fatal(copy)
	}
}
func TestDay25CodecRoundTrip(t *testing.T) {
	for _, outcome := range []runstate.DecisionOutcome{"", runstate.DecisionApprove, runstate.DecisionReject} {
		g := gate25(t, context.Background(), true)
		if outcome != "" {
			decide25(t, g, outcome)
		}
		s := snapshot25(t, g)
		// RawMessage bytes are not JSON-decoded or reformatted by this codec.
		for _, args := range [][]byte{[]byte("{ \"id\" : \"doc-1\" }\n"), {0xff, 0, 1}} {
			s = s.Clone()
			s.Request.Checkpoint.State.Pending[0].Arguments = append([]byte(nil), args...)
			s.Request.Checkpoint.State.Messages[1].ToolCalls[0].Arguments = append([]byte(nil), args...)
			data, e := runstate.EncodePauseSnapshot(s)
			if e != nil {
				t.Fatal(e)
			}
			got, e := runstate.DecodePauseSnapshot(data)
			if e != nil || !reflect.DeepEqual(s, got) {
				t.Fatal(got, e)
			}
			got.Request.Checkpoint.State.Pending[0].Arguments[0] = 'x'
			if got.Decision != nil {
				got.Decision.Actor = "changed"
			}
			again, e := runstate.DecodePauseSnapshot(data)
			if e != nil || !reflect.DeepEqual(s, again) {
				t.Fatal(again, e)
			}
			for i := range data {
				data[i] = 0
			}
			if !bytes.Equal(again.Request.Checkpoint.State.Pending[0].Arguments, args) {
				t.Fatal("decoded data aliases input bytes")
			}
		}
	}
}
func TestDay25InvalidEnvelope(t *testing.T) {
	g := gate25(t, context.Background(), true)
	decide25(t, g, runstate.DecisionApprove)
	base := snapshot25(t, g)
	rows := []struct {
		name   string
		change func(*runstate.PauseSnapshot)
	}{
		{"schema_zero", func(s *runstate.PauseSnapshot) { s.SchemaVersion = 0 }},
		{"schema_future", func(s *runstate.PauseSnapshot) { s.SchemaVersion = 2 }},
		{"request_id", func(s *runstate.PauseSnapshot) { s.Request.ID = " " }},
		{"request_reason", func(s *runstate.PauseSnapshot) { s.Request.Reason = "" }},
		{"run_id", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.RunID = " " }},
		{"budget", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.MaxSteps = 0 }},
		{"step", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Step = 4 }},
		{"boundary", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.Boundary = runstate.CheckpointReady }},
		{"phase", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Phase = runstate.ToolsRunning }},
		{"result", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Result = "done" }},
		{"failure", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Failure = "failed" }},
		{"messages", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Messages = nil }},
		{"first_role", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Messages[0].Role = "tool" }},
		{"pending_empty", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Pending = nil }},
		{"last_role", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Messages[1].Role = "user" }},
		{"last_calls", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Messages[1].ToolCalls = nil }},
		{"call_id", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Pending[0].ID = "" }},
		{"call_name", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Pending[0].Name = " " }},
		{"call_args", func(s *runstate.PauseSnapshot) { s.Request.Checkpoint.State.Pending[0].Arguments = []byte("other") }},
		{"duplicate_call", func(s *runstate.PauseSnapshot) {
			st := &s.Request.Checkpoint.State
			st.Pending = append(st.Pending, st.Pending[0])
			st.Messages[1].ToolCalls = append(st.Messages[1].ToolCalls, st.Messages[1].ToolCalls[0])
		}},
		{"decision_run", func(s *runstate.PauseSnapshot) { s.Decision.RunID = "other" }},
		{"decision_request", func(s *runstate.PauseSnapshot) { s.Decision.RequestID = "pause-1" }},
		{"decision_actor", func(s *runstate.PauseSnapshot) { s.Decision.Actor = " " }},
		{"decision_reason", func(s *runstate.PauseSnapshot) { s.Decision.Reason = "" }},
		{"decision_outcome", func(s *runstate.PauseSnapshot) { s.Decision.Outcome = "APPROVE" }},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			s := base.Clone()
			row.change(&s)
			if data, e := runstate.EncodePauseSnapshot(s); !errors.Is(e, runstate.ErrInvalidPauseSnapshot) || data != nil {
				t.Fatal(data, e)
			}
			got, e := runstate.DecodePauseSnapshot(raw25(t, s))
			zeroSnapshot25(t, got, e, runstate.ErrInvalidPauseSnapshot)
		})
	}
	// An old Checkpoint contains no request/decision/schema; do not invent them.
	got, e := runstate.DecodePauseSnapshot(raw25(t, base.Request.Checkpoint))
	zeroSnapshot25(t, got, e, runstate.ErrInvalidPauseSnapshot)
}
func TestDay25MalformedAndSize(t *testing.T) {
	s := snapshot25(t, gate25(t, context.Background(), true))
	data, e := runstate.EncodePauseSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	for _, input := range [][]byte{nil, {}, []byte("not gob"), data[:len(data)-1], append(append([]byte(nil), data...), 0), append(append([]byte(nil), data...), data...), make([]byte, runstate.MaxPauseSnapshotBytes+1)} {
		got, e := runstate.DecodePauseSnapshot(input)
		zeroSnapshot25(t, got, e, runstate.ErrInvalidPauseSnapshot)
	}
	s.Request.Checkpoint.State.Messages[0].Content = strings.Repeat("x", runstate.MaxPauseSnapshotBytes)
	if data, e := runstate.EncodePauseSnapshot(s); !errors.Is(e, runstate.ErrInvalidPauseSnapshot) || data != nil {
		t.Fatal(len(data), e)
	}
}

type deadline25 struct{ context.Context }

func (d deadline25) Err() error {
	if d.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
func TestDay25UnavailableAndCancellation(t *testing.T) {
	open := gate25(t, context.Background(), false)
	s, e := open.PauseSnapshot()
	zeroSnapshot25(t, s, e, runstate.ErrNoPause)
	for _, outcome := range []runstate.DecisionOutcome{runstate.DecisionApprove, runstate.DecisionReject} {
		g := gate25(t, context.Background(), true)
		decide25(t, g, outcome)
		if _, e := g.ResolveDecision("r25", " pause-1 "); e != nil {
			t.Fatal(e)
		}
		s, e := g.PauseSnapshot()
		zeroSnapshot25(t, s, e, runstate.ErrNoPause)
	}
	for _, deadline := range []bool{false, true} {
		for _, paused := range []bool{false, true} {
			t.Run(fmt.Sprintf("deadline=%v/paused=%v", deadline, paused), func(t *testing.T) {
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				var ctx context.Context = base
				want := error(context.Canceled)
				if deadline {
					ctx = deadline25{base}
					want = context.DeadlineExceeded
				}
				g := gate25(t, ctx, paused)
				before := g.Snapshot()
				if paused {
					decide25(t, g, runstate.DecisionApprove)
				}
				cancel()
				s, e := g.PauseSnapshot()
				zeroSnapshot25(t, s, e, want)
				if !reflect.DeepEqual(before, g.Snapshot()) {
					t.Fatal("capture committed cancellation")
				}
				if _, ok := g.CurrentInterrupt(); ok != paused {
					t.Fatal("capture cleared pause")
				}
				if _, ok := g.CurrentDecision(); ok != paused {
					t.Fatal("capture cleared decision")
				}
				if s, e := g.Apply(runstate.Event{Kind: runstate.StartTools}); !errors.Is(e, want) || s.Phase != runstate.Cancelled {
					t.Fatal(s, e)
				}
			})
		}
	}
}
func TestDay25ConcurrentCapture(t *testing.T) {
	g := gate25(t, context.Background(), true)
	decide25(t, g, runstate.DecisionApprove)
	original := snapshot25(t, g)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := g.PauseSnapshot()
			if e != nil {
				t.Error(e)
				return
			}
			data, e := runstate.EncodePauseSnapshot(s)
			if e != nil {
				t.Error(e)
				return
			}
			copy, e := runstate.DecodePauseSnapshot(data)
			if e != nil || !reflect.DeepEqual(copy, original) {
				t.Error(copy, e)
				return
			}
			s.Request.Checkpoint.State.Pending[0].Arguments[0] = 'x'
			copy.Decision.Actor = "edit"
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(snapshot25(t, g), original) {
		t.Fatal("concurrent snapshots alias gate")
	}
}
func TestDay25CaptureVersusControl(t *testing.T) {
	for i := 0; i < 24; i++ {
		g := gate25(t, context.Background(), true)
		start := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			<-start
			_, e := g.Decide(runstate.ApprovalDecision{RunID: "r25", RequestID: " pause-1 ", Actor: "reviewer", Outcome: runstate.DecisionApprove, Reason: "reviewed"})
			done <- e
		}()
		close(start)
		s, e := g.PauseSnapshot()
		if e != nil {
			t.Fatal(e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if s.Decision != nil && (s.Decision.RequestID != s.Request.ID || s.Decision.RunID != s.Request.Checkpoint.State.RunID) {
			t.Fatal("torn decision", s)
		}
		// Capture vs consumption yields an older valid snapshot or ErrNoPause.
		start = make(chan struct{})
		go func() { <-start; _, e := g.ResolveDecision("r25", " pause-1 "); done <- e }()
		close(start)
		s, e = g.PauseSnapshot()
		if e != nil {
			zeroSnapshot25(t, s, e, runstate.ErrNoPause)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if g.Snapshot().Phase != runstate.ToolsRunning {
			t.Fatal("resolution failed")
		}
		if s.SchemaVersion != 0 {
			if s.Request.Checkpoint.State.Phase != runstate.ToolsPending {
				t.Fatal(s)
			}
			if _, e := runstate.EncodePauseSnapshot(s); e != nil {
				t.Fatal("old capture is still encodable, not fresh-state proof", e)
			}
		}
	}
}
