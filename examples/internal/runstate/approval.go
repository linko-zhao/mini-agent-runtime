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

// Decide records the first valid decision for this gate's held pause.
// It does not unpause, execute, terminate or write a checkpoint.
func (g *PauseGate) Decide(d ApprovalDecision) (ApprovalDecision, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.run.Context().Err(); err != nil {
		return ApprovalDecision{}, err
	}
	if strings.TrimSpace(d.RunID) == "" || strings.TrimSpace(d.RequestID) == "" ||
		strings.TrimSpace(d.Actor) == "" || strings.TrimSpace(d.Reason) == "" ||
		(d.Outcome != DecisionApprove && d.Outcome != DecisionReject) {
		return ApprovalDecision{}, ErrInvalidDecision
	}
	if g.request == nil {
		return ApprovalDecision{}, ErrNoPause
	}
	if d.RunID != g.request.Checkpoint.State.RunID || d.RequestID != g.request.ID {
		return ApprovalDecision{}, ErrDecisionMismatch
	}
	if g.decision != nil {
		return ApprovalDecision{}, ErrDecisionRecorded
	}
	if err := g.run.Context().Err(); err != nil {
		return ApprovalDecision{}, err
	}
	g.decision = &d
	return d, nil
}

// CurrentDecision reads held metadata without observing cancellation.
// A terminal Apply clears it together with the held pause request.
func (g *PauseGate) CurrentDecision() (ApprovalDecision, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.decision == nil {
		return ApprovalDecision{}, false
	}
	return *g.decision, true
}
