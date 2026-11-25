package runstate

import (
	"errors"
	"strings"
	"sync"
)

var (
	ErrInvalidPause  = errors.New("runstate: invalid pause request")
	ErrPauseBoundary = errors.New("runstate: pause requires tools_pending")
	ErrPaused        = errors.New("runstate: gate is paused")
)

type PauseRequest struct {
	ID         string
	Reason     string
	Checkpoint Checkpoint
}
type PauseGate struct {
	mu       sync.Mutex
	run      *Run
	request  *PauseRequest
	desicion *ApprovalDecision
}

func NewPauseGate(run *Run) (*PauseGate, error) {
	if run == nil {
		return nil, ErrInvalidPause
	}
	return &PauseGate{run: run}, nil
}
func (g *PauseGate) Snapshot() State {
	return g.run.Snapshot()
}
func (g *PauseGate) CurrentInterrupt() (PauseRequest, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.request == nil {
		return PauseRequest{}, false
	}
	return PauseRequest{ID: g.request.ID, Reason: g.request.Reason, Checkpoint: g.request.Checkpoint.Clone()}, true
}
func (g *PauseGate) Pause(id, reason string) (PauseRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.run.ctx.Err() != nil {
		return PauseRequest{}, g.run.ctx.Err()
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(reason) == "" {
		return PauseRequest{}, ErrInvalidPause
	}
	if g.request != nil {
		return PauseRequest{}, ErrPaused
	}
	cp, err := g.run.Checkpoint()
	if err != nil || cp.State.Phase != ToolsPending {
		return PauseRequest{}, ErrPauseBoundary
	}
	if g.run.ctx.Err() != nil {
		return PauseRequest{}, g.run.ctx.Err()
	}
	r := PauseRequest{ID: id, Reason: reason, Checkpoint: cp.Clone()}
	g.request = &r
	out := r
	out.Checkpoint = out.Checkpoint.Clone()
	return out, nil
}
func (g *PauseGate) Apply(e Event) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.request != nil && g.run.ctx.Err() == nil {
		return g.run.Snapshot(), ErrPaused
	}
	s, err := g.run.Apply(e)
	if terminal(s.Phase) {
		g.request = nil
		g.desicion = nil
	}
	return s, err
}
