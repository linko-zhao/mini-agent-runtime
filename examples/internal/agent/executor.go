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
	return a.executeBatchWithState(ctx, calls, step, nil)
}
func (a *Agent) executeBatchWithState(ctx context.Context, calls []model.ToolCall, step int, record *runRecord) ([]model.Message, error) {
	base := 0
	if record != nil {
		var err error
		base, err = record.plan(calls, step)
		if err != nil {
			return nil, err
		}
	}
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
	if err := a.authorizeBatch(ctx, calls, step); err != nil {
		return nil, err
	}
	for index, call := range calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if record != nil {
			if err := record.startCall(base + index); err != nil {
				return nil, err
			}
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
			if record != nil {
				if stateErr := record.completeCall(base+index, nil, err); stateErr != nil {
					return nil, errors.Join(err, stateErr)
				}
			}
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
		message := model.Message{Role: "tool", ToolCallID: call.ID, Content: result}
		if record != nil {
			if stateErr := record.completeCall(base+index, &message, err); stateErr != nil {
				return nil, stateErr
			}
		}
		results = append(results, message)
	}
	return results, nil
}
