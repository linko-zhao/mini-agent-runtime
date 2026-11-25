package runstate

import (
	"errors"
	"strings"
)

type DecisionOutcome string

const (
	DecisionApprove DecisionOutcome = "approve"
	DecisionReject  DecisionOutcome = "reject"
)

var (
	ErrInvalidDecision  = errors.New("runstate: invalid approval decision")
	ErrNoPause          = errors.New("runstate: no held pause request")
	ErrDecisionMismatch = errors.New("runstate: decision does not match held request")
	ErrDecisionRecorded = errors.New("runstate: decision already recorded")
)

// ApprovalDecision is host-provided metadata, not proof of identity, authority,
// persistence or permission to execute. All fields are immutable value strings.
type ApprovalDecision struct {
	RunID     string
	RequestID string
	Actor     string
	Outcome   DecisionOutcome
	Reason    string
}

// Decide records a decision without executing or releasing the pause.
func (g *PauseGate) Decide(d ApprovalDecision) (ApprovalDecision, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.run.ctx.Err() != nil {
		return ApprovalDecision{}, g.run.ctx.Err()
	}
	if strings.TrimSpace(d.RunID) == "" || strings.TrimSpace(d.RequestID) == "" || strings.TrimSpace(d.Actor) == "" || strings.TrimSpace(d.Reason) == "" || (d.Outcome != DecisionApprove && d.Outcome != DecisionReject) {
		return ApprovalDecision{}, ErrInvalidDecision
	}
	if g.request == nil {
		return ApprovalDecision{}, ErrNoPause
	}
	if d.RunID != g.run.state.RunID || d.RequestID != g.request.ID {
		return  ApprovalDecision{}, ErrDecisionMismatch
	}
	if g.desicion != nil {
		return ApprovalDecision{}, ErrDecisionRecorded
	}
	if g.run.ctx.Err() != nil {
		return ApprovalDecision{}, g.run.ctx.Err()
	}
	g.desicion = &d
	return d, nil
}

// CurrentDecision returns a value copy of held decision metadata.
func (g *PauseGate) CurrentDecision() (ApprovalDecision, bool) {
	if g.desicion == nil {
		return ApprovalDecision{}, false
	}
	return ApprovalDecision(*g.desicion), true
}
