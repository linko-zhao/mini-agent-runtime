package agent

type Option func(*Agent)

func WithEventSink(sink EventSink) Option {
	return func(a *Agent) {
		a.eventSink = sink
	}
}
