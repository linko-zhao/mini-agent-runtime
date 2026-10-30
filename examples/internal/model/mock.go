package model

import (
	"context"
	"fmt"
)

// Mock 是单个 Run 的脚本模型；并发 Run 应分别创建 Mock。
type Mock struct {
	responses []Response
	i         int
}

func NewMock(responses []Response) *Mock { return &Mock{responses: responses} }
func (m *Mock) Generate(ctx context.Context, _ []Message) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.i >= len(m.responses) {
		return nil, fmt.Errorf("mock: exhausted responses")
	}
	r := m.responses[m.i]
	m.i++
	return &r, nil
}
