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
	panic("unreach")
}
