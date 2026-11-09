// Package runstate models in-memory transitions. It is not a checkpoint store.
package runstate

import (
	"errors"
	"strings"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type Phase string

const (
	Ready        Phase = "ready"
	ModelRunning Phase = "model_running"
	ToolsPending Phase = "tools_pending"
	ToolsRunning Phase = "tools_running"
	Completed    Phase = "completed"
	Failed       Phase = "failed"
	Cancelled    Phase = "cancelled"
)

type EventKind string

const (
	StartModel    EventKind = "start_model"
	ModelReturned EventKind = "model_returned"
	StartTools    EventKind = "start_tools"
	ToolsReturned EventKind = "tools_returned"
	Abort         EventKind = "abort"
	Cancel        EventKind = "cancel"
)

var (
	ErrTransition = errors.New("runstate: invalid transition")
	ErrMaxSteps   = errors.New("runstate: max model rounds exceeded")
)

type State struct {
	RunID           string
	Phase           Phase
	Step, MaxSteps  int
	Messages        []model.Message
	Pending         []model.ToolCall
	Result, Failure string
}
type Event struct {
	Kind     EventKind
	Response *model.Response
	Results  []model.Message
	Reason   string
}

func New(runID, input string, maxSteps int) (State, error) {
	if strings.TrimSpace(runID) == "" || maxSteps <= 0 {
		return State{}, ErrTransition
	}
	return State{RunID: runID, Phase: Ready, MaxSteps: maxSteps, Messages: []model.Message{{Role: "user", Content: input}}}, nil
}
func cloneCalls(calls []model.ToolCall) []model.ToolCall {
	if calls == nil {
		return nil
	}
	out := append([]model.ToolCall{}, calls...)
	for i := range out {
		out[i].Arguments = append([]byte(nil), out[i].Arguments...)
	}
	return out
}
func cloneMessages(ms []model.Message) []model.Message {
	if ms == nil {
		return nil
	}
	out := append([]model.Message{}, ms...)
	for i := range out {
		out[i].ToolCalls = cloneCalls(out[i].ToolCalls)
	}
	return out
}
func cloneState(s State) State {
	s.Messages = cloneMessages(s.Messages)
	s.Pending = cloneCalls(s.Pending)
	return s
}
func terminal(p Phase) bool { return p == Completed || p == Failed || p == Cancelled }

// Advance has no I/O. The host performs calls between start/returned events.
// It accepts states from New/Advance and never mutates input state/event buffers.
func Advance(current State, e Event) (State, error) {
	if terminal(current.Phase) {
		return State{}, ErrTransition
	}
	next := cloneState(current)
	switch e.Kind {
	case StartModel:
		if current.Phase != Ready {
			return State{}, ErrTransition
		}
		if current.Step >= current.MaxSteps {
			return State{}, ErrMaxSteps
		}
		next.Step++
		next.Phase = ModelRunning
	case ModelReturned:
		if current.Phase != ModelRunning || e.Response == nil || e.Response.Message.Role != "assistant" {
			return State{}, ErrTransition
		}
		msg := e.Response.Message
		ids := map[string]bool{}
		for _, call := range msg.ToolCalls {
			if call.ID == "" || strings.TrimSpace(call.Name) == "" || ids[call.ID] {
				return State{}, ErrTransition
			}
			ids[call.ID] = true
		}
		next.Messages = append(next.Messages, cloneMessages([]model.Message{msg})...)
		if len(msg.ToolCalls) == 0 {
			next.Phase = Completed
			next.Result = msg.Content
		} else {
			next.Phase = ToolsPending
			next.Pending = cloneCalls(msg.ToolCalls)
		}
	case StartTools:
		if current.Phase != ToolsPending {
			return State{}, ErrTransition
		}
		next.Phase = ToolsRunning
	case ToolsReturned:
		if current.Phase != ToolsRunning || len(e.Results) != len(current.Pending) {
			return State{}, ErrTransition
		}
		for i, msg := range e.Results {
			if msg.Role != "tool" || msg.ToolCallID != current.Pending[i].ID {
				return State{}, ErrTransition
			}
		}
		next.Messages = append(next.Messages, cloneMessages(e.Results)...)
		next.Pending = nil
		next.Phase = Ready
	case Abort:
		if strings.TrimSpace(e.Reason) == "" {
			return State{}, ErrTransition
		}
		next.Phase = Failed
		next.Failure = e.Reason
	case Cancel:
		next.Phase = Cancelled
		next.Failure = "cancelled"
	default:
		return State{}, ErrTransition
	}
	return next, nil
}
