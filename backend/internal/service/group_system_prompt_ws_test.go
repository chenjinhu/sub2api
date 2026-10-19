package service

import (
	"context"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupSystemPromptWSPassthroughEveryTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstream := newStagedPassthroughConn()
	hooks := &OpenAIWSIngressHooks{SystemPromptGroup: &Group{SystemPromptEnabled: true, SystemPrompt: "Group instruction"}}
	server, _ := startPassthroughHookRecordingServer(t, ctx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount(), hooks)
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer client.CloseNow()
	first := requirePassthroughUpstreamWrite(t, upstream, time.Second)
	require.Equal(t, "Group instruction", gjson.GetBytes(first, "instructions").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"r1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), time.Second)
	defer writeCancel()
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"Client instruction","input":[]}`)))
	next := requirePassthroughUpstreamWrite(t, upstream, time.Second)
	require.Equal(t, "Client instruction\n\nGroup instruction", gjson.GetBytes(next, "instructions").String())
}
