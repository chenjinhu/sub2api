package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupSystemPromptAdminVisibility(t *testing.T) {
	group := &service.Group{ID: 5, SystemPromptEnabled: true, SystemPrompt: "Group instruction"}
	user, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(user), "system_prompt")
	admin, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(admin), `"system_prompt_enabled":true`)
	require.Contains(t, string(admin), `"system_prompt":"Group instruction"`)
}
