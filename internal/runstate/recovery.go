package runstate

import (
	"context"
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

func recoveryRecordValid(v StoredCheckpoint, runID string) bool {
	cp := v.Checkpoint
	s := cp.State
	if v.Version <= 0 || s.RunID != runID || validateCheckpoint(cp) || len(s.Messages) == 0 || s.Messages[0].Role != "user" {
		return false
	}
	last := s.Messages[len(s.Messages) - 1]
	switch s.Phase {
	case Ready:
		return len(s.Pending) == 0 && len(last.ToolCalls) == 0
	case ToolsPending:
		return len(s.Pending) != 0 && last.Role == "assistant" && len(last.ToolCalls) == len(s.Pending)
	case Completed:
		return len(s.Pending) == 0 && last.Role == "assistant" && len(last.ToolCalls) == 0 && last.Content == s.Result
	}
	if len(s.Pending) != 0 {
		if last.Role != "assistant" && len(last.ToolCalls) != len(s.Pending) {
			return false
		}
	}
}

func PlanRecovery(ctx context.Context, store CheckpointStore, runID string) (RecoveryPlan, error) {
	sc, err := store.Load(ctx, runID)
	if err != nil {
		return RecoveryPlan{}, err
	}
	if ctx.Err() != nil {
		return RecoveryPlan{}, ctx.Err()
	}
	switch sc.Checkpoint.Boundary {
	case CheckpointReady:
		if sc.Checkpoint.State.Step < sc.Checkpoint.State.MaxSteps {
			return RecoveryPlan{Position: RecoveryReady, Record: sc}, nil
		} else {
			return RecoveryPlan{Position: RecoveryExhausted, Record: sc}, nil
		}
	case CheckpointBeforeTools:
		return RecoveryPlan{Position: RecoveryReviewPending, Record: sc}, nil
	case CheckpointTerminal:
		return RecoveryPlan{Position: RecoveryTerminal, Record: sc}, nil
	}
	panic("unreachable")
}
