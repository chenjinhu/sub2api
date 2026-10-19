package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupSystemPromptPreservesClientData(t *testing.T) {
	group := &Group{SystemPromptEnabled: true, SystemPrompt: "Group instruction"}
	cases := []struct{ name, body, field, expected string }{
		{"responses", `{"instructions":"Client instruction","input":"Hello","large_id":9007199254740993}`, "instructions", `"Client instruction\n\nGroup instruction"`},
		{"responses", `{"input":"Hello"}`, "instructions", `"Group instruction"`},
		{"chat", `{"messages":[{"role":"developer","content":"Client"},{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`, "messages", `[{"role":"developer","content":"Client"},{"content":"Group instruction","role":"system"},{"role":"user","content":[{"type":"text","text":"Hello"}]}]`},
		{"messages", `{"system":[{"type":"text","text":"Client","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"Hello"}]}`, "system", `[{"type":"text","text":"Client","cache_control":{"type":"ephemeral"}},{"text":"Group instruction","type":"text"}]`},
		{"messages", `{"system":"Client","messages":[]}`, "system", `"Client\n\nGroup instruction"`},
		{"gemini", `{"systemInstruction":{"role":"system","parts":[{"text":"Client"}]},"contents":[]}`, "systemInstruction", `{"parts":[{"text":"Client"},{"text":"Group instruction"}],"role":"system"}`},
	}
	for _, tt := range cases {
		t.Run(tt.name+tt.body, func(t *testing.T) {
			out, err := ApplyGroupSystemPrompt([]byte(tt.body), tt.name, group)
			require.NoError(t, err)
			var result, original map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out, &result))
			require.NoError(t, json.Unmarshal([]byte(tt.body), &original))
			require.JSONEq(t, tt.expected, string(result[tt.field]))
			for key, raw := range original {
				if key != tt.field {
					require.Equal(t, string(raw), string(result[key]), key)
				}
			}
		})
	}
}

func TestGroupSystemPromptDisabledAndInvalid(t *testing.T) {
	for _, group := range []*Group{nil, {}, {SystemPrompt: "Saved draft"}} {
		out, err := ApplyGroupSystemPrompt([]byte("opaque"), "responses", group)
		require.NoError(t, err)
		require.Equal(t, "opaque", string(out))
	}
	enabled := &Group{SystemPromptEnabled: true, SystemPrompt: "Instruction"}
	for protocol, body := range map[string]string{"responses": `{"instructions":[]}`, "chat": `{"messages":"invalid"}`, "messages": `{"system":5}`, "gemini": `{"systemInstruction":[]}`} {
		_, err := ApplyGroupSystemPrompt([]byte(body), protocol, enabled)
		require.Error(t, err, protocol)
	}
	require.Error(t, ValidateGroupSystemPrompt(&Group{SystemPromptEnabled: true, SystemPrompt: "  "}))
	// 长度不设上限：完整装甲提示词必须能原样保存。
	require.NoError(t, ValidateGroupSystemPrompt(&Group{SystemPromptEnabled: true, SystemPrompt: strings.Repeat("字", 200000)}))
}

func TestGroupSystemPromptAuthSnapshotAndWS(t *testing.T) {
	key := &APIKey{ID: 1, Key: "sk-prompt", User: &User{ID: 2}, Group: &Group{ID: 5, SystemPromptEnabled: true, SystemPrompt: "Prompt"}}
	svc := &APIKeyService{}
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), key)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	materialized, used, err := svc.applyAuthCacheEntry(key.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, "Prompt", materialized.Group.SystemPrompt)
	require.True(t, materialized.Group.SystemPromptEnabled)
	hooks := &OpenAIWSIngressHooks{SystemPromptGroup: materialized.Group}
	for _, input := range []string{`{"type":"response.create","instructions":"First","input":[]}`, `{"type":"response.create","instructions":"Next","previous_response_id":"r1","input":[]}`} {
		out, err := applyWSGroupSystemPrompt([]byte(input), hooks)
		require.NoError(t, err)
		require.Contains(t, string(out), `\n\nPrompt`)
	}
}
