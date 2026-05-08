package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/copilot"
)

// getZAIUsage fetches Z.AI / GLM Coding Plan quota and synthesizes a UsageInfo
// suitable for the admin account list cell. Maps:
//   - TOKENS_LIMIT (5h rolling) → FiveHour
//   - WEEKLY_TOKENS_LIMIT or WEEK_TOKENS_LIMIT (if present) → SevenDay
func (s *AccountUsageService) getZAIUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	if s.zaiQuotaService == nil {
		return &UsageInfo{Error: "zai quota service not configured", ErrorCode: "network_error"}, nil
	}
	now := time.Now()
	quota, err := s.zaiQuotaService.FetchQuota(ctx, account)
	if err != nil {
		return &UsageInfo{
			Source:    "active",
			UpdatedAt: &now,
			Error:     err.Error(),
			ErrorCode: "network_error",
		}, nil
	}

	usage := &UsageInfo{
		Source:    "active",
		UpdatedAt: &now,
	}
	for _, limit := range quota.Limits {
		bar := zaiLimitToProgress(limit, now)
		switch limit.Type {
		case "TOKENS_LIMIT":
			usage.FiveHour = bar
		case "WEEKLY_TOKENS_LIMIT", "WEEK_TOKENS_LIMIT":
			usage.SevenDay = bar
		}
	}
	if quota != nil && len(quota.Errors) > 0 {
		// Surface partial errors as an info hint (non-fatal).
		first := ""
		for _, msg := range quota.Errors {
			first = msg
			break
		}
		usage.Error = fmt.Sprintf("partial quota: %s", first)
	}
	return usage, nil
}

// zaiLimitToProgress converts a single Z.AI quota limit entry into a UsageProgress
// bar (utilization %, resets_at, used/total tokens) for rendering as 5h/7d bar.
func zaiLimitToProgress(limit ZAIQuotaLimit, now time.Time) *UsageProgress {
	p := &UsageProgress{
		Utilization: limit.Percentage,
	}
	if limit.NextResetTime > 0 {
		ms := limit.NextResetTime
		if ms < 1e12 { // seconds
			ms *= 1000
		}
		t := time.UnixMilli(ms)
		p.ResetsAt = &t
		if remaining := int(t.Sub(now).Seconds()); remaining > 0 {
			p.RemainingSeconds = remaining
		}
	}
	// Build window stats so the cell can display absolute tokens.
	used := int64(0)
	total := int64(0)
	if limit.CurrentValue != nil {
		used = int64(*limit.CurrentValue)
	} else if limit.Usage != nil && limit.Number != nil {
		// fallback: usage = total entitlement, derive used from percentage
		used = int64(float64(*limit.Number) * (limit.Percentage / 100.0))
	}
	if limit.Number != nil {
		total = int64(*limit.Number)
	} else if limit.Unit != nil {
		total = int64(*limit.Unit)
	}
	if total > 0 || used > 0 {
		p.WindowStats = &WindowStats{Tokens: used}
		// Use UsedRequests slot (already JSON-tagged) only when total is meaningful.
		// We rely on percentage for the bar; tokens shown via WindowStats.Tokens.
		_ = total
	}
	return p
}

// getCopilotUsage fetches Copilot quota and synthesizes a UsageInfo with a
// CopilotQuotaSnapshot. Copilot has no 5h/7d windows; the cell displays a single
// "premium interactions used / entitlement" total bar instead.
func (s *AccountUsageService) getCopilotUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	if s.copilotGatewayService == nil {
		return &UsageInfo{Error: "copilot service not configured", ErrorCode: "network_error"}, nil
	}
	now := time.Now()
	info, err := s.copilotGatewayService.FetchQuota(ctx, account)
	if err != nil {
		return &UsageInfo{
			Source:    "active",
			UpdatedAt: &now,
			Error:     err.Error(),
			ErrorCode: "network_error",
		}, nil
	}
	snap := copilotQuotaToSnapshot(info)
	return &UsageInfo{
		Source:       "active",
		UpdatedAt:    &now,
		CopilotQuota: snap,
	}, nil
}

func copilotQuotaToSnapshot(info *copilot.CopilotQuotaInfo) *CopilotQuotaSnapshot {
	if info == nil {
		return nil
	}
	snap := &CopilotQuotaSnapshot{
		Plan:           info.Plan,
		PlanType:       info.PlanType,
		QuotaResetDate: info.QuotaResetDate,
	}
	if pi := info.PremiumInteractions; pi != nil {
		snap.PremiumUsed = int64(pi.Used)
		snap.PremiumLimit = int64(pi.Entitlement)
		snap.PremiumOveragePermitted = pi.OveragePermitted
		if pi.Entitlement > 0 {
			snap.PremiumPercentage = float64(pi.Used) / float64(pi.Entitlement) * 100.0
		}
	}
	return snap
}
