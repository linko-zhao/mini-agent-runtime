package runstate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

func recovery20(t *testing.T, n int) (*runstate.Run, *runstate.MemoryCheckpointStore, runstate.RecoveryPlan) {
	t.Helper()
	ctx := context.Background()
	r, e := runstate.NewRun(ctx, "r20", "读取文档", 3)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Apply(runstate.Event{Kind: runstate.StartModel}); e != nil {
		t.Fatal(e)
	}
	calls := make([]model.ToolCall, n)
	for i := range calls {
		calls[i] = model.ToolCall{ID: fmt.Sprintf("call-%d", i+1), Name: "get_document", Arguments: []byte(fmt.Sprintf(`{"id":"doc-%d"}`, i+1))}
	}
	if _, e = r.Apply(runstate.Event{Kind: runstate.ModelReturned, Response: &model.Response{Message: model.Message{Role: "assistant", ToolCalls: calls}}}); e != nil {
		t.Fatal(e)
	}
	cp, e := r.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	s := runstate.NewMemoryCheckpointStore()
	if _, e = s.Save(ctx, cp, 0); e != nil {
		t.Fatal(e)
	}
	p, e := runstate.PlanRecovery(ctx, s, "r20")
	if e != nil {
		t.Fatal(e)
	}
	return r, s, p
}
func evidence20(p runstate.RecoveryPlan, bases ...runstate.ReplayBasis) runstate.ReplayEvidenceSet {
	e := runstate.ReplayEvidenceSet{RunID: "r20", Version: p.Record.Version, Calls: make([]runstate.ReplayEvidence, len(bases))}
	for i, b := range bases {
		call := p.Record.Checkpoint.State.Pending[i]
		e.Calls[i] = runstate.ReplayEvidence{Call: call, Basis: b}
		if b == runstate.BasisResultKnown {
			e.Calls[i].Result = &model.Message{Role: "tool", ToolCallID: call.ID, Content: "已确认正文"}
		}
		if b == runstate.BasisBackendDedup {
			e.Calls[i].IdempotencyKey = fmt.Sprintf("host-key-%d", i+1)
		}
	}
	return e
}
func replayZero20(t *testing.T, p runstate.ReplayPlan, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || !reflect.DeepEqual(p, runstate.ReplayPlan{}) {
		t.Fatal(p, e, want)
	}
}
func TestDay20DecisionsAndPurity(t *testing.T) {
	r, s, source := recovery20(t, 4)
	before := r.Snapshot()
	evidence := evidence20(source, runstate.BasisUnknown, runstate.BasisNotStarted, runstate.BasisResultKnown, runstate.BasisBackendDedup)
	p, e := runstate.PlanReplay(context.Background(), source, evidence)
	if e != nil || !p.Blocked || len(p.Steps) != 4 || !reflect.DeepEqual(p.Record, source.Record) {
		t.Fatal(p, e)
	}
	wants := []runstate.ReplayAction{runstate.ActionHold, runstate.ActionInvoke, runstate.ActionReuse, runstate.ActionInvoke}
	for i, want := range wants {
		if p.Steps[i].Action != want || p.Steps[i].Call.ID != fmt.Sprintf("call-%d", i+1) {
			t.Fatal(p)
		}
	}
	if p.Steps[0].Result != nil || p.Steps[1].IdempotencyKey != "" || p.Steps[2].Result.Content != "已确认正文" || p.Steps[3].IdempotencyKey != "host-key-4" {
		t.Fatal(p)
	}
	evidence.Calls[0].Basis = runstate.BasisNotStarted
	p, e = runstate.PlanReplay(context.Background(), source, evidence)
	if e != nil || p.Blocked {
		t.Fatal(p, e)
	}
	after, e := s.Load(context.Background(), "r20")
	if e != nil || after.Version != 1 || !reflect.DeepEqual(after, source.Record) || !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal(after, e)
	}
	// A controlled tool-error message is also an accepted confirmed result.
	evidence.Calls[2].Result.Content = "execution_failed: controlled failure"
	p, e = runstate.PlanReplay(context.Background(), source, evidence)
	if e != nil || p.Steps[2].Action != runstate.ActionReuse || p.Steps[2].Result.Content != evidence.Calls[2].Result.Content {
		t.Fatal(p, e)
	}
}
func TestDay20InvalidRecovery(t *testing.T) {
	_, _, source := recovery20(t, 1)
	evidence := evidence20(source, runstate.BasisUnknown)
	for _, pos := range []runstate.RecoveryPosition{runstate.RecoveryReady, runstate.RecoveryTerminal, runstate.RecoveryExhausted, ""} {
		bad := source
		bad.Position = pos
		p, e := runstate.PlanReplay(context.Background(), bad, evidence)
		replayZero20(t, p, e, runstate.ErrInvalidCheckpoint)
	}
	for _, mutate := range []func(*runstate.RecoveryPlan){
		func(p *runstate.RecoveryPlan) { p.Record.Version = 0 },
		func(p *runstate.RecoveryPlan) {
			p.Record.Checkpoint.State.Phase = runstate.Cancelled
			p.Record.Checkpoint.Boundary = runstate.CheckpointTerminal
		},
		func(p *runstate.RecoveryPlan) { p.Record.Checkpoint.State.Messages = nil },
		func(p *runstate.RecoveryPlan) { p.Record.Checkpoint.State.Pending[0].ID = "wrong" },
	} {
		bad := source
		bad.Record.Checkpoint = source.Record.Checkpoint.Clone()
		mutate(&bad)
		p, e := runstate.PlanReplay(context.Background(), bad, evidence)
		replayZero20(t, p, e, runstate.ErrInvalidCheckpoint)
	}
}
func TestDay20EvidenceBinding(t *testing.T) {
	_, _, source := recovery20(t, 2)
	mutations := []func(*runstate.ReplayEvidenceSet){
		func(e *runstate.ReplayEvidenceSet) { e.RunID = "other" },
		func(e *runstate.ReplayEvidenceSet) { e.Version++ },
		func(e *runstate.ReplayEvidenceSet) { e.Calls = e.Calls[:1] },
		func(e *runstate.ReplayEvidenceSet) { e.Calls = nil },
		func(e *runstate.ReplayEvidenceSet) { e.Calls[0], e.Calls[1] = e.Calls[1], e.Calls[0] },
		func(e *runstate.ReplayEvidenceSet) { e.Calls[0].Call.ID = "other" },
		func(e *runstate.ReplayEvidenceSet) { e.Calls[0].Call.Name = "other" },
		func(e *runstate.ReplayEvidenceSet) { e.Calls[0].Call.Arguments = []byte("changed") },
	}
	for i, mutate := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			evidence := evidence20(source, runstate.BasisUnknown, runstate.BasisUnknown)
			mutate(&evidence)
			p, e := runstate.PlanReplay(context.Background(), source, evidence)
			replayZero20(t, p, e, runstate.ErrReplayEvidence)
		})
	}
}
func TestDay20EvidenceShapes(t *testing.T) {
	_, _, source := recovery20(t, 2)
	callID := source.Record.Checkpoint.State.Pending[0].ID
	rows := []struct {
		basis  runstate.ReplayBasis
		result *model.Message
		key    string
	}{
		{"", nil, ""}, {"invalid", nil, ""},
		{runstate.BasisUnknown, nil, "a-key-is-not-proof"},
		{runstate.BasisUnknown, &model.Message{}, ""},
		{runstate.BasisNotStarted, nil, "unexpected-key"},
		{runstate.BasisNotStarted, &model.Message{}, ""},
		{runstate.BasisResultKnown, nil, ""},
		{runstate.BasisResultKnown, &model.Message{Role: "assistant", ToolCallID: callID}, ""},
		{runstate.BasisResultKnown, &model.Message{Role: "tool", ToolCallID: "other"}, ""},
		{runstate.BasisResultKnown, &model.Message{Role: "tool", ToolCallID: callID, ToolCalls: []model.ToolCall{{ID: "nested"}}}, ""},
		{runstate.BasisResultKnown, &model.Message{Role: "tool", ToolCallID: callID}, "unexpected-key"},
		{runstate.BasisBackendDedup, nil, ""},
		{runstate.BasisBackendDedup, nil, " "},
		{runstate.BasisBackendDedup, &model.Message{}, "key"},
	}
	for i, row := range rows {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			evidence := evidence20(source, row.basis, runstate.BasisUnknown)
			evidence.Calls[0].Result = row.result
			evidence.Calls[0].IdempotencyKey = row.key
			p, e := runstate.PlanReplay(context.Background(), source, evidence)
			replayZero20(t, p, e, runstate.ErrReplayEvidence)
		})
	}
	evidence := evidence20(source, runstate.BasisBackendDedup, runstate.BasisBackendDedup)
	evidence.Calls[1].IdempotencyKey = evidence.Calls[0].IdempotencyKey
	p, e := runstate.PlanReplay(context.Background(), source, evidence)
	replayZero20(t, p, e, runstate.ErrReplayEvidence)
	// Opaque keys are preserved verbatim after rejecting all-whitespace keys.
	evidence = evidence20(source, runstate.BasisBackendDedup, runstate.BasisNotStarted)
	evidence.Calls[0].IdempotencyKey = " scoped-key "
	p, e = runstate.PlanReplay(context.Background(), source, evidence)
	if e != nil || p.Blocked || p.Steps[0].IdempotencyKey != " scoped-key " {
		t.Fatal(p, e)
	}
}
func TestDay20Cancellation(t *testing.T) {
	_, _, source := recovery20(t, 1)
	evidence := evidence20(source, runstate.BasisUnknown)
	p, e := runstate.PlanReplay(nil, source, evidence)
	replayZero20(t, p, e, runstate.ErrInvalidCheckpoint)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, finish := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer finish()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{canceled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		p, e = runstate.PlanReplay(tc.ctx, source, evidence)
		replayZero20(t, p, e, tc.want)
	}
}
func TestDay20OwnershipAndConcurrency(t *testing.T) {
	_, _, source := recovery20(t, 2)
	original := source.Record.Checkpoint.Clone()
	evidence := evidence20(source, runstate.BasisResultKnown, runstate.BasisBackendDedup)
	p, e := runstate.PlanReplay(context.Background(), source, evidence)
	if e != nil {
		t.Fatal(e)
	}
	p.Steps[0].Call.Arguments[0] = 'x'
	if p.Record.Checkpoint.State.Pending[0].Arguments[0] != '{' {
		t.Fatal("step aliases Record")
	}
	p.Steps[0].Result.Content = "changed"
	p.Record.Checkpoint.State.Messages[1].ToolCalls[0].Arguments[0] = 'y'
	if evidence.Calls[0].Result.Content != "已确认正文" || !reflect.DeepEqual(original, source.Record.Checkpoint) {
		t.Fatal("input modified")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := runstate.PlanReplay(context.Background(), source, evidence)
			if e != nil {
				t.Error(e)
				return
			}
			if p.Blocked || p.Record.Version != 1 || p.Steps[1].IdempotencyKey != "host-key-2" {
				t.Error(p)
			}
			p.Steps[0].Call.Arguments[0] = 'z'
			p.Steps[0].Result.Content = "owned"
		}()
	}
	wg.Wait()
	if evidence.Calls[0].Result.Content != "已确认正文" || !reflect.DeepEqual(original, source.Record.Checkpoint) {
		t.Fatal("shared output")
	}
}
