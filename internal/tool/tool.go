package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Name() string
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}
