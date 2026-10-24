package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

type Agent struct {
	model    model.Model
	tools    map[string]tool.Tool
	maxSteps int
}

func New(m model.Model, tools map[string]tool.Tool, maxSteps int) *Agent {
	return &Agent{model: m, tools: tools, maxSteps: maxSteps}
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	m := model.Message{
		Role:    "user",
		Content: input,
	}
	msgs := []model.Message{m}

	if tb, ok := a.model.(model.ToolBinder); ok {
		var ts []model.ToolDefinition
		for _, t := range a.tools {
			if td, ok := t.(tool.DescribedTool); ok {
				ts = append(ts, td.Definition())
			}
		}
		bound, err := tb.WithTools(ts)
		if err != nil {
			return "", err
		}
		a.model = bound
	}

	for step := range a.maxSteps {
		slog.InfoContext(ctx, "generating model response", "step", step+1)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		r, err := a.model.Generate(ctx, msgs)
		if err != nil {
			return "", err
		}

		msgs = append(msgs, r.Message)

		if len(r.Message.ToolCalls) == 0 {
			return r.Message.Content, nil
		}

		for _, t := range r.Message.ToolCalls {
			tm := model.Message{
				Role:       "tool",
				ToolCallID: t.ID,
			}
			tool, ok := a.tools[t.Name]
			if !ok {
				return "", fmt.Errorf("tool %q not found", t.Name)
			}
			slog.InfoContext(ctx, "executing tool", "name", t.Name)
			tr, err := tool.Execute(ctx, t.Arguments)
			if err != nil {
				slog.ErrorContext(ctx, "tool execution failed", "name", t.Name, "error", err)
				tm.Content = err.Error()
			} else {
				tm.Content = tr
			}
			msgs = append(msgs, tm)
		}
	}

	return "", fmt.Errorf("arrive max steps")
}
