package service

import (
	"encoding/json"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ValidateGroupSystemPrompt 只校验语义约束。长度不再设上限：分组提示词用于
// 承载完整装甲/长提示词，截断或拒绝会让整份提示词不可用；存储列为 text，
// 实际容量由数据库列类型与全局请求体上限决定（见 server.max_request_body_size）。
func ValidateGroupSystemPrompt(group *Group) error {
	if group.SystemPromptEnabled && strings.TrimSpace(group.SystemPrompt) == "" {
		return infraerrors.BadRequest("INVALID_GROUP_SYSTEM_PROMPT", "an enabled group system prompt must not be empty")
	}
	return nil
}

// ApplyGroupSystemPrompt appends the group instruction without replacing client
// instructions. Unknown JSON fields remain intact, including large numeric IDs.
func ApplyGroupSystemPrompt(body []byte, protocol string, group *Group) ([]byte, error) {
	if group == nil || !group.SystemPromptEnabled || strings.TrimSpace(group.SystemPrompt) == "" {
		return body, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("invalid request body for group system prompt")
	}
	prompt := strings.TrimSpace(group.SystemPrompt)
	switch protocol {
	case "responses":
		var current string
		if raw := fields["instructions"]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, fmt.Errorf("instructions must be a string")
			}
		}
		fields["instructions"], _ = json.Marshal(appendGroupInstruction(current, prompt))
	case "chat":
		var messages []json.RawMessage
		if err := json.Unmarshal(fields["messages"], &messages); err != nil || messages == nil {
			return nil, fmt.Errorf("messages must be an array")
		}
		instruction, _ := json.Marshal(map[string]string{"role": "system", "content": prompt})
		// Insert after the existing instruction prefix, before conversation turns.
		index := 0
		for index < len(messages) {
			var message struct {
				Role string `json:"role"`
			}
			if json.Unmarshal(messages[index], &message) != nil || (message.Role != "system" && message.Role != "developer") {
				break
			}
			index++
		}
		updated := make([]json.RawMessage, 0, len(messages)+1)
		updated = append(updated, messages[:index]...)
		updated = append(updated, instruction)
		updated = append(updated, messages[index:]...)
		fields["messages"], _ = json.Marshal(updated)
	case "messages":
		if raw := fields["system"]; len(raw) > 0 && string(raw) != "null" {
			var current string
			if json.Unmarshal(raw, &current) == nil {
				fields["system"], _ = json.Marshal(appendGroupInstruction(current, prompt))
			} else {
				var blocks []json.RawMessage
				if json.Unmarshal(raw, &blocks) != nil {
					return nil, fmt.Errorf("system must be a string or an array")
				}
				block, _ := json.Marshal(map[string]string{"type": "text", "text": prompt})
				fields["system"], _ = json.Marshal(append(blocks, block))
			}
		} else {
			fields["system"], _ = json.Marshal(prompt)
		}
	case "gemini":
		instruction := make(map[string]json.RawMessage)
		if raw := fields["systemInstruction"]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &instruction) != nil || instruction == nil {
				return nil, fmt.Errorf("systemInstruction must be an object")
			}
		}
		var parts []json.RawMessage
		if raw := instruction["parts"]; len(raw) > 0 {
			if json.Unmarshal(raw, &parts) != nil {
				return nil, fmt.Errorf("systemInstruction.parts must be an array")
			}
		}
		part, _ := json.Marshal(map[string]string{"text": prompt})
		instruction["parts"], _ = json.Marshal(append(parts, part))
		fields["systemInstruction"], _ = json.Marshal(instruction)
	default:
		return body, nil
	}
	return json.Marshal(fields)
}

func appendGroupInstruction(current, prompt string) string {
	if current == "" {
		return prompt
	}
	return current + "\n\n" + prompt
}

// applyWSGroupSystemPrompt operates on original client frames, once per turn.
func applyWSGroupSystemPrompt(body []byte, hooks *OpenAIWSIngressHooks) ([]byte, error) {
	if hooks == nil {
		return body, nil
	}
	return ApplyGroupSystemPrompt(body, "responses", hooks.SystemPromptGroup)
}
