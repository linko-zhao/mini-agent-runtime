package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"sort"
	"time"
)

var ErrMaxSteps = errors.New("agent: max steps exceeded")

type Agent struct {
	model       model.Model
	tools       map[string]tool.Tool
	maxSteps    int
	sink        EventSink
	toolTimeout time.Duration
}

// New 保留 map 注册形状，复制注册表并验证 key == Tool.Name。
// map 构造前的重复 key 已被 Go 覆盖，调用方必须在组装 map 时拒绝重复。
func New(m model.Model, tools map[string]tool.Tool, maxSteps int, opts ...Option) (*Agent, error) {
	if m == nil || maxSteps <= 0 {
		return nil, fmt.Errorf("agent: model and positive maxSteps required")
	}
	a := &Agent{model: m, tools: make(map[string]tool.Tool), maxSteps: maxSteps}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	if a.toolTimeout < 0 {
		return nil, fmt.Errorf("agent: negative timeout")
	}
	names := make([]string, 0, len(tools))
	for name, t := range tools {
		if t == nil || name == "" || name != t.Name() {
			return nil, fmt.Errorf("agent: invalid tool registration %q", name)
		}
		a.tools[name] = t
		names = append(names, name)
	}
	sort.Strings(names)
	var defs []model.ToolDefinition
	for _, name := range names {
		if t, ok := a.tools[name].(tool.DescribedTool); ok {
			d := t.Definition()
			if d.Name != name {
				return nil, fmt.Errorf("agent: definition name mismatch")
			}
			defs = append(defs, d)
		}
	}
	if binder, ok := m.(model.ToolBinder); ok {
		bound, err := binder.WithTools(defs)
		if err != nil {
			return nil, err
		}
		if bound == nil {
			return nil, fmt.Errorf("agent: nil bound model")
		}
		a.model = bound
	}
	return a, nil
}
func status(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	if err != nil {
		return "error"
	}
	return "ok"
}
func (a *Agent) Run(ctx context.Context, input string) (output string, runErr error) {
	started := time.Now()
	step := 0
	a.emit(ctx, Event{Type: EventRunStarted, Status: "ok"})
	defer func() {
		kind := EventRunFinished
		if runErr != nil {
			kind = EventRunFailed
		}
		a.emit(ctx, Event{Type: kind, Step: step, Duration: time.Since(started), Status: status(runErr)})
	}()
	messages := []model.Message{{Role: "user", Content: input}}
	for step = 1; step <= a.maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		a.emit(ctx, Event{Type: EventStepStarted, Step: step, Status: "ok"})
		start := time.Now()
		r, err := a.model.Generate(ctx, messages)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil && r == nil {
			err = fmt.Errorf("agent: nil model response")
		}
		a.emit(ctx, Event{Type: EventModelCompleted, Step: step, Duration: time.Since(start), Status: status(err)})
		if err != nil {
			return "", err
		}
		if r.Message.Role != "assistant" {
			return "", fmt.Errorf("agent: expected assistant message")
		}
		messages = append(messages, r.Message)
		if len(r.Message.ToolCalls) == 0 {
			return r.Message.Content, nil
		}
		// 整批预校验，避免后面的未知工具或重复 ID 导致前面的工具已产生副作用。
		seen := map[string]bool{}
		for _, call := range r.Message.ToolCalls {
			if call.ID == "" || seen[call.ID] {
				return "", fmt.Errorf("agent: missing or duplicate tool call ID")
			}
			seen[call.ID] = true
			if _, ok := a.tools[call.Name]; !ok {
				return "", &tool.Error{Kind: tool.NotFoundError, Message: "tool not registered"}
			}
		}
		for _, call := range r.Message.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			start := time.Now()
			e := Event{Type: EventToolStarted, Step: step, ToolName: call.Name, ToolCallID: call.ID, Status: "ok"}
			a.emit(ctx, e)
			toolCtx := ctx
			cancel := func() {}
			if a.toolTimeout > 0 {
				toolCtx, cancel = context.WithTimeout(ctx, a.toolTimeout)
			}
			result, err := a.tools[call.Name].Execute(toolCtx, call.Arguments)
			if toolCtx.Err() != nil {
				err = toolCtx.Err()
			}
			cancel()
			e.Type = EventToolCompleted
			e.Duration = time.Since(start)
			e.Status = status(err)
			a.emit(ctx, e)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			if err != nil {
				// 仅回填受控提示，未知内部错误不暴露给模型。
				var known *tool.Error
				if errors.As(err, &known) {
					result = known.Error()
				} else {
					result = "execution_failed: tool execution failed"
				}
			}
			messages = append(messages, model.Message{Role: "tool", ToolCallID: call.ID, Content: result})
		}
	}
	step = a.maxSteps
	return "", ErrMaxSteps
}
