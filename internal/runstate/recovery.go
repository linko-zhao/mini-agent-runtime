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

type RecoveryPlan struct {
	Position RecoveryPosition
	Record   StoredCheckpoint
}

func validateRecoveryRecord(v StoredCheckpoint, runID string) bool {
	cp := v.Checkpoint
	s := cp.State
	if v.Version <= 0 || s.RunID != runID || !validateCheckpoint(cp) || len(s.Messages) == 0 || s.Messages[0].Role != "user" {
		return false
	}
	last := s.Messages[len(s.Messages)-1]
	if s.Phase == Ready {
		return len(s.Pending) == 0 && len(last.ToolCalls) == 0
	}
	if s.Phase == Completed {
		return len(s.Pending) == 0 && last.Role == "assistant" && len(last.ToolCalls) == 0 && last.Content == s.Result
	}

	if s.Phase == ToolsPending && len(s.Pending) == 0 {
		return false
	}
	if len(s.Pending) > 0 {
		if last.Role != "assistant" || len(last.ToolCalls) != len(s.Pending) {
			return false
		}
		ids := make(map[string]bool)
		for i, p := range s.Pending {
			t := last.ToolCalls[i]
			if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" || ids[p.ID] || p.ID != t.ID || p.Name != t.Name || !bytes.Equal(p.Arguments, t.Arguments) {
				return false
			}
			ids[p.ID] = true
		}
	}
	return true
}

func PlanRecovery(ctx context.Context, store CheckpointStore, runID string) (RecoveryPlan, error) {
	if err := storeContext(ctx); err != nil {
		return RecoveryPlan{}, err
	}
	if store == nil || strings.TrimSpace(runID) == "" {
		return RecoveryPlan{}, ErrInvalidCheckpoint
	}
	sc, err := store.Load(ctx, runID)
	if ctx.Err() != nil {
		return RecoveryPlan{}, ctx.Err()
	}
	if err != nil {
		return RecoveryPlan{}, err
	}
	if !validateRecoveryRecord(sc, runID) {
		return RecoveryPlan{}, ErrInvalidCheckpoint
	}
	p := RecoveryTerminal
	switch sc.Checkpoint.Boundary {
	case CheckpointReady:
		p = RecoveryReady
		if sc.Checkpoint.State.Step == sc.Checkpoint.State.MaxSteps {
			p = RecoveryExhausted
		}
	case CheckpointBeforeTools:
		p = RecoveryReviewPending
	}
	return RecoveryPlan{Position: p, Record: cloneStore(sc)}, nil
}
