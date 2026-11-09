package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type RunStatus string

const (
	RunCreated   RunStatus = "created"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type CallStatus string

const (
	CallPlanned     CallStatus = "planned"
	CallRunning     CallStatus = "running"
	CallReturned    CallStatus = "returned"
	CallErrored     CallStatus = "errored"
	CallUnconfirmed CallStatus = "unconfirmed"
	CallBlocked     CallStatus = "blocked"
)

var ErrInvalidTransition = errors.New("agent: invalid state transition")

// CallRecord describes Runtime observations, not a remote commit/rollback.
// Step+Index identifies the position even in a batch with invalid duplicate IDs.
type CallRecord struct {
	Step, Index      int
	CallID, ToolName string
	Status           CallStatus
	Result           *model.Message
}
type RunSnapshot struct {
	RunID        string
	Status       RunStatus
	Step         int
	Messages     []model.Message
	Calls        []CallRecord
	FailureClass string // controlled category, never the raw error text
}
type RunResult struct {
	Output   string
	Snapshot RunSnapshot
}

func CanTransition(from, to RunStatus) bool {
	return (from == RunCreated && to == RunRunning) || (from == RunRunning && (to == RunCompleted || to == RunFailed || to == RunCancelled))
}
func cloneMessage(m model.Message) model.Message {
	if m.ToolCalls != nil {
		calls := make([]model.ToolCall, len(m.ToolCalls))
		copy(calls, m.ToolCalls)
		for i := range calls {
			if calls[i].Arguments != nil {
				raw := make([]byte, len(calls[i].Arguments))
				copy(raw, calls[i].Arguments)
				calls[i].Arguments = raw
			}
		}
		m.ToolCalls = calls
	}
	return m
}

// Clone copies nested call arguments and per-call result pointers.
// Snapshots may contain private prompts/results and are not public audit events.
func (s RunSnapshot) Clone() RunSnapshot {
	if s.Messages != nil {
		messages := make([]model.Message, len(s.Messages))
		for i, m := range s.Messages {
			messages[i] = cloneMessage(m)
		}
		s.Messages = messages
	}
	if s.Calls != nil {
		calls := make([]CallRecord, len(s.Calls))
		copy(calls, s.Calls)
		for i := range calls {
			if calls[i].Result != nil {
				m := cloneMessage(*calls[i].Result)
				calls[i].Result = &m
			}
		}
		s.Calls = calls
	}
	return s
}

type runRecord struct{ data RunSnapshot }

func newRunRecord(input string) (*runRecord, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("agent: create run ID: %w", err)
	}
	return &runRecord{data: RunSnapshot{RunID: hex.EncodeToString(id[:]), Status: RunCreated, Messages: []model.Message{{Role: "user", Content: input}}}}, nil
}
func (r *runRecord) transition(to RunStatus) error {
	if !CanTransition(r.data.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, r.data.Status, to)
	}
	r.data.Status = to
	return nil
}
func (r *runRecord) plan(calls []model.ToolCall, step int) (int, error) {
	if r.data.Status != RunRunning {
		return 0, ErrInvalidTransition
	}
	base := len(r.data.Calls)
	for i, c := range calls {
		r.data.Calls = append(r.data.Calls, CallRecord{Step: step, Index: i, CallID: c.ID, ToolName: c.Name, Status: CallPlanned})
	}
	return base, nil
}
func (r *runRecord) startCall(index int) error {
	if r.data.Status != RunRunning || index < 0 || index >= len(r.data.Calls) || r.data.Calls[index].Status != CallPlanned {
		return ErrInvalidTransition
	}
	r.data.Calls[index].Status = CallRunning
	return nil
}
func (r *runRecord) completeCall(index int, result *model.Message, err error) error {
	if r.data.Status != RunRunning || index < 0 || index >= len(r.data.Calls) || r.data.Calls[index].Status != CallRunning {
		return ErrInvalidTransition
	}
	call := &r.data.Calls[index]
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		call.Status = CallUnconfirmed
		return nil
	}
	if result == nil || result.Role != "tool" || result.ToolCallID != call.CallID {
		return ErrInvalidTransition
	}
	m := cloneMessage(*result)
	call.Result = &m
	call.Status = CallReturned
	if err != nil {
		call.Status = CallErrored
	}
	return nil
}
func failureClass(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, ErrMaxSteps):
		return "max_steps"
	case errors.Is(err, ErrInvalidTransition):
		return "invalid_transition"
	default:
		return "run_error"
	}
}
func (r *runRecord) finish(runErr error) error {
	if r.data.Status != RunRunning {
		return errors.Join(runErr, ErrInvalidTransition)
	}
	for i := range r.data.Calls {
		c := &r.data.Calls[i]
		if c.Status == CallPlanned || c.Status == CallRunning {
			if runErr == nil {
				runErr = fmt.Errorf("%w: unfinished calls", ErrInvalidTransition)
			}
			if c.Status == CallPlanned {
				c.Status = CallBlocked
			} else {
				c.Status = CallUnconfirmed
			}
		}
	}
	target := RunCompleted
	if runErr != nil {
		target = RunFailed
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		target = RunCancelled
	}
	if err := r.transition(target); err != nil {
		runErr = errors.Join(runErr, err)
	}
	r.data.FailureClass = failureClass(runErr)
	return runErr
}
func (r *runRecord) snapshot() RunSnapshot { return r.data.Clone() }
