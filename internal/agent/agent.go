package agent

import (
	"context"
	"fmt"

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

	for range a.maxSteps {
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
			tr, err := a.tools[t.Name].Execute(ctx, t.Arguments)
			if err != nil {
				tm.Content = err.Error()
			} else {
				tm.Content = tr
			}
			msgs = append(msgs, tm)
		}
		fmt.Println(msgs)
	}

	return "", fmt.Errorf("arrive max steps")
}
