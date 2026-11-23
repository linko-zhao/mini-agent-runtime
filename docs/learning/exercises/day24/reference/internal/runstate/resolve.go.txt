package runstate

import (
	"errors"
	"strings"
)

var ErrDecisionRequired = errors.New("runstate: a recorded decision is required")

// ResolveDecision consumes the held decision and commits one transition.
// Approve starts ToolsRunning; reject commits Failed using the recorded reason.
// No external I/O runs here. The host may execute tools only after nil error AND
// ToolsRunning from this call, never by inspecting a retry's returned State alone.
func (g *PauseGate) ResolveDecision(runID, requestID string) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Preserve Run.Apply's cancellation observation and terminal-first behavior.
	if g.run.Context().Err() != nil {
		state, err := g.run.Apply(Event{Kind: Cancel})
		if terminal(state.Phase) {
			g.request, g.decision = nil, nil
		}
		return state, err
	}
	current := g.run.Snapshot()
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(requestID) == "" {
		return current, ErrInvalidDecision
	}
	if g.request == nil {
		return current, ErrNoPause
	}
	if runID != g.request.Checkpoint.State.RunID || requestID != g.request.ID {
		return current, ErrDecisionMismatch
	}
	if g.decision == nil {
		return current, ErrDecisionRequired
	}
	event := Event{Kind: StartTools}
	if g.decision.Outcome == DecisionReject {
		event = Event{Kind: Abort, Reason: g.decision.Reason}
	}
	state, err := g.run.Apply(event)
	if err == nil || terminal(state.Phase) {
		g.request, g.decision = nil, nil
	}
	return state, err
}
