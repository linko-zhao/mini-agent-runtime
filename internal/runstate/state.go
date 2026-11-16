package runstate

import (
	"errors"
	"strings"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type Phase string

const (
	Ready Phase = "ready"
	ModelRunning Phase = "model_running"
	ToolsPending Phase = "tools_pending"
	ToolsRunning Phase = "tools_running"
	Completed Phase = "completed"
	Failed Phase = "failed"
	Cancelled Phase = "cancelled"
)

type EventKind string

const (
	StartModel EventKind = "start_model"
	ModelReturned EventKind = "model_returned"
	StartTools EventKind = "start_tools"
	ToolsReturned EventKind = "tools_returned"
	Abort EventKind = "abort"
	Cancel EventKind = "cancel"
)

var (
	ErrTransition = errors.New("runstate: invalid transition")
	ErrMaxSteps = errors.New("runstate: max model rounds exceeded")
)

type State struct {
	RunID string
	Phase Phase
	Step, MaxSteps int
	Messages []model.Message
	Pending []model.ToolCall
	Result, Failure string
}

type Event struct {
	Kind EventKind
	Response *model.Response
	Results []model.Message
	Reason string
}

func New(runID, input string, maxSteps int) (State, error) {
	var s State
	if strings.TrimSpace(runID) == "" || maxSteps <= 0 {
		return s, ErrTransition
	}
	s.RunID = runID
	s.MaxSteps = maxSteps
	s.Phase = Ready
	if input != "" {
		s.Messages = append([]model.Message(nil), model.Message{ Role: "user", Content: input })
	}
	return s, nil
}

func cloneCalls(calls []model.ToolCall) []model.ToolCall {
	panic("TODO: cloneCalls")
}

func cloneMessages(ms []model.Message) []model.Message {
	panic("TODO: cloneMessages")
}

func cloneState(s State) State {
	panic("TODO: cloneState")
}

func terminal(p Phase) bool {
	panic("TODO: terminal")
}

func Advance(current State, e Event) (State, error) {
	panic("TODO: Advance")
}
