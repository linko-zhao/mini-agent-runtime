package runstate

import (
	"context"
	"errors"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type ReplayBasis string

const (
	BasisUnknown      ReplayBasis = "unknown"
	BasisNotStarted   ReplayBasis = "not_started"
	BasisResultKnown  ReplayBasis = "result_known"
	BasisBackendDedup ReplayBasis = "backend_dedup"
)

type ReplayAction string

const (
	ActionHold   ReplayAction = "hold"
	ActionInvoke ReplayAction = "invoke"
	ActionReuse  ReplayAction = "reuse_result"
)

var ErrReplayEvidence = errors.New("runstate: invalid replay evidence")

type ReplayEvidence struct {
	Call           model.ToolCall
	Basis          ReplayBasis
	Result         *model.Message
	IdempotencyKey string
}
type ReplayEvidenceSet struct {
	RunID   string
	Version uint64
	Calls   []ReplayEvidence
}
type ReplayStep struct {
	Call           model.ToolCall
	Action         ReplayAction
	Result         *model.Message
	IdempotencyKey string
}
type ReplayPlan struct {
	Record  StoredCheckpoint
	Steps   []ReplayStep
	Blocked bool // any Hold blocks the entire batch before effects/history updates
}

func PlanReplay(ctx context.Context, recovery RecoveryPlan, evidence ReplayEvidenceSet) (ReplayPlan, error) {
	if err := storeContext(ctx); err != nil {
		return ReplayPlan{}, err
	}
	if recovery.Position != RecoveryReviewPending || recovery.Record.Checkpoint.State.Phase != ToolsPending || !recoveryRecordValid(recovery.Record, evidence.RunID) {
		return ReplayPlan{}, ErrInvalidCheckpoint
	}
	if recovery.Record.Version != evidence.Version || recovery.Record.Checkpoint.State.RunID != evidence.RunID {
		return ReplayPlan{}, ErrReplayEvidence
	}
	rp := ReplayPlan{
		Record: cloneStore(recovery.Record),
		Steps: make([]ReplayStep, 0),
	}
	for _, e := range evidence.Calls {
		step := ReplayStep{}
		step.Call = e.Call
		switch e.Basis {
		case BasisUnknown:
			step.Action = ActionHold
			rp.Blocked = true
		case BasisNotStarted:
			step.Action = ActionInvoke
		case BasisResultKnown:
			step.Action = ActionReuse
			step.Result = &model.Message{Role: "tool", ToolCallID: e.Call.ID, Content: e.Result.Content}
		case BasisBackendDedup:
			step.Action = ActionInvoke
			step.IdempotencyKey = e.IdempotencyKey
		default:
			return ReplayPlan{}, ErrReplayEvidence
		}
		rp.Steps = append(rp.Steps, step)
	}
	return rp, nil
}
