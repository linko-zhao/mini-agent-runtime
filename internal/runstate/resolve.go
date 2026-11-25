package runstate

import (
	"errors"
	"strings"
)

var ErrDecisionRequired = errors.New("runstate: a recorded decision is required")

// ResolveDecision consumes the held decision without executing external I/O.
func (g *PauseGate) ResolveDecision(runID, requestID string) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.run.ctx.Err() != nil {
		s, err := g.run.Apply(Event{Kind: Cancel})
		g.request = nil
		g.desicion = nil
		if !terminal(s.Phase) {
			return s, g.run.ctx.Err()
		}
		return s, err
	}
	s := g.Snapshot()
	desicion := g.desicion
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(requestID) == "" {
		return s, ErrInvalidDecision
	}
	if g.request == nil || terminal(s.Phase) {
		return s, ErrNoPause
	}
	if runID != g.run.state.RunID || requestID != g.request.ID {
		return s, ErrDecisionMismatch
	}
	if desicion == nil {
		return s, ErrDecisionRequired
	}
	outcome := desicion.Outcome
	e := Event{Kind: StartTools}
	if outcome != DecisionApprove {
		e.Kind = Abort
		e.Reason = g.desicion.Reason
	}
	g.request = nil
	g.desicion = nil
	next, err := g.run.Apply(e)
	if g.run.ctx.Err() != nil {
		return s, g.run.ctx.Err()
	}
	return next, err
}
