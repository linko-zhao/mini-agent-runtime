package agent

import (
	"context"
	"testing"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentRun(t *testing.T) {
	t.Run("模型直接返回文本", func(t *testing.T) {
		m := model.NewMock([]model.Response{
			{Message: model.Message{
				Role: "assistant",
				Content: "Hi, I'm an AI assistant",
			}},
		})

		a := New(m, nil, 5)
		if r, err := a.Run(context.Background(), "Hi"); err != nil {
			require.NoError(t, err)
		} else {
			assert.Equal(t, "Hi, I'm an AI assistant", r, "Should return 'Hi, I'm an AI assistant'")
		}
	})
}
