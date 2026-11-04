package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
	"github.com/stretchr/testify/require"
)

type authorizationCounter struct {
	name  string
	calls int
	seen  string
}

func (c *authorizationCounter) Name() string { return c.name }
func (c *authorizationCounter) Execute(_ context.Context, args json.RawMessage) (string, error) {
	c.calls++
	c.seen = string(args)
	return "ok", nil
}

type authorizationModel struct {
	first   []model.ToolCall
	calls   int
	history []model.Message
}

func (m *authorizationModel) Generate(_ context.Context, messages []model.Message) (*model.Response, error) {
	m.calls++
	m.history = append([]model.Message(nil), messages...)
	if m.calls == 1 {
		return &model.Response{Message: model.Message{Role: "assistant", ToolCalls: m.first}}, nil
	}
	return &model.Response{Message: model.Message{Role: "assistant", Content: "done"}}, nil
}

type authorizationFunc func(context.Context, tool.Call) error

func (f authorizationFunc) Authorize(ctx context.Context, call tool.Call) error { return f(ctx, call) }

func authorizationPolicy(t *testing.T) *tool.CapabilityAuthorizer {
	t.Helper()
	a, err := tool.NewCapabilityAuthorizer(
		map[string]tool.Capability{"read": tool.ReadCapability, "write": tool.WriteCapability},
		map[string][]tool.Capability{"reader": {tool.ReadCapability}},
	)
	require.NoError(t, err)
	return a
}

func TestAuthorizationDeniesWholeBatchBeforeExecution(t *testing.T) {
	read, write := &authorizationCounter{name: "read"}, &authorizationCounter{name: "write"}
	m := &authorizationModel{first: []model.ToolCall{{ID: "r", Name: "read"}, {ID: "w", Name: "write"}}}
	events := &eventCollector{}
	a, err := New(m, map[string]tool.Tool{"read": read, "write": write}, 2, WithVisibleTools(), WithAuthorizer(authorizationPolicy(t)), WithEventSink(events))
	require.NoError(t, err)
	ctx := tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"})
	_, err = a.Run(ctx, "model can request hidden tools")
	require.ErrorIs(t, err, tool.ErrForbidden)
	require.Zero(t, read.calls)
	require.Zero(t, write.calls)
	require.Equal(t, 1, m.calls)
	all := events.snapshot()
	var denied, finished, failed, started int
	for _, e := range all {
		switch e.Type {
		case ToolDenied:
			denied++
			require.Equal(t, "w", e.ToolCallID)
			require.Equal(t, "reader", e.PrincipalID)
			require.Equal(t, EventDenied, e.Status)
		case RunFinished:
			finished++
		case RunFailed:
			failed++
		case ToolStarted:
			started++
		}
	}
	require.Equal(t, 1, denied)
	require.Zero(t, finished)
	require.Equal(t, 1, failed)
	require.Zero(t, started)
}

func TestAuthorizationVisibilityIsNotPermission(t *testing.T) {
	read := &authorizationCounter{name: "read"}
	m := &authorizationModel{first: []model.ToolCall{{ID: "r", Name: "read", Arguments: json.RawMessage("{}")}}}
	a, err := New(m, map[string]tool.Tool{"read": read}, 2, WithVisibleTools(), WithAuthorizer(authorizationPolicy(t)))
	require.NoError(t, err)
	out, err := a.Run(tool.WithPrincipal(context.Background(), tool.Principal{ID: "reader"}), "read")
	require.NoError(t, err)
	require.Equal(t, "done", out)
	require.Equal(t, 1, read.calls)
	require.Len(t, m.history, 3)
	require.Equal(t, "tool", m.history[2].Role)
	require.Equal(t, "r", m.history[2].ToolCallID)
}

func TestAuthorizationFailureAndArgumentIsolation(t *testing.T) {
	cause := errors.New("private policy backend failure")
	for _, missingPrincipal := range []bool{false, true} {
		target := &authorizationCounter{name: "read"}
		m := &authorizationModel{first: []model.ToolCall{{ID: "r", Name: "read"}}}
		var auth tool.Authorizer = authorizationFunc(func(context.Context, tool.Call) error { return cause })
		if missingPrincipal {
			auth = authorizationPolicy(t)
		}
		a, err := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(auth))
		require.NoError(t, err)
		_, err = a.Run(context.Background(), "")
		require.Error(t, err)
		require.Zero(t, target.calls)
		require.Equal(t, 1, m.calls)
		if missingPrincipal {
			require.ErrorIs(t, err, tool.ErrForbidden)
		} else {
			require.ErrorIs(t, err, cause)
		}
	}
	target := &authorizationCounter{name: "read"}
	m := &authorizationModel{first: []model.ToolCall{{ID: "r", Name: "read", Arguments: json.RawMessage("{}")}}}
	a, err := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(authorizationFunc(func(_ context.Context, call tool.Call) error {
		call.Arguments[0] = 'x'
		return nil
	})))
	require.NoError(t, err)
	_, err = a.Run(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "{}", target.seen)
}

func TestAuthorizationCancellationAndNilOption(t *testing.T) {
	_, err := New(&authorizationModel{}, nil, 1, WithAuthorizer(nil))
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := &authorizationCounter{name: "read"}
	m := &authorizationModel{first: []model.ToolCall{{ID: "r", Name: "read"}}}
	authFailure := errors.New("authorization backend failed after cancellation")
	a, err := New(m, map[string]tool.Tool{"read": target}, 2, WithAuthorizer(authorizationFunc(func(context.Context, tool.Call) error {
		cancel()
		return authFailure
	})))
	require.NoError(t, err)
	_, err = a.Run(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, target.calls)
	require.Equal(t, 1, m.calls)
}
