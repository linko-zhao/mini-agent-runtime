package agent

import (
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

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

func WithVisibleTools(names ...string) Option {
	snapshot := append([]string(nil), names...)
	return func(a *Agent) {
		a.visibilitySet = true
		a.visibleTools = append([]string(nil), snapshot...)
	}
}

func WithAuthorizer(auth tool.Authorizer) Option {
	return func(a *Agent) {
		a.authorizationSet = true
		a.authorizer = auth
	}
}
