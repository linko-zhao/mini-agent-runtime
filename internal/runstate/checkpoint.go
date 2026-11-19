package runstate

import "errors"

type CheckpointBoundary string

const (
	CheckpointReady       CheckpointBoundary = "ready"
	CheckpointBeforeTools CheckpointBoundary = "before_tools"
	CheckpointTerminal    CheckpointBoundary = "terminal"
)

var ErrCheckpointBoundary = errors.New("runstate: checkpoint requires a stable boundary")

type Checkpoint struct {
	Boundary CheckpointBoundary
	State    State
}

func (c Checkpoint) Clone() Checkpoint {
	return Checkpoint{Boundary: c.Boundary, State: cloneState(c.State)}
}
func (r *Run) Checkpoint() (Checkpoint, error) {
	c := Checkpoint{State: r.Snapshot()}
	if terminal(c.State.Phase) {
		return Checkpoint{Boundary: CheckpointTerminal, State: r.Snapshot()}, nil
	}
	switch c.State.Phase {
	case Ready:
		c.Boundary = CheckpointReady
	case ToolsPending:
		c.Boundary = CheckpointBeforeTools
	default:
		return Checkpoint{}, ErrCheckpointBoundary
	}
	return c, nil
}
