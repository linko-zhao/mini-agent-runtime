package agent

import (
	"context"
	"sync"
	"time"
)

type EventType string

const (
	EventRunStarted     EventType = "run_started"
	EventStepStarted    EventType = "step_started"
	EventModelCompleted EventType = "model_completed"
	EventToolStarted    EventType = "tool_started"
	EventToolCompleted  EventType = "tool_completed"
	EventRunFinished    EventType = "run_finished"
	EventRunFailed      EventType = "run_failed"
)

type Event struct {
	Type       EventType
	Step       int
	At         time.Time
	ToolName   string
	ToolCallID string
	Duration   time.Duration
	Status     string
}
type EventSink interface{ Emit(context.Context, Event) }

// SynchronizedSink 可被多个 Agent 共享；所有调用必须经同一个包装器。
// 下游不得重入此包装器，否则会死锁。
type SynchronizedSink struct {
	mu   sync.Mutex
	sink EventSink
}

func NewSynchronizedSink(s EventSink) *SynchronizedSink { return &SynchronizedSink{sink: s} }
func (s *SynchronizedSink) Emit(ctx context.Context, e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sink != nil {
		s.sink.Emit(ctx, e)
	}
}
func (a *Agent) emit(ctx context.Context, e Event) {
	if a.sink != nil {
		e.At = time.Now()
		a.sink.Emit(ctx, e)
	}
}
