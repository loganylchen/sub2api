//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAccount_IsMixedSchedulingEnabled_Copilot ensures Copilot accounts can opt
// into mixed scheduling via extra.mixed_scheduling=true.
func TestAccount_IsMixedSchedulingEnabled_Copilot(t *testing.T) {
	t.Run("copilot enabled", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformCopilot,
			Extra:    map[string]any{"mixed_scheduling": true},
		}
		require.True(t, acc.IsMixedSchedulingEnabled())
	})

	t.Run("copilot not enabled returns false", func(t *testing.T) {
		acc := &Account{Platform: PlatformCopilot}
		require.False(t, acc.IsMixedSchedulingEnabled())
	})

	t.Run("copilot extra without flag returns false", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformCopilot,
			Extra:    map[string]any{"other": true},
		}
		require.False(t, acc.IsMixedSchedulingEnabled())
	})

	t.Run("non-copilot non-antigravity returns false even when flag set", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"mixed_scheduling": true},
		}
		require.False(t, acc.IsMixedSchedulingEnabled())
	})
}

// TestGatewayService_isModelSupportedByAccount_Copilot exercises the
// dash↔dot/date-suffix-tolerant matching that allows Copilot accounts to be
// scheduled when the request uses Anthropic-style model IDs.
func TestGatewayService_isModelSupportedByAccount_Copilot(t *testing.T) {
	svc := &GatewayService{}

	t.Run("empty model is allowed", func(t *testing.T) {
		acc := &Account{Platform: PlatformCopilot}
		require.True(t, svc.isModelSupportedByAccount(acc, ""))
	})

	t.Run("dash model with date suffix matches dot whitelist", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformCopilot,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-sonnet-4.5": "claude-sonnet-4.5",
				},
			},
		}
		require.True(t, svc.isModelSupportedByAccount(acc, "claude-sonnet-4-5-20250929"))
	})

	t.Run("unmapped model with no whitelist allows all", func(t *testing.T) {
		acc := &Account{Platform: PlatformCopilot}
		require.True(t, svc.isModelSupportedByAccount(acc, "claude-sonnet-4-5-20250929"))
	})

	t.Run("model not in whitelist returns false", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformCopilot,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-haiku-4.5": "claude-haiku-4.5",
				},
			},
		}
		require.False(t, svc.isModelSupportedByAccount(acc, "claude-opus-4-5"))
	})
}

// TestGatewayService_isAccountAllowedForPlatform_Copilot verifies Copilot
// accounts can only mix into Anthropic groups, never Gemini.
func TestGatewayService_isAccountAllowedForPlatform_Copilot(t *testing.T) {
	svc := &GatewayService{}

	enabled := &Account{
		Platform: PlatformCopilot,
		Extra:    map[string]any{"mixed_scheduling": true},
	}
	disabled := &Account{Platform: PlatformCopilot}

	require.True(t, svc.isAccountAllowedForPlatform(enabled, PlatformAnthropic, true))
	require.False(t, svc.isAccountAllowedForPlatform(disabled, PlatformAnthropic, true))
	require.False(t, svc.isAccountAllowedForPlatform(enabled, PlatformGemini, true),
		"copilot must never enter gemini groups")
	require.False(t, svc.isAccountAllowedForPlatform(enabled, PlatformAnthropic, false),
		"without useMixed, only same-platform accounts are allowed")
}

// TestGatewayService_listSchedulableAccounts_IncludesCopilot ensures the
// platform list passed to the repository includes Copilot when the native
// platform is Anthropic and mixed scheduling is in effect.
func TestGatewayService_listSchedulableAccounts_IncludesCopilot(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 5, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformAntigravity, Priority: 5, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true}},
			{ID: 3, Platform: PlatformCopilot, Priority: 5, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true}},
			{ID: 4, Platform: PlatformCopilot, Priority: 5, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	svc := &GatewayService{
		accountRepo: repo,
		cfg:         testConfig(),
	}

	accounts, useMixed, err := svc.listSchedulableAccounts(ctx, nil, PlatformAnthropic, false)
	require.NoError(t, err)
	require.True(t, useMixed)

	ids := map[int64]bool{}
	for _, a := range accounts {
		ids[a.ID] = true
	}
	require.True(t, ids[1], "anthropic account should be present")
	require.True(t, ids[2], "antigravity with mixed_scheduling should be present")
	require.True(t, ids[3], "copilot with mixed_scheduling should be present")
	require.False(t, ids[4], "copilot without mixed_scheduling should be filtered out")
}

