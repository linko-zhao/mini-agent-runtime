package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)


func (a *Agent) executeBatch(ctx context.Context, calls []model.ToolCall, step int) ([]model.Message, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(calls) == 0 {
		return nil, fmt.Errorf("agent: tool call list empty")
	}

	seen := make(map[string]bool, 0)
	for _, call := range calls {
		if call.ID == "" || seen[call.ID] {
			return nil, fmt.Errorf("agent: missing or duplicate tool call ID")
		}
		seen[call.ID] = true
		if _, ok := a.tools[call.Name]; !ok {
			return nil, &tool.Error{Kind: tool.NotFoundError, Message: fmt.Sprintf("tool %q not registered", call.Name)}
		}
	}

	msgs := make([]model.Message, len(calls))
	for i, call := range calls {
		var t tool.Tool
		t, ok := a.tools[call.Name]
		if !ok {
			return nil, fmt.Errorf("agent: tool %q not exist", call.Name)
		}
		toolStartTime := time.Now()
		a.emit(ctx, Event{
			Type:       ToolStarted,
			Step:       step + 1,
			At:         toolStartTime,
			ToolName:   call.Name,
			ToolCallID: call.ID,
		})

		toolCallCtx, cancel := context.WithTimeout(ctx, a.toolTimeout)
		tr, err := t.Execute(toolCallCtx, call.Arguments)
		toolErr := toolCallCtx.Err()
		cancel()
		if toolErr != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				a.emit(ctx, Event{
					Type:       ToolCompleted,
					Step:       step + 1,
					At:         time.Now(),
					ToolName:   call.Name,
					ToolCallID: call.ID,
					Duration:   time.Since(toolStartTime),
					Status:     EventError,
				})
			}
			return nil, fmt.Errorf("agent: executing tool context error: %w", toolErr)
		}
		toolCallStatus := EventOk
		if err != nil {
			toolCallStatus = EventError
			var known *tool.Error
			if errors.As(err, &known) {
				tr = known.Error()
			} else {
				tr = "execution_failed: tool execution failed"
			}
		}
		a.emit(ctx, Event{
			Type:       ToolCompleted,
			Step:       step + 1,
			At:         time.Now(),
			ToolName:   call.Name,
			ToolCallID: call.ID,
			Duration:   time.Since(toolStartTime),
			Status:     toolCallStatus,
		})
		msgs[i] = model.Message{
			Role: "tool",
			ToolCallID: call.ID,
			Content: tr,
		}
	}
	return msgs, nil
}
