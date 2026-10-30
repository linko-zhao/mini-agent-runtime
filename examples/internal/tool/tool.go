package tool

import (
	"context"
	"encoding/json"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type Tool interface {
	Name() string
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

type DescribedTool interface {
	Tool
	Definition() model.ToolDefinition
}
