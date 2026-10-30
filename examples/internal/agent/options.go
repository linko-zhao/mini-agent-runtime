package agent

import "time"

type Option func(*Agent)

func WithEventSink(s EventSink) Option { return func(a *Agent) { a.sink = s } }

// 零值禁用单工具超时；取消为合作式，工具必须遵守 ctx。
func WithToolTimeout(d time.Duration) Option { return func(a *Agent) { a.toolTimeout = d } }
