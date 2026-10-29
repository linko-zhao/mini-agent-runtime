package agent

import (
	"context"
	"sync"
	"time"
)

type EventType string

const (
	RunStarted     EventType = "run_started"
	RunFailed      EventType = "run_failed"
	RunFinished    EventType = "run_finished"
	ToolStarted    EventType = "tool_started"
	ToolCompleted  EventType = "tool_completed"
	StepStarted    EventType = "step_started"
	StepFinished   EventType = "step_finished"
	ModelCompleted EventType = "model_completed"
)

type EventStatus string

const (
	EventOk        EventStatus = "ok"
	EventError     EventStatus = "error"
	EventCancelled EventStatus = "cancelled"
)

type Event struct {
	Type       EventType
	Step       int
	At         time.Time
	ToolName   string
	ToolCallID string
	Duration   time.Duration
	Status     EventStatus
}

type EventSink interface {
	Emit(ctx context.Context, e Event)
}

type SynchronizedSink struct {
	mu sync.Mutex
	sink EventSink
}

func (s *SynchronizedSink) Emit(ctx context.Context, e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sink.Emit(ctx, e)
}
