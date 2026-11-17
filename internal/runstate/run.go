package runstate

import (
	"context"
	"sync"
)

// Run owns one in-memory State. Do not copy after use; use NewRun.
type Run struct {
	ctx   context.Context
	mu    sync.Mutex
	state State
}

func NewRun(ctx context.Context, runID, input string, maxSteps int) (*Run, error) {
	if ctx == nil {
		return nil, ErrTransition
	}
	if err := ctx.Err(); err != nil {
		return nil, ctx.Err()
	}
	s, err := New(runID, input, maxSteps)
	if err != nil {
		return nil, err
	}
	return &Run{
		ctx:   ctx,
		state: s,
	}, nil
}
func (r *Run) Context() context.Context {
	return r.ctx
}
func (r *Run) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneState(r.state)
}
func (r *Run) Apply(e Event) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, err := Advance(r.state, e)
	if err != nil {
		return cloneState(r.state), err
	}
	return s, nil
}
