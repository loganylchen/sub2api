// Package claude: known Anthropic-compatible third-party providers.
//
// Many providers (Zhipu/GLM via z.ai or open.bigmodel.cn, Moonshot/Kimi, etc.)
// expose an Anthropic-compatible /v1/messages endpoint without implementing a
// reliable /v1/models listing. The presets below let the admin UI show a
// useful model catalog for these providers without depending on /v1/models.
package claude

import "strings"

// AnthropicCompatPreset describes the model catalog of a known third-party
// Anthropic-compatible provider, matched by base_url substring.
type AnthropicCompatPreset struct {
	// Name is a human-readable provider name (e.g. "Zhipu (BigModel)").
	Name string
	// Models is the list of model IDs the provider accepts on its
	// /v1/messages endpoint.
	Models []Model
}

// anthropicCompatPresets maps a host substring (case-insensitive) to its
// preset. base_url is matched by checking if it contains the substring.
// Order matters only for documentation — the first match wins.
var anthropicCompatPresets = []struct {
	HostSubstring string
	Preset        AnthropicCompatPreset
}{
	{
		// Zhipu (China region)
		HostSubstring: "open.bigmodel.cn",
		Preset: AnthropicCompatPreset{
			Name:   "Zhipu (BigModel)",
			Models: glmModels,
		},
	},
	{
		// Z.AI (international Zhipu)
		HostSubstring: "api.z.ai",
		Preset: AnthropicCompatPreset{
			Name:   "Z.AI",
			Models: glmModels,
		},
	},
	{
		// Moonshot / Kimi (anthropic-compat path is documented at
		// https://api.moonshot.cn/anthropic — only enabled when users
		// explicitly point at this path).
		HostSubstring: "api.moonshot.cn",
		Preset: AnthropicCompatPreset{
			Name:   "Moonshot (Kimi)",
			Models: kimiModels,
		},
	},
}

// glmModels lists the GLM models accepted by Zhipu's Anthropic-compat layer
// (both api.z.ai/api/anthropic and open.bigmodel.cn/api/anthropic accept the
// same set of GLM IDs).
var glmModels = []Model{
	{ID: "glm-5.1", Type: "model", DisplayName: "GLM-5.1"},
	{ID: "glm-4.7", Type: "model", DisplayName: "GLM-4.7"},
	{ID: "glm-4.6", Type: "model", DisplayName: "GLM-4.6"},
	{ID: "glm-4.5", Type: "model", DisplayName: "GLM-4.5"},
	{ID: "glm-4.5-air", Type: "model", DisplayName: "GLM-4.5-Air"},
	{ID: "glm-4.5-flash", Type: "model", DisplayName: "GLM-4.5-Flash"},
}

// kimiModels lists Moonshot/Kimi models accepted on the anthropic-compat path.
var kimiModels = []Model{
	{ID: "kimi-k2-0905-preview", Type: "model", DisplayName: "Kimi K2 0905"},
	{ID: "kimi-k2-0711-preview", Type: "model", DisplayName: "Kimi K2 0711"},
	{ID: "kimi-latest", Type: "model", DisplayName: "Kimi Latest"},
	{ID: "moonshot-v1-128k", Type: "model", DisplayName: "Moonshot v1 128k"},
	{ID: "moonshot-v1-32k", Type: "model", DisplayName: "Moonshot v1 32k"},
	{ID: "moonshot-v1-8k", Type: "model", DisplayName: "Moonshot v1 8k"},
}

// LookupAnthropicCompatPreset returns the preset for a known Anthropic-compat
// third-party provider matched by base_url substring, or nil if no match.
//
// Matching is case-insensitive and substring-based, so it works for both
// "https://api.z.ai/api/anthropic" and "https://api.z.ai/api/anthropic/" etc.
func LookupAnthropicCompatPreset(baseURL string) *AnthropicCompatPreset {
	if baseURL == "" {
		return nil
	}
	lower := strings.ToLower(baseURL)
	for _, entry := range anthropicCompatPresets {
		if strings.Contains(lower, entry.HostSubstring) {
			p := entry.Preset
			return &p
		}
	}
	return nil
}
