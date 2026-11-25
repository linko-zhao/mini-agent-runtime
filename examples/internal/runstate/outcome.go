package runstate

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
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

func outcomeStateValid(s State) bool {
	if strings.TrimSpace(s.RunID) == "" || s.Phase != ToolsRunning || s.MaxSteps <= 0 || s.Step <= 0 || s.Step > s.MaxSteps ||
		s.Result != "" || s.Failure != "" || len(s.Messages) == 0 || s.Messages[0].Role != "user" || len(s.Pending) == 0 {
		return false
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != "assistant" || len(last.ToolCalls) != len(s.Pending) {
		return false
	}
	ids := map[string]bool{}
	for i, call := range s.Pending {
		h := last.ToolCalls[i]
		if call.ID == "" || strings.TrimSpace(call.Name) == "" || ids[call.ID] || call.ID != h.ID || call.Name != h.Name || !bytes.Equal(call.Arguments, h.Arguments) {
			return false
		}
		ids[call.ID] = true
	}
	return true
}

// InspectToolOutcomes queries each pending logical call once in input order.
// Every failure returns zero report. It never Apply/Save/Invoke, retries a
// query or converts missing responses into confirmed not-started evidence.
func InspectToolOutcomes(ctx context.Context, source ToolOutcomeSource, observed State) (ToolOutcomeReport, error) {
	if ctx == nil {
		return ToolOutcomeReport{}, ErrOutcomeState
	}
	if err := ctx.Err(); err != nil {
		return ToolOutcomeReport{}, err
	}
	if !outcomeStateValid(observed) {
		return ToolOutcomeReport{}, ErrOutcomeState
	}
	if source == nil {
		return ToolOutcomeReport{}, ErrOutcomeSourceRequired
	}
	out := ToolOutcomeReport{Observed: cloneState(observed), Evidence: make([]ToolOutcomeEvidence, len(observed.Pending)), AllResultsKnown: true}
	results := make([]model.Message, len(observed.Pending))
	for i, call := range out.Observed.Pending {
		if err := ctx.Err(); err != nil {
			return ToolOutcomeReport{}, err
		}
		input := ToolAttempt{RunID: observed.RunID, Step: observed.Step, Call: cloneCalls([]model.ToolCall{call})[0]}
		e, err := source.Lookup(ctx, input)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return ToolOutcomeReport{}, cancelErr
		}
		if err != nil {
			return ToolOutcomeReport{}, err
		}
		v := e.Evidence
		if e.RunID != observed.RunID || e.Step != observed.Step || v.Call.ID != call.ID || v.Call.Name != call.Name ||
			!bytes.Equal(v.Call.Arguments, call.Arguments) || v.IdempotencyKey != "" {
			return ToolOutcomeReport{}, ErrReplayEvidence
		}
		switch v.Basis {
		case BasisUnknown, BasisNotStarted:
			if v.Result != nil {
				return ToolOutcomeReport{}, ErrReplayEvidence
			}
			out.AllResultsKnown = false
		case BasisResultKnown:
			if v.Result == nil || v.Result.Role != "tool" || v.Result.ToolCallID != call.ID || len(v.Result.ToolCalls) != 0 {
				return ToolOutcomeReport{}, ErrReplayEvidence
			}
			results[i] = cloneMessages([]model.Message{*v.Result})[0]
			msg := cloneMessages([]model.Message{*v.Result})[0]
			v.Result = &msg
		default: // BackendDedup is a retry policy assertion, not a known outcome.
			return ToolOutcomeReport{}, ErrReplayEvidence
		}
		v.Call = cloneCalls([]model.ToolCall{call})[0]
		out.Evidence[i] = ToolOutcomeEvidence{RunID: e.RunID, Step: e.Step, Evidence: v}
	}
	if err := ctx.Err(); err != nil {
		return ToolOutcomeReport{}, err
	}
	if out.AllResultsKnown {
		out.Results = results
	}
	return out, nil
}
