package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// adaptCopilotForwardResult converts a CopilotForwardResult into the shared
// ForwardResult shape used by the main /v1/messages handler so usage logging,
// failover tracking, and channel/billing accounting all see the same fields
// regardless of upstream platform.
//
// Copilot does not return Anthropic-style request IDs and does not expose
// detailed timing fields, so RequestID is left empty and Duration is supplied
// by the caller (measured at the dispatch site).
func adaptCopilotForwardResult(r *service.CopilotForwardResult, stream bool, dur time.Duration) *service.ForwardResult {
	if r == nil {
		return nil
	}
	out := &service.ForwardResult{
		Model:         r.Model,
		UpstreamModel: r.Model,
		Stream:        stream,
		Duration:      dur,
	}
	if r.Usage != nil {
		out.Usage = service.ClaudeUsage{
			InputTokens:  r.Usage.PromptTokens,
			OutputTokens: r.Usage.CompletionTokens,
		}
	}
	return out
}
