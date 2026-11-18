package runstate

import (
	"bytes"
	"context"
	"strings"
)

type RecoveryPosition string

const (
	RecoveryReady         RecoveryPosition = "recorded_ready"
	RecoveryReviewPending RecoveryPosition = "review_pending"
	RecoveryTerminal      RecoveryPosition = "terminal"
	RecoveryExhausted     RecoveryPosition = "budget_exhausted"
)

// RecoveryPlan is a read-only description of one loaded record, not permission
// to execute or a restored Run. Record retains the store's observed Version.
type RecoveryPlan struct {
	Position RecoveryPosition
	Record   StoredCheckpoint
}

// recoveryRecordValid performs limited consistency checks, not complete replay,
// provenance verification, schema migration or external-effect reconciliation.
func recoveryRecordValid(v StoredCheckpoint, runID string) bool {
	cp := v.Checkpoint
	s := cp.State
	if v.Version == 0 || s.RunID != runID || !validateCandidate(cp) || len(s.Messages) == 0 || s.Messages[0].Role != "user" {
		return false
	}
	last := s.Messages[len(s.Messages)-1]
	if s.Phase == Completed {
		return len(s.Pending) == 0 && last.Role == "assistant" && len(last.ToolCalls) == 0 && s.Result == last.Content
	}
	if s.Phase == Ready {
		return len(s.Pending) == 0 && len(last.ToolCalls) == 0
	}
	if s.Phase == ToolsPending && len(s.Pending) == 0 {
		return false
	}
	if len(s.Pending) > 0 {
		if last.Role != "assistant" || len(last.ToolCalls) != len(s.Pending) {
			return false
		}
		ids := map[string]bool{}
		for i, c := range s.Pending {
			h := last.ToolCalls[i]
			if c.ID == "" || strings.TrimSpace(c.Name) == "" || ids[c.ID] || c.ID != h.ID || c.Name != h.Name || !bytes.Equal(c.Arguments, h.Arguments) {
				return false
			}
			ids[c.ID] = true
		}
	}
	return true
}

// PlanRecovery loads exactly once; never Save, Apply, Generate or Invoke.
// Pre-cancelled contexts cause no Load. Cancellation observed after Load wins over
// any returned value/error. Later cancellation can still follow a successful read.
// The injected store must support the caller's concurrency and owned return data.
func PlanRecovery(ctx context.Context, store CheckpointStore, runID string) (RecoveryPlan, error) {
	if err := storeContext(ctx); err != nil {
		return RecoveryPlan{}, err
	}
	if store == nil || strings.TrimSpace(runID) == "" {
		return RecoveryPlan{}, ErrInvalidCheckpoint
	}
	v, err := store.Load(ctx, runID)
	if cancelErr := ctx.Err(); cancelErr != nil {
		return RecoveryPlan{}, cancelErr
	}
	if err != nil {
		return RecoveryPlan{}, err
	}
	if !recoveryRecordValid(v, runID) {
		return RecoveryPlan{}, ErrInvalidCheckpoint
	}
	position := RecoveryTerminal
	switch v.Checkpoint.State.Phase {
	case Ready:
		position = RecoveryReady
		if v.Checkpoint.State.Step >= v.Checkpoint.State.MaxSteps {
			position = RecoveryExhausted
		}
	case ToolsPending:
		position = RecoveryReviewPending
	}
	return RecoveryPlan{Position: position, Record: cloneStored(v)}, nil
}
