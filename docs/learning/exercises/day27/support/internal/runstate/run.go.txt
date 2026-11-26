package runstate

import (
	"context"
	"sync"
)

// Run owns one in-memory State. Use its pointer; do not copy it after use.
// The zero value is not usable; construct with NewRun.
type Run struct {
	ctx   context.Context
	mu    sync.Mutex
	state State
}

// NewRun retains ctx for cancellation/deadlines, not for storing State.
// A cancelled context rejects construction; invalid identity/budget uses ErrTransition.
func NewRun(ctx context.Context, runID, input string, maxSteps int) (*Run, error) {
	if ctx == nil {
		return nil, ErrTransition
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := New(runID, input, maxSteps)
	if err != nil {
		return nil, err
	}
	return &Run{ctx: ctx, state: s}, nil
}

// Context returns the original context. The host owns any cancel function.
// Model/tool I/O must use this context outside Apply, never while holding its lock.
func (r *Run) Context() context.Context { return r.ctx }

// Snapshot returns an independent copy without triggering transitions.
func (r *Run) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneState(r.state)
}

// Apply commits exactly one valid event, or returns the unchanged current copy.
// For a nonterminal Run, observed context cancellation commits Cancel and rejects
// the event with the original context error. A terminal State is never overwritten.
// This serializes transitions, not a snapshot -> external I/O -> Apply transaction.
func (r *Run) Apply(e Event) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if terminal(r.state.Phase) {
		return cloneState(r.state), ErrTransition
	}
	if err := r.ctx.Err(); err != nil {
		next, transitionErr := Advance(r.state, Event{Kind: Cancel})
		if transitionErr != nil {
			return cloneState(r.state), transitionErr
		}
		r.state = next
		return cloneState(r.state), err
	}
	next, err := Advance(r.state, e)
	if err != nil {
		return cloneState(r.state), err
	}
	r.state = next
	return cloneState(r.state), nil
}
