package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

// Spec 只包含供模型选择能力的元信息，不含执行函数或凭据。
type Spec struct {
	Name, Description string
	InputSchema       json.RawMessage
}
type Described interface {
	Meta(context.Context) (Spec, error)
}
type Invokable interface {
	Described
	Invoke(context.Context, json.RawMessage) (string, error)
}

// Call 沿用最早建立的 ToolCall 数据结构，避免第二套调用协议。
type Call = model.ToolCall

func cloneSpec(s Spec) Spec { s.InputSchema = append(json.RawMessage(nil), s.InputSchema...); return s }

// Adapt 将 Day 08 能力接入 Day 01 的 Tool/DescribedTool，元信息在适配时快照。
// Invoke 仍接收每次执行的 context，而非构造时的 context。
func Adapt(ctx context.Context, t Invokable) (DescribedTool, error) {
	if t == nil {
		return nil, fmt.Errorf("tool: nil invokable")
	}
	s, err := t.Meta(ctx)
	if err != nil {
		return nil, err
	}
	if err = validateSpec(s); err != nil {
		return nil, err
	}
	return &legacyAdapter{spec: cloneSpec(s), target: t}, nil
}

type legacyAdapter struct {
	spec   Spec
	target Invokable
}

func (t *legacyAdapter) Name() string { return t.spec.Name }
func (t *legacyAdapter) Definition() model.ToolDefinition {
	return model.ToolDefinition{Name: t.spec.Name, Description: t.spec.Description, Parameters: append(json.RawMessage(nil), t.spec.InputSchema...)}
}
func (t *legacyAdapter) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.target.Invoke(ctx, args)
}
