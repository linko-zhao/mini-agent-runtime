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
	ToolFailed EventType = "tool_failed"
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
