package runstate

import "errors"

type CheckpointBoundary string

const (
	CheckpointReady       CheckpointBoundary = "ready"
	CheckpointBeforeTools CheckpointBoundary = "before_tools"
	CheckpointTerminal    CheckpointBoundary = "terminal"
)

var ErrCheckpointBoundary = errors.New("runstate: checkpoint requires a stable boundary")

// Checkpoint is an in-memory candidate, not a storage acknowledgement or resume token.
// State uses the existing message/call contract. Boundary classifies State.Phase.
type Checkpoint struct {
	Boundary CheckpointBoundary
	State    State
}

// Clone copies nested mutable state. It does not validate or authorize a checkpoint.
func (c Checkpoint) Clone() Checkpoint {
	c.State = cloneState(c.State)
	return c
}

// Checkpoint samples boundary and state under the same lock.
// It does not observe context cancellation, advance state, pause the driver,
// perform I/O, or promise that an external tool has not already taken effect.
func (r *Run) Checkpoint() (Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var boundary CheckpointBoundary
	switch r.state.Phase {
	case Ready:
		boundary = CheckpointReady
	case ToolsPending:
		boundary = CheckpointBeforeTools
	case Completed, Failed, Cancelled:
		boundary = CheckpointTerminal
	default:
		return Checkpoint{}, ErrCheckpointBoundary
	}
	return Checkpoint{Boundary: boundary, State: cloneState(r.state)}, nil
}
