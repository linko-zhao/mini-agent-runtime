package agent

import (
	"context"
	"time"
)

type EventType string

const (
	RunStarted EventType = "run_started"
	RunFailed EventType = "run_failed"
	RunFinished EventType = "run_finished"
	ToolStarted EventType = "tool_started"
	ToolCompleted EventType = "tool_completed"
	StepStarted EventType = "step_started"
	ModelCompleted EventType = "model_completed"
)

type Event struct {
	Type EventType
	Step int
	At time.Time
	ToolName string
	ToolCallID string
	Duration time.Duration
	Status string
}

type EventSink interface {
	Emit(ctx context.Context, e Event)
}
