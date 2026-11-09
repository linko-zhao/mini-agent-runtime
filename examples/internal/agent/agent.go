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
	model            model.Model
	tools            map[string]tool.Tool
	maxSteps         int
	sink             EventSink
	visibleTools     []string
	visibilitySet    bool
	toolTimeout      time.Duration
	authorizer       tool.Authorizer
	authorizationSet bool
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
	if a.authorizationSet && a.authorizer == nil {
		return nil, fmt.Errorf("agent: nil authorizer")
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
	if a.visibilitySet {
		defs = nil
		visible := map[string]bool{}
		for _, name := range a.visibleTools {
			if _, ok := a.tools[name]; !ok {
				return nil, fmt.Errorf("agent: visible tool %q not registered", name)
			}
			visible[name] = true
		}
		for _, name := range names {
			if visible[name] {
				t, ok := a.tools[name].(tool.DescribedTool)
				if !ok {
					return nil, fmt.Errorf("agent: visible tool %q has no definition", name)
				}
				defs = append(defs, t.Definition())
			}
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
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	result, err := a.RunWithState(ctx, input)
	return result.Output, err
}

// RunWithState returns a final, independent observation snapshot, even on a Run error.
// It does not persist, resume, or authorize replay from a snapshot.
func (a *Agent) RunWithState(ctx context.Context, input string) (result RunResult, runErr error) {
	record, err := newRunRecord(input)
	if err != nil {
		return RunResult{}, err
	}
	if err = record.transition(RunRunning); err != nil {
		return RunResult{Snapshot: record.snapshot()}, err
	}
	started := time.Now()
	a.emit(ctx, Event{Type: EventRunStarted, Status: "ok"})
	defer func() {
		runErr = record.finish(runErr)
		if runErr != nil {
			result.Output = ""
		}
		result.Snapshot = record.snapshot()
		kind := EventRunFinished
		if runErr != nil {
			kind = EventRunFailed
		}
		a.emit(ctx, Event{Type: kind, Step: record.data.Step, Duration: time.Since(started), Status: status(runErr)})
	}()
	for record.data.Step = 1; record.data.Step <= a.maxSteps; record.data.Step++ {
		if err := ctx.Err(); err != nil {
			return RunResult{}, err
		}
		a.emit(ctx, Event{Type: EventStepStarted, Step: record.data.Step, Status: "ok"})
		start := time.Now()
		r, err := a.model.Generate(ctx, record.data.Messages)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil && r == nil {
			err = fmt.Errorf("agent: nil model response")
		}
		a.emit(ctx, Event{Type: EventModelCompleted, Step: record.data.Step, Duration: time.Since(start), Status: status(err)})
		if err != nil {
			return RunResult{}, err
		}
		if r.Message.Role != "assistant" {
			return RunResult{}, fmt.Errorf("agent: expected assistant message")
		}
		record.data.Messages = append(record.data.Messages, r.Message)
		if len(r.Message.ToolCalls) == 0 {
			return RunResult{Output: r.Message.Content}, nil
		}
		results, err := a.executeBatchWithState(ctx, r.Message.ToolCalls, record.data.Step, record)
		if err != nil {
			return RunResult{}, err
		}
		record.data.Messages = append(record.data.Messages, results...)
	}
	record.data.Step = a.maxSteps
	return RunResult{}, ErrMaxSteps
}
