package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GroupSystemPrompt applies authenticated group instructions before parsing,
// auditing, and forwarding. WebSocket frames are handled in their ingress parser.
func GroupSystemPrompt() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := GetAPIKeyFromContext(c)
		if !ok || key == nil || key.Group == nil || !key.Group.SystemPromptEnabled || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		protocol := groupSystemPromptProtocol(c.Request.URL.Path)
		if protocol == "" {
			c.Next()
			return
		}
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
			}
			groupModelAllowlistErrorWriter(c)(c, status, "Failed to read request body")
			c.Abort()
			return
		}
		body, err = service.ApplyGroupSystemPrompt(body, protocol, key.Group)
		if err != nil {
			groupModelAllowlistErrorWriter(c)(c, http.StatusBadRequest, err.Error())
			c.Abort()
			return
		}
		requestmodel.ResetRequestBody(c.Request, body)
		c.Request.Header.Del("Content-Encoding")
		c.Next()
	}
}

func groupSystemPromptProtocol(path string) string {
	path = strings.TrimSuffix(path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return "chat"
	case strings.HasSuffix(path, "/messages"), strings.HasSuffix(path, "/messages/count_tokens"):
		return "messages"
	case strings.HasSuffix(path, "/responses"), strings.HasSuffix(path, "/responses/compact"), strings.HasSuffix(path, "/responses/input_tokens"):
		return "responses"
	case strings.HasSuffix(path, ":generateContent"), strings.HasSuffix(path, ":streamGenerateContent"), strings.HasSuffix(path, ":countTokens"):
		return "gemini"
	default:
		return ""
	}
}
