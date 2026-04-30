package claude

import "testing"

func TestLookupAnthropicCompatPreset(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		baseURL     string
		wantNil     bool
		wantName    string
		wantHasGLM4 bool
	}{
		{name: "empty", baseURL: "", wantNil: true},
		{name: "official anthropic", baseURL: "https://api.anthropic.com", wantNil: true},
		{name: "unknown third-party", baseURL: "https://api.deepseek.com", wantNil: true},
		{
			name: "z.ai international",
			baseURL: "https://api.z.ai/api/anthropic",
			wantName: "Z.AI", wantHasGLM4: true,
		},
		{
			name: "z.ai with trailing slash",
			baseURL: "https://api.z.ai/api/anthropic/",
			wantName: "Z.AI", wantHasGLM4: true,
		},
		{
			name: "bigmodel China",
			baseURL: "https://open.bigmodel.cn/api/anthropic",
			wantName: "Zhipu (BigModel)", wantHasGLM4: true,
		},
		{
			name: "case insensitive",
			baseURL: "HTTPS://OPEN.BIGMODEL.CN/api/anthropic",
			wantName: "Zhipu (BigModel)", wantHasGLM4: true,
		},
		{
			name: "moonshot",
			baseURL: "https://api.moonshot.cn/anthropic",
			wantName: "Moonshot (Kimi)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LookupAnthropicCompatPreset(tc.baseURL)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected preset for %q, got nil", tc.baseURL)
			}
			if got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantName)
			}
			if len(got.Models) == 0 {
				t.Error("expected non-empty Models")
			}
			if tc.wantHasGLM4 {
				found := false
				for _, m := range got.Models {
					if m.ID == "glm-4.6" {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected glm-4.6 in models, not found")
				}
			}
		})
	}
}
