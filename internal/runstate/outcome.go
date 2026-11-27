package runstate

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

var (
	ErrOutcomeState          = errors.New("runstate: invalid outcome observation")
	ErrOutcomeSourceRequired = errors.New("runstate: outcome source required")
)

// ToolAttempt identifies the logical call being inspected, not a network retry
// counter. The host must additionally bind tenant, authority and backend scope.
type ToolAttempt struct {
	RunID string
	Step  int
	Call  model.ToolCall
}

type ToolOutcomeEvidence struct {
	RunID    string
	Step     int
	Evidence ReplayEvidence
}

// ToolOutcomeSource is a trusted read-only host adapter. It must cooperate with
// ctx, support intended concurrency and return owned buffers; Lookup must not
// execute/retry the business operation while looking for its outcome.
type ToolOutcomeSource interface {
	Lookup(context.Context, ToolAttempt) (ToolOutcomeEvidence, error)
}

// ToolOutcomeReport is a point-in-time report, not fresh-state proof, a restored
// Run or authorization. Results is nil unless ALL results are positively known.
type ToolOutcomeReport struct {
	Observed        State
	Evidence        []ToolOutcomeEvidence
	AllResultsKnown bool
	Results         []model.Message
}

func validateEvidence(e ReplayEvidence, c model.ToolCall) bool {
	if e.Call.ID != c.ID || e.Call.Name != c.Name || !bytes.Equal(e.Call.Arguments, c.Arguments) || e.IdempotencyKey != "" {
		return false
	}
	switch e.Basis {
	case BasisUnknown, BasisNotStarted:
		if e.Result != nil {
			return false
		}
	case BasisResultKnown:
		if e.Result == nil || e.Result.Role != "tool" || e.Result.ToolCallID != e.Call.ID || len(e.Result.ToolCalls) != 0 {
			return false
		}
	default:
		return false
	}
	return true
}

// InspectToolOutcomes only reads evidence; it never executes business calls.
func InspectToolOutcomes(ctx context.Context, source ToolOutcomeSource, observed State) (ToolOutcomeReport, error) {
	if ctx == nil {
		return ToolOutcomeReport{}, ErrOutcomeState
	}
	if ctx.Err() != nil {
		return ToolOutcomeReport{}, ctx.Err()
	}
	if strings.TrimSpace(observed.RunID) == "" || observed.Phase != ToolsRunning || observed.MaxSteps <= 0 || observed.Step <= 0 || observed.Step > observed.MaxSteps || observed.Result != "" || observed.Failure != "" || len(observed.Messages) == 0 || len(observed.Pending) == 0 {
		return ToolOutcomeReport{}, ErrOutcomeState
	}
	msgs := observed.Messages
	first := msgs[0]
	last := msgs[len(msgs)-1]
	if first.Role != "user" || last.Role != "assistant" || len(last.ToolCalls) != len(observed.Pending) {
		return ToolOutcomeReport{}, ErrOutcomeState
	}
	pending := observed.Pending
	ids := map[string]bool{}
	for i, c := range last.ToolCalls {
		if c.ID != pending[i].ID || c.Name != pending[i].Name || !bytes.Equal(c.Arguments, pending[i].Arguments) || ids[c.ID] {
			return ToolOutcomeReport{}, ErrOutcomeState
		}
		ids[c.ID] = true
	}
	if source == nil {
		return ToolOutcomeReport{}, ErrOutcomeSourceRequired
	}
	report := ToolOutcomeReport{Observed: cloneState(observed), AllResultsKnown: true}
	for _, c := range observed.Pending {
		if ctx.Err() != nil {
			return ToolOutcomeReport{}, ctx.Err()
		}
		e, err := source.Lookup(ctx, ToolAttempt{RunID: observed.RunID, Step: observed.Step, Call: cloneCalls([]model.ToolCall{c})[0]})
		if ctx.Err() != nil {
			return ToolOutcomeReport{}, ctx.Err()
		}
		if err != nil {
			return ToolOutcomeReport{}, err
		}
		if e.RunID != observed.RunID || e.Step != observed.Step || !validateEvidence(e.Evidence, c) {
			return ToolOutcomeReport{}, ErrReplayEvidence
		}
		if e.Evidence.Basis != BasisResultKnown {
			report.AllResultsKnown = false
		}
		if report.AllResultsKnown {
			report.Results = append(report.Results, cloneMessages([]model.Message{*e.Evidence.Result})[0])
		} else {
			report.Results = nil
		}
		if e.Evidence.Result != nil {
			msg := cloneMessages([]model.Message{*e.Evidence.Result})[0]
			e.Evidence.Result = &msg
		}
		e.Evidence.Call = cloneCalls([]model.ToolCall{c})[0]
		report.Evidence = append(report.Evidence, e)
	}
	if ctx.Err() != nil {
		return ToolOutcomeReport{}, ctx.Err()
	}
	return report, nil
}
