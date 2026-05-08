package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupPreviewModelsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/preview-models", handler.PreviewAvailableModels)
	return router
}

func postPreviewModels(t *testing.T, router *gin.Engine, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(buf).Encode(body))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/preview-models", buf)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestPreviewAvailableModels_LiveFetchSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		require.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"custom-model-1","type":"model","display_name":"Custom 1"},{"id":"custom-model-2","type":"model","display_name":"Custom 2"}]}`))
	}))
	defer upstream.Close()

	router := setupPreviewModelsRouter()
	// Use the httptest server URL (not a known preset domain) so the live
	// fetch path runs.
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": upstream.URL,
		"api_key":  "test-api-key",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data, 2)
	require.Equal(t, "custom-model-1", resp.Data[0].ID)
}

func TestPreviewAvailableModels_KnownPresetSkipsLive(t *testing.T) {
	// For a known third-party (z.ai), the handler must return the preset
	// without calling the upstream — even with bogus credentials this should
	// succeed and return GLM models.
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": "https://api.z.ai/api/anthropic",
		"api_key":  "any-key-the-preset-path-doesnt-call-upstream",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.NotEmpty(t, resp.Data)
	// All models should be GLM, none Claude.
	for _, m := range resp.Data {
		require.NotContains(t, m.ID, "claude", "preset returned a Claude id: %s", m.ID)
	}
	// Verify glm-4.6 is in the list
	found := false
	for _, m := range resp.Data {
		if m.ID == "glm-4.6" {
			found = true
			break
		}
	}
	require.True(t, found, "expected glm-4.6 in preset response")
}

func TestPreviewAvailableModels_BigmodelPreset(t *testing.T) {
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": "https://open.bigmodel.cn/api/anthropic",
		"api_key":  "any",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)
}

func TestPreviewAvailableModels_MissingAPIKey(t *testing.T) {
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": "https://api.z.ai/api/anthropic",
		"api_key":  "",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "api_key")
}

func TestPreviewAvailableModels_MissingBaseURL(t *testing.T) {
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": "",
		"api_key":  "test-key",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "base_url")
}

func TestPreviewAvailableModels_InvalidBaseURL(t *testing.T) {
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": "not-a-url",
		"api_key":  "test-key",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPreviewAvailableModels_RejectsNonAnthropicPlatform(t *testing.T) {
	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "openai",
		"base_url": "https://api.openai.com",
		"api_key":  "sk-test",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, strings.ToLower(rec.Body.String()), "anthropic")
}

func TestPreviewAvailableModels_Upstream401(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": upstream.URL,
		"api_key":  "bad-key",
	})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPreviewAvailableModels_Upstream500ReturnsBadGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	router := setupPreviewModelsRouter()
	rec := postPreviewModels(t, router, map[string]string{
		"platform": "anthropic",
		"base_url": upstream.URL,
		"api_key":  "test-key",
	})
	require.Equal(t, http.StatusBadGateway, rec.Code)
}
