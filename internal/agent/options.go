package agent

import "time"

type Option func(*Agent)

func WithEventSink(sink EventSink) Option {
	return func(a *Agent) {
		a.eventSink = sink
	}
}

func WithToolTimeout(toolTimeout time.Duration) Option {
	return func(a *Agent) {
		a.toolTimeout = toolTimeout
	}
}
