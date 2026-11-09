package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

// authorizeBatch runs after protocol validation, before the first tool executes.
func (a *Agent) authorizeBatch(ctx context.Context, calls []model.ToolCall, step int) error {
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
				return fmt.Errorf("agent: authorization failed: %w", err)
			}
		}
	}
	return nil
}
