package agent

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

const DefaultToolTimeout = 300 * time.Second

type Agent struct {
	model         model.Model
	tools         map[string]tool.Tool
	maxSteps      int
	eventSink     EventSink
	visibleTools  []string
	visibilitySet bool
	toolTimeout   time.Duration
}

func New(m model.Model, tools map[string]tool.Tool, maxSteps int, opts ...Option) (*Agent, error) {
	if maxSteps <= 0 {
		return nil, fmt.Errorf("maxSteps must greater than 0")
	}
	a := Agent{model: m, tools: make(map[string]tool.Tool), maxSteps: maxSteps, toolTimeout: DefaultToolTimeout}

	for _, opt := range opts {
		if opt != nil {
			opt(&a)
		}
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
	var defs [] model.ToolDefinition
	if a.visibilitySet {
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
	} else {
		for _, name := range names {
			if dt, ok := a.tools[name].(tool.DescribedTool); ok {
				d := dt.Definition()
				if d.Name != name {
					return nil, fmt.Errorf("agent: definition name mismatch")
				}
				defs = append(defs, d)
			}
		}
	}

	if tb, ok := a.model.(model.ToolBinder); ok {
		bound, err := tb.WithTools(defs)
		if err != nil {
			return nil, err
		}
		if bound == nil {
			return nil, fmt.Errorf("agent: nil bound model")
		}
		a.model = bound
	}

	return &a, nil
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	m := model.Message{
		Role:    "user",
		Content: input,
	}
	msgs := []model.Message{m}

	var step int
	runStartTime := time.Now()
	a.emit(ctx, Event{
		Type: RunStarted,
		At:   runStartTime,
	})

	for step = range a.maxSteps {
		if ctx.Err() != nil {
			a.emit(ctx, Event{
				Type: RunFailed,
				At:   time.Now(),
			})
			return "", fmt.Errorf("run context error: %w", ctx.Err())
		}
		stepStartTime := time.Now()
		a.emit(ctx, Event{
			Type: StepStarted,
			Step: step + 1,
			At:   stepStartTime,
		})
		r, err := a.model.Generate(ctx, msgs)
		if err != nil {
			a.emit(ctx, Event{
				Type:     ModelCompleted,
				Step:     step + 1,
				At:       time.Now(),
				Duration: time.Since(stepStartTime),
				Status:   EventError,
			})
			a.emit(ctx, Event{Type: RunFailed, At: time.Now()})
			return "", fmt.Errorf("model generate error: %w", err)
		}
		a.emit(ctx, Event{
			Type:     ModelCompleted,
			Step:     step + 1,
			At:       time.Now(),
			Duration: time.Since(stepStartTime),
			Status:   EventOk,
		})

		msgs = append(msgs, r.Message)

		if len(r.Message.ToolCalls) == 0 {
			a.emit(ctx, Event{
				Type:     StepFinished,
				Step:     step + 1,
				At:       time.Now(),
				Duration: time.Since(stepStartTime),
			})
			a.emit(ctx, Event{
				Type:     RunFinished,
				At:       time.Now(),
				Duration: time.Since(runStartTime),
			})
			return r.Message.Content, nil
		}

		results, err := a.executeBatch(ctx, r.Message.ToolCalls, step)
		if err != nil {
			a.emit(ctx, Event{
				Type: RunFailed,
				At: time.Now(),
			})
			return "", err
		}
		msgs = append(msgs, results...)

		a.emit(ctx, Event{
			Type:     StepFinished,
			Step:     step + 1,
			At:       time.Now(),
			Duration: time.Since(stepStartTime),
		})
	}

	if step >= a.maxSteps {
		a.emit(ctx, Event{
			Type: RunFailed,
			At:   time.Now(),
		})
	} else {
		a.emit(ctx, Event{
			Type:     RunFinished,
			At:       time.Now(),
			Duration: time.Since(runStartTime),
		})
	}

	return "", fmt.Errorf("arrive max steps")
}

func (a *Agent) emit(ctx context.Context, e Event) {
	if a.eventSink == nil {
		return
	}
	a.eventSink.Emit(ctx, e)
}
