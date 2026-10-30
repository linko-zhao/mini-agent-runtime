package agent

import "time"

type Option func(*Agent)

func WithEventSink(s EventSink) Option { return func(a *Agent) { a.sink = s } }

// 零值禁用单工具超时；取消为合作式，工具必须遵守 ctx。
func WithToolTimeout(d time.Duration) Option { return func(a *Agent) { a.toolTimeout = d } }

// WithVisibleTools 只过滤提供给模型的描述，不改变 Runtime 注册集合，也不构成授权。
// 不传此选项时保持旧行为：暴露全部可描述工具；传空集合则不暴露任何描述。
func WithVisibleTools(names ...string) Option {
	snapshot := append([]string(nil), names...)
	return func(a *Agent) { a.visibilitySet = true; a.visibleTools = append([]string(nil), snapshot...) }
}
