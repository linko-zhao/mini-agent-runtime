package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type Spec struct {
	Name, Description string
	InputSchema json.RawMessage
}

type Described interface {
	Meta(context.Context) (Spec, error)
}

type Invokable interface {
	Described
	Invoke(context.Context, json.RawMessage) (string, error)
}

type Call = model.ToolCall

func cloneSpec(s Spec) Spec { 
	s.InputSchema = append(json.RawMessage(nil), s.InputSchema...)
	return s 
}

func Adapt(ctx context.Context, t Invokable) (DescribedTool, error) {
	if t == nil {
		return nil, fmt.Errorf("tool: nil Invokable")
	}
	s, err := t.Meta(ctx)
	if err != nil {
		return nil, err
	}
	
	if err := validateSpec(s); err != nil {
		return nil, err
	}

	return &legacyAdapter{spec: cloneSpec(s), target: t}, nil
}

type legacyAdapter struct {
	spec Spec
	target Invokable
}

func (t *legacyAdapter) Name() string { return t.spec.Name }
func (t *legacyAdapter) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name: t.spec.Name,
		Description: t.spec.Description,
		Parameters: append(json.RawMessage(nil), t.spec.InputSchema...),
	}
}
func (t *legacyAdapter) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.target.Invoke(ctx, args)
}
