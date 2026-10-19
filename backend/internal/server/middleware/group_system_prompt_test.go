package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupSystemPromptGatewayBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, prefix := range []string{"/v1", "", "/backend-api/codex"} {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{SystemPromptEnabled: true, SystemPrompt: "Group instruction"}})
		})
		r.Use(GroupSystemPrompt())
		r.POST(prefix+"/responses", func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			require.Equal(t, int64(len(body)), c.Request.ContentLength)
			c.Data(http.StatusOK, "application/json", body)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, prefix+"/responses", strings.NewReader(`{"input":"Hello","instructions":"Client"}`)))
		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, `{"input":"Hello","instructions":"Client\n\nGroup instruction"}`, w.Body.String())
		bad := httptest.NewRecorder()
		r.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, prefix+"/responses", strings.NewReader(`{"instructions":[]}`)))
		require.Equal(t, http.StatusBadRequest, bad.Code)
	}
}

func TestGroupSystemPromptSkipsMediaAndGET(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/v1/embeddings", "/v1/videos", "/api/v1/admin/groups/5"} {
		require.Empty(t, groupSystemPromptProtocol(path))
	}
	require.Equal(t, "gemini", groupSystemPromptProtocol("/v1beta/models/gemini:streamGenerateContent"))
	require.Equal(t, "responses", groupSystemPromptProtocol("/v1/responses/compact"))
}
