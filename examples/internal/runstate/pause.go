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

// PauseRequest is an owned in-memory interruption snapshot, not a saved receipt,
// an approval, a resume token or a declaration about external side effects.
type PauseRequest struct {
	ID         string
	Reason     string
	Checkpoint Checkpoint
}

// PauseGate coordinates transitions only through this gate. The host must
// transfer exclusive write ownership of the Run to ONE gate and never bypass it.
// Zero value is not usable; do not copy after use.
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

func (g *PauseGate) Snapshot() State { return g.run.Snapshot() }

func (g *PauseGate) CurrentInterrupt() (PauseRequest, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.request == nil {
		return PauseRequest{}, false
	}
	out := *g.request
	out.Checkpoint = out.Checkpoint.Clone()
	return out, true
}

// Pause captures only ToolsPending, before StartTools through this gate.
// It samples the original Run context and performs no I/O or state transition.
func (g *PauseGate) Pause(id, reason string) (PauseRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.run.Context().Err(); err != nil {
		return PauseRequest{}, err
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
	if err := g.run.Context().Err(); err != nil {
		return PauseRequest{}, err
	}
	request := PauseRequest{ID: id, Reason: reason, Checkpoint: cp}
	g.request = &request
	out := request
	out.Checkpoint = out.Checkpoint.Clone()
	return out, nil
}

// Apply preserves Run.Apply semantics while open. During a live pause every
// event is rejected with a current owned State and ErrPaused. Observed context
// cancellation is allowed through to Run.Apply; terminal state clears the pause.
// The host may perform I/O only AFTER a successful transition; no I/O runs here.
func (g *PauseGate) Apply(e Event) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.request != nil && g.run.Context().Err() == nil {
		return g.run.Snapshot(), ErrPaused
	}
	state, err := g.run.Apply(e)
	if terminal(state.Phase) {
		g.request = nil
	}
	return state, err
}
