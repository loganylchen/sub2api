//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAdaptCopilotForwardResult(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		require.Nil(t, adaptCopilotForwardResult(nil, false, time.Second))
	})

	t.Run("usage and stream flags propagate", func(t *testing.T) {
		src := &service.CopilotForwardResult{
			StatusCode: 200,
			Model:      "claude-sonnet-4.5",
			Usage: &service.CopilotUsage{
				PromptTokens:     12,
				CompletionTokens: 34,
				TotalTokens:      46,
			},
		}
		dur := 250 * time.Millisecond
		got := adaptCopilotForwardResult(src, true, dur)
		require.NotNil(t, got)
		require.Equal(t, "claude-sonnet-4.5", got.Model)
		require.Equal(t, "claude-sonnet-4.5", got.UpstreamModel)
		require.True(t, got.Stream)
		require.Equal(t, dur, got.Duration)
		require.Equal(t, 12, got.Usage.InputTokens)
		require.Equal(t, 34, got.Usage.OutputTokens)
	})

	t.Run("nil usage produces zero usage", func(t *testing.T) {
		src := &service.CopilotForwardResult{Model: "claude-haiku-4.5"}
		got := adaptCopilotForwardResult(src, false, 0)
		require.NotNil(t, got)
		require.Equal(t, 0, got.Usage.InputTokens)
		require.Equal(t, 0, got.Usage.OutputTokens)
	})
}
