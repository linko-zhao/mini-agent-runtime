package runstate

import (
	"bytes"
	"context"
	"errors"
	"strings"

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
	record := recovery.Record
	if recovery.Position != RecoveryReviewPending || record.Checkpoint.State.Phase != ToolsPending || !recoveryRecordValid(recovery.Record, record.Checkpoint.State.RunID) {
		return ReplayPlan{}, ErrInvalidCheckpoint
	}
	if record.Version != evidence.Version || record.Checkpoint.State.RunID != evidence.RunID || len(evidence.Calls) != len(record.Checkpoint.State.Pending) {
		return ReplayPlan{}, ErrReplayEvidence
	}
	rp := ReplayPlan{
		Record: cloneStore(record),
		Steps:  make([]ReplayStep, 0),
	}
	idKeys := map[string]bool{}
	for i, e := range evidence.Calls {
		call := e.Call
		p := record.Checkpoint.State.Pending[i]
		if call.ID != p.ID || call.Name != p.Name || !bytes.Equal(call.Arguments, p.Arguments) {
			return ReplayPlan{}, ErrReplayEvidence
		}
		step := ReplayStep{
			Call: cloneCalls([]model.ToolCall{e.Call})[0],
		}
		switch e.Basis {
		case BasisUnknown:
			if e.Result != nil || e.IdempotencyKey != "" {
				return ReplayPlan{}, ErrReplayEvidence
			}
			step.Action = ActionHold
			rp.Blocked = true
		case BasisNotStarted:
			if e.Result != nil || e.IdempotencyKey != "" {
				return ReplayPlan{}, ErrReplayEvidence
			}
			step.Action = ActionInvoke
		case BasisResultKnown:
			step.Action = ActionReuse
			if e.Result == nil || e.Result.Role != "tool" || e.Result.ToolCallID != call.ID || len(e.Result.ToolCalls) != 0 || e.IdempotencyKey != "" {
				return ReplayPlan{}, ErrReplayEvidence
			}
			step.Result = &cloneMessages([]model.Message{*e.Result})[0]
		case BasisBackendDedup:
			key := e.IdempotencyKey
			if strings.TrimSpace(key) == "" || e.Result != nil || idKeys[key] {
				return ReplayPlan{}, ErrReplayEvidence
			}
			step.Action = ActionInvoke
			step.IdempotencyKey = key
			idKeys[key] = true
		default:
			return ReplayPlan{}, ErrReplayEvidence
		}
		rp.Steps = append(rp.Steps, step)
	}
	return rp, nil
}
