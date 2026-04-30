package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ZAIQuotaService fetches quota and usage info from the Z.AI / Zhipu monitor API.
//
// Endpoints (no "Bearer" prefix on Authorization header):
//   - GET /api/monitor/usage/quota/limit
//   - GET /api/monitor/usage/model-usage?startTime=&endTime=
//   - GET /api/monitor/usage/tool-usage?startTime=&endTime=
//
// Reference: https://github.com/guyinwonder168/opencode-glm-quota
type ZAIQuotaService struct {
	httpClient *http.Client
}

// NewZAIQuotaService creates a new ZAIQuotaService.
func NewZAIQuotaService() *ZAIQuotaService {
	return &ZAIQuotaService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ZAIQuotaLimit is one entry in the quota/limit response.
// `Type` is "TOKENS_LIMIT" (5h rolling token bucket) or "TIME_LIMIT" (monthly MCP).
type ZAIQuotaLimit struct {
	Type          string                   `json:"type"`
	Unit          *int                     `json:"unit,omitempty"`
	Number        *int                     `json:"number,omitempty"`
	Percentage    float64                  `json:"percentage"`
	NextResetTime int64                    `json:"next_reset_time,omitempty"`
	Usage         *int                     `json:"usage,omitempty"`         // total entitlement (TIME_LIMIT)
	CurrentValue  *int                     `json:"current_value,omitempty"` // used (TIME_LIMIT)
	Remaining     *int                     `json:"remaining,omitempty"`
	UsageDetails  []map[string]interface{} `json:"usage_details,omitempty"`
}

// ZAIQuotaInfo aggregates the three monitor endpoints into a single response.
type ZAIQuotaInfo struct {
	Platform string          `json:"platform"`
	Level    string          `json:"level,omitempty"`
	Limits   []ZAIQuotaLimit `json:"limits,omitempty"`

	// 24h rolling totals from /model-usage
	TotalTokensUsage    *int64 `json:"total_tokens_usage,omitempty"`
	TotalModelCallCount *int64 `json:"total_model_call_count,omitempty"`

	// 24h rolling totals from /tool-usage
	TotalNetworkSearchCount *int64 `json:"total_network_search_count,omitempty"`
	TotalWebReadMcpCount    *int64 `json:"total_web_read_mcp_count,omitempty"`
	TotalZreadMcpCount      *int64 `json:"total_zread_mcp_count,omitempty"`

	// QueryWindowStart/End describe the [start,end] query window for the 24h endpoints.
	QueryWindowStart string `json:"query_window_start,omitempty"`
	QueryWindowEnd   string `json:"query_window_end,omitempty"`

	// Errors maps endpoint name to error string for partial failures (non-fatal).
	Errors map[string]string `json:"errors,omitempty"`
}

// IsZAIAccount reports whether an account targets z.ai/Zhipu via base_url.
func IsZAIAccount(account *Account) bool {
	if account == nil {
		return false
	}
	host := strings.ToLower(account.GetBaseURL())
	return strings.Contains(host, "z.ai") ||
		strings.Contains(host, "bigmodel.cn")
}

// pickEndpoints returns the monitor base for the given account.
func pickZAIBase(account *Account) string {
	host := strings.ToLower(account.GetBaseURL())
	if strings.Contains(host, "bigmodel.cn") {
		if strings.Contains(host, "dev.bigmodel.cn") {
			return "https://dev.bigmodel.cn"
		}
		return "https://open.bigmodel.cn"
	}
	return "https://api.z.ai"
}

// FetchQuota queries the three monitor endpoints in parallel and aggregates the result.
// Partial failures are recorded under Errors but do not abort the whole call.
func (s *ZAIQuotaService) FetchQuota(ctx context.Context, account *Account) (*ZAIQuotaInfo, error) {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, fmt.Errorf("zai: account %d has no api_key", account.ID)
	}
	if !IsZAIAccount(account) {
		return nil, fmt.Errorf("zai: account %d is not a z.ai/zhipu account (base_url=%q)", account.ID, account.GetBaseURL())
	}

	base := pickZAIBase(account)
	out := &ZAIQuotaInfo{
		Platform: base,
		Errors:   make(map[string]string),
	}

	// 24h rolling window: yesterday at current hour → now (current hour end).
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 59, 59, 0, now.Location())
	start := end.Add(-24*time.Hour + time.Second)
	const layout = "2006-01-02 15:04:05"
	out.QueryWindowStart = start.Format(layout)
	out.QueryWindowEnd = end.Format(layout)
	q := url.Values{}
	q.Set("startTime", start.Format(layout))
	q.Set("endTime", end.Format(layout))
	rangeParams := q.Encode()

	type res struct {
		key  string
		data map[string]interface{}
		err  error
	}
	ch := make(chan res, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		d, err := s.get(ctx, base+"/api/monitor/usage/quota/limit", apiKey)
		ch <- res{"quota_limit", d, err}
	}()
	go func() {
		defer wg.Done()
		d, err := s.get(ctx, base+"/api/monitor/usage/model-usage?"+rangeParams, apiKey)
		ch <- res{"model_usage", d, err}
	}()
	go func() {
		defer wg.Done()
		d, err := s.get(ctx, base+"/api/monitor/usage/tool-usage?"+rangeParams, apiKey)
		ch <- res{"tool_usage", d, err}
	}()
	wg.Wait()
	close(ch)

	for r := range ch {
		if r.err != nil {
			out.Errors[r.key] = r.err.Error()
			continue
		}
		switch r.key {
		case "quota_limit":
			applyZAIQuotaLimit(out, r.data)
		case "model_usage":
			applyZAIModelUsage(out, r.data)
		case "tool_usage":
			applyZAIToolUsage(out, r.data)
		}
	}

	if len(out.Errors) == 3 {
		// All three failed - return the first error.
		for _, msg := range out.Errors {
			return nil, fmt.Errorf("zai: all monitor endpoints failed: %s", msg)
		}
	}
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out, nil
}

// get performs an authenticated GET against a z.ai monitor endpoint and returns the
// parsed `data` field of the standard envelope `{code, msg, data, success}`.
func (s *ZAIQuotaService) get(ctx context.Context, endpoint, apiKey string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// IMPORTANT: z.ai expects the raw token, no "Bearer " prefix.
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}
	var env struct {
		Code    int                    `json:"code"`
		Msg     string                 `json:"msg"`
		Data    map[string]interface{} `json:"data"`
		Success bool                   `json:"success"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if env.Code != 200 && env.Code != 0 {
		return nil, fmt.Errorf("api code %d: %s", env.Code, env.Msg)
	}
	return env.Data, nil
}

func applyZAIQuotaLimit(out *ZAIQuotaInfo, data map[string]interface{}) {
	if data == nil {
		return
	}
	if lvl, ok := data["level"].(string); ok {
		out.Level = lvl
	}
	rawLimits, _ := data["limits"].([]interface{})
	for _, item := range rawLimits {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		l := ZAIQuotaLimit{}
		if v, ok := m["type"].(string); ok {
			l.Type = v
		}
		if v, ok := m["percentage"].(float64); ok {
			l.Percentage = v
		}
		if v, ok := m["nextResetTime"].(float64); ok {
			l.NextResetTime = int64(v)
		}
		if v, ok := m["unit"].(float64); ok {
			n := int(v)
			l.Unit = &n
		}
		if v, ok := m["number"].(float64); ok {
			n := int(v)
			l.Number = &n
		}
		if v, ok := m["usage"].(float64); ok {
			n := int(v)
			l.Usage = &n
		}
		if v, ok := m["currentValue"].(float64); ok {
			n := int(v)
			l.CurrentValue = &n
		}
		if v, ok := m["remaining"].(float64); ok {
			n := int(v)
			l.Remaining = &n
		}
		if details, ok := m["usageDetails"].([]interface{}); ok {
			for _, d := range details {
				if dm, ok := d.(map[string]interface{}); ok {
					l.UsageDetails = append(l.UsageDetails, dm)
				}
			}
		}
		out.Limits = append(out.Limits, l)
	}
}

func applyZAIModelUsage(out *ZAIQuotaInfo, data map[string]interface{}) {
	if data == nil {
		return
	}
	tu, _ := data["totalUsage"].(map[string]interface{})
	if tu == nil {
		return
	}
	if v, ok := tu["totalTokensUsage"].(float64); ok {
		n := int64(v)
		out.TotalTokensUsage = &n
	}
	if v, ok := tu["totalModelCallCount"].(float64); ok {
		n := int64(v)
		out.TotalModelCallCount = &n
	}
}

func applyZAIToolUsage(out *ZAIQuotaInfo, data map[string]interface{}) {
	if data == nil {
		return
	}
	tu, _ := data["totalUsage"].(map[string]interface{})
	if tu == nil {
		return
	}
	if v, ok := tu["totalNetworkSearchCount"].(float64); ok {
		n := int64(v)
		out.TotalNetworkSearchCount = &n
	}
	if v, ok := tu["totalWebReadMcpCount"].(float64); ok {
		n := int64(v)
		out.TotalWebReadMcpCount = &n
	}
	if v, ok := tu["totalZreadMcpCount"].(float64); ok {
		n := int64(v)
		out.TotalZreadMcpCount = &n
	}
}
