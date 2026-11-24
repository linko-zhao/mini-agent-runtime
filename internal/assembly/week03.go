package assembly

import (
	"context"
	"errors"

	"github.com/linko-zhao/mini-agent-runtime/internal/runstate"
)

type ReplayEvidenceSource interface {
	Evidence(context.Context, runstate.StoredCheckpoint) (runstate.ReplayEvidenceSet, error)
}

var ErrEvidenceSourceRequired = errors.New("assembly: pending review requires an evidence source")

type Week03Report struct {
	Recovery runstate.RecoveryPlan
	Replay   *runstate.ReplayPlan
}

func ReviewWeek03(ctx context.Context, store runstate.CheckpointStore, source ReplayEvidenceSource, runID string) (Week03Report, error) {
	rp, err := runstate.PlanRecovery(ctx, store, runID)
	if err != nil {
		return Week03Report{}, err
	}
	if ctx.Err() != nil {
		return Week03Report{}, ctx.Err()
	}
	if rp.Position != runstate.RecoveryReviewPending {
		return Week03Report{Recovery: rp}, nil
	}
	if source == nil {
		return Week03Report{}, ErrEvidenceSourceRequired
	}
	check := rp.Record.Checkpoint.Clone()
	s, err := source.Evidence(ctx, runstate.StoredCheckpoint{Version: rp.Record.Version, Checkpoint: check})
	if ctx.Err() != nil {
		return Week03Report{}, ctx.Err()
	}
	if err != nil {
		return Week03Report{}, err
	}
	replay, err := runstate.PlanReplay(ctx, rp, s)
	if err != nil {
		return Week03Report{}, err
	}
	return Week03Report{Recovery: rp, Replay: &replay}, nil
}
