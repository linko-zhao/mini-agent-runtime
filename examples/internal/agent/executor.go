package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

// executeBatch validates the entire batch before executing any tool.
// Business arguments remain the tool's responsibility. Results preserve input
// order and call IDs; cancellation or timeout terminates the batch with an error.
func (a *Agent) executeBatch(ctx context.Context, calls []model.ToolCall, step int) ([]model.Message, error) {
	var results []model.Message
	// 整批预校验，避免后面的未知工具或重复 ID 导致前面的工具已产生副作用。
	seen := map[string]bool{}
	for _, call := range calls {
		if call.ID == "" || seen[call.ID] {
			return nil, fmt.Errorf("agent: missing or duplicate tool call ID")
		}
		seen[call.ID] = true
		if _, ok := a.tools[call.Name]; !ok {
			return nil, &tool.Error{Kind: tool.NotFoundError, Message: "tool not registered"}
		}
	}
	// Authorization is a whole-batch preflight, before any side effects.
	// A custom authorizer receives a copy of arguments, not the executable buffer.
	if a.authorizer != nil {
		principal, _ := tool.PrincipalFromContext(ctx)
		for _, call := range calls {
			check := call
			check.Arguments = append([]byte(nil), call.Arguments...)
			err := a.authorizer.Authorize(ctx, check)
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			kind := EventToolAuthorized
			decisionStatus := "ok"
			if err != nil {
				kind = EventToolDenied
				decisionStatus = status(err)
				if errors.Is(err, tool.ErrForbidden) {
					decisionStatus = "denied"
				}
			}
			a.emit(ctx, Event{Type: kind, Step: step, ToolName: call.Name, ToolCallID: call.ID, PrincipalID: principal.ID, Status: decisionStatus})
			if err != nil {
				return nil, fmt.Errorf("agent: authorization failed: %w", err)
			}
		}
	}
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return nil, err
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
			return nil, err
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
		results = append(results, model.Message{Role: "tool", ToolCallID: call.ID, Content: result})
	}
	return results, nil
}
