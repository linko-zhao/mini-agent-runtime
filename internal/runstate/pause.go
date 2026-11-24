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
	mu      sync.Mutex
	run     *Run
	request *PauseRequest
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
	if g.request == nil {
		return PauseRequest{}, false
	}
	return PauseRequest{ID: g.request.ID, Reason: g.request.Reason, Checkpoint: g.request.Checkpoint.Clone()}, true
}
func (g *PauseGate) Pause(id, reason string) (PauseRequest, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(reason) == "" {
		return PauseRequest{}, ErrInvalidPause
	}
	if g.request != nil {
		return PauseRequest{}, ErrPaused
	}
	return PauseRequest{ID: id, Reason: reason, Checkpoint: g.request.Checkpoint.Clone()}, nil
}
func (g *PauseGate) Apply(e Event) (State, error) {
	if g.request == nil {
		return g.run.Apply(e)
	}
	if g.run.ctx.Err() == nil {
		return g.request.Checkpoint.State, ErrPaused
	}
	return State{}, nil
}
