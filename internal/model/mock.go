package model

import (
	"context"
	"fmt"
)

// Mock 按预设 Response 序列依次返回，无需 API Key。
type Mock struct {
	responses []Response
	i         int
}

func NewMock(responses []Response) *Mock {
	return &Mock{responses: responses}
}

func (m *Mock) Generate(ctx context.Context, messages []Message) (*Response, error) {
	if m.i >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more preset responses")
	}
	r := m.responses[m.i]
	m.i++
	return &r, nil
}
