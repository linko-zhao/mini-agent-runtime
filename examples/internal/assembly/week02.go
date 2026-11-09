// Package assembly is a host-side composition root, not a new Runtime protocol.
package assembly

import (
	"context"
	"fmt"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/agent"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool/mcptool"
)

type Config struct {
	Model              model.Model
	Local              tool.Invokable
	Transport          mcp.Transport
	Prefix             string
	Rules              map[string]tool.Capability
	Grants             map[string][]tool.Capability
	Policies           map[string]tool.Policy
	Visible            []string
	MaxPages, MaxSteps int
	ToolTimeout        time.Duration
	Sink               agent.EventSink
}
type Runtime struct {
	Agent  *agent.Agent
	Client *mcp.Client
}

// Build initializes/discovers capabilities but does not invoke business tools.
// The caller owns transport cleanup; a failed/ambiguous session is not replayed.
func Build(ctx context.Context, c Config) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Model == nil || c.Local == nil || c.Transport == nil || c.MaxPages <= 0 || c.MaxSteps <= 0 || c.ToolTimeout < 0 {
		return nil, fmt.Errorf("assembly: invalid config")
	}
	client, err := mcp.NewClient(c.Transport)
	if err != nil {
		return nil, err
	}
	if _, err = client.Initialize(ctx); err != nil {
		return nil, err
	}
	candidates, err := mcptool.Discover(ctx, client, c.Prefix, c.MaxPages)
	if err != nil {
		return nil, err
	}
	registry := tool.NewRegistry()
	if err = registry.Register(ctx, c.Local); err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		if err = registry.Register(ctx, candidate); err != nil {
			return nil, err
		}
	}
	tools := registry.Tools()
	for name, target := range tools {
		described, ok := target.(tool.DescribedTool)
		if !ok {
			return nil, fmt.Errorf("assembly: description required")
		}
		wrapped, err := tool.WithPolicy(described, c.Policies[name])
		if err != nil {
			return nil, err
		}
		tools[name] = wrapped
	}
	auth, err := tool.NewCapabilityAuthorizer(c.Rules, c.Grants)
	if err != nil {
		return nil, err
	}
	opts := []agent.Option{agent.WithAuthorizer(auth), agent.WithToolTimeout(c.ToolTimeout), agent.WithEventSink(c.Sink)}
	if c.Visible != nil {
		opts = append(opts, agent.WithVisibleTools(c.Visible...))
	}
	a, err := agent.New(c.Model, tools, c.MaxSteps, opts...)
	if err != nil {
		return nil, err
	}
	return &Runtime{Agent: a, Client: client}, nil
}
