package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

const toolTimeout = 300 * time.Second

type Agent struct {
	model    model.Model
	tools    map[string]tool.Tool
	maxSteps int
	eventSink EventSink
}

func New(m model.Model, tools map[string]tool.Tool, maxSteps int, opts ...Option) (*Agent, error) {
	a := Agent{model: m, tools: tools, maxSteps: maxSteps}

	for _, opt := range opts {
		if opt != nil {
			opt(&a)
		}
	}

	if tb, ok := a.model.(model.ToolBinder); ok {
		var ts []model.ToolDefinition
		for _, t := range a.tools {
			if td, ok := t.(tool.DescribedTool); ok {
				ts = append(ts, td.Definition())
			}
		}
		bound, err := tb.WithTools(ts)
		if err != nil {
			return nil, err
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
		At: runStartTime,
	})

	for step = range a.maxSteps {
		slog.InfoContext(ctx, "generating model response", "step", step+1)
		if ctx.Err() != nil {
			a.emit(ctx, Event{
				Type: RunFailed,
				At: time.Now(),
			})
			return "", ctx.Err()
		}
		stepStartTime := time.Now()
		a.emit(ctx, Event{
			Type: StepStarted,
			Step: step+1,
			At: stepStartTime,
		})
		r, err := a.model.Generate(ctx, msgs)
		if err != nil {
			a.emit(ctx, Event{
				Type: ModelCompleted,
				Step: step+1,
				At: time.Now(),
				Duration: time.Since(stepStartTime),
				Status: EventError,
			})
			continue
		}
		a.emit(ctx, Event{
			Type: ModelCompleted,
			Step: step+1,
			At: time.Now(),
			Duration: time.Since(stepStartTime),
			Status: EventOk,
		})

		msgs = append(msgs, r.Message)

		if len(r.Message.ToolCalls) == 0 {
			a.emit(ctx, Event{
				Type: RunFinished,
				At: time.Now(),
				Duration: time.Since(runStartTime),
			})
			return r.Message.Content, nil
		}

		for _, t := range r.Message.ToolCalls {
			tm := model.Message{
				Role:       "tool",
				ToolCallID: t.ID,
			}
			tool, ok := a.tools[t.Name]
			if !ok {
				tm.Content = fmt.Sprintf("tool %s not found", t.Name)
				continue
			}
			toolStartTime := time.Now()
			a.emit(ctx, Event{
				Type: ToolStarted,
				Step: step+1,
				At: toolStartTime,
				ToolName: t.Name,
				ToolCallID: t.ID,
			})

			slog.InfoContext(ctx, "executing tool", "name", t.Name)
			toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
			defer cancel()
			tr, err := tool.Execute(toolCtx, t.Arguments)
			if toolCtx.Err() != nil {
				a.emit(ctx, Event{
					Type: RunFailed,
					At: time.Now(),
				})
				return "", ctx.Err()
			}
			toolCallStatus := EventOk
			if err != nil {
				slog.ErrorContext(ctx, "tool execution failed", "name", t.Name, "error", err)
				tm.Content = err.Error()
				toolCallStatus = EventError
			} else {
				tm.Content = tr
			}
			a.emit(ctx, Event{
				Type: ToolCompleted,
				Step: step+1,
				At: time.Now(),
				ToolName: t.Name,
				ToolCallID: t.ID,
				Duration: time.Since(toolStartTime),
				Status: toolCallStatus,
			})
			msgs = append(msgs, tm)
		}

		a.emit(ctx, Event{
			Type: StepFinished,
			Step: step+1,
			At: time.Now(),
			Duration: time.Since(stepStartTime),
		})
	}

	if step >= a.maxSteps {
		a.emit(ctx, Event{
			Type: RunFailed,
			At: time.Now(),
		})
	} else {
		a.emit(ctx, Event{
			Type: RunFinished,
			At: time.Now(),
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
