package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIRPMTestCache struct {
	RPMCache
	counts        map[int64]int
	err           error
	reads, writes int
}

func (c *openAIRPMTestCache) GetRPM(_ context.Context, id int64) (int, error) {
	c.reads++
	return c.counts[id], c.err
}
func (c *openAIRPMTestCache) IncrementRPM(_ context.Context, id int64) (int, error) {
	c.writes++
	if c.err != nil {
		return 0, c.err
	}
	c.counts[id]++
	return c.counts[id], nil
}

func TestOpenAIRPMAdmission(t *testing.T) {
	for _, tt := range []struct {
		name, platform, kind, strategy string
		base, count                    int
		sticky, cacheError, allowed    bool
	}{
		{name: "below threshold", base: 2, count: 1, allowed: true},
		{name: "new session yellow", base: 2, count: 2},
		{name: "sticky yellow", base: 2, count: 2, sticky: true, allowed: true},
		{name: "sticky red", base: 2, count: 3, sticky: true},
		{name: "sticky exempt", base: 2, count: 100, sticky: true, strategy: "sticky_exempt", allowed: true},
		{name: "new session not exempt", base: 2, count: 100, strategy: "sticky_exempt"},
		{name: "disabled", allowed: true},
		{name: "api key excluded", kind: AccountTypeAPIKey, base: 2, count: 100, allowed: true},
		{name: "other platform excluded", platform: PlatformAnthropic, base: 2, count: 100, allowed: true},
		{name: "setup token", kind: AccountTypeSetupToken, base: 2, count: 3},
		{name: "cache failure", base: 2, count: 0, cacheError: true, allowed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			platform, kind := tt.platform, tt.kind
			if platform == "" {
				platform = PlatformOpenAI
			}
			if kind == "" {
				kind = AccountTypeOAuth
			}
			a := &Account{ID: 1, Platform: platform, Type: kind, Extra: map[string]any{"base_rpm": tt.base, "rpm_sticky_buffer": 1, "rpm_strategy": tt.strategy}}
			cache := &openAIRPMTestCache{counts: map[int64]int{1: tt.count}}
			if tt.cacheError {
				cache.err = errors.New("unavailable")
			}
			svc := &OpenAIGatewayService{rpmCache: cache}
			require.Equal(t, tt.allowed, svc.AdmitOpenAIAccountRPM(context.Background(), a, tt.sticky))
			enabled := a.IsOpenAIOAuthLike() && tt.base > 0
			if !enabled {
				require.Zero(t, cache.reads)
			}
			expectedWrites := 0
			if enabled && tt.allowed {
				expectedWrites = 1
			}
			require.Equal(t, expectedWrites, cache.writes)
		})
	}
	var svc *OpenAIGatewayService
	require.True(t, svc.AdmitOpenAIAccountRPM(context.Background(), nil, false))
	require.True(t, (&OpenAIGatewayService{}).AdmitOpenAIAccountRPM(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"base_rpm": 1}}, false))
}

func TestOpenAIRPMSchedulerPaths(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			for _, binding := range []string{"fresh", "session", "previous"} {
				for _, count := range []int{1, 2, 3} {
					t.Run(fmt.Sprintf("advanced=%t/batch=%t/%s/count=%d", advanced, batch, binding, count), func(t *testing.T) {
						resetOpenAIAdvancedSchedulerSettingCacheForTest()
						t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
						ctx := context.Background()
						groupID := int64(9001)
						a := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Extra: map[string]any{"base_rpm": 2, "rpm_sticky_buffer": 1, "responses_websockets_v2_enabled": true}}
						cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
						cfg := &config.Config{}
						cfg.Gateway.Scheduling.LoadBatchEnabled = batch
						cfg.Gateway.OpenAIWS.Enabled = true
						cfg.Gateway.OpenAIWS.OAuthEnabled = true
						cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
						rpm := &openAIRPMTestCache{counts: map[int64]int{1: count}}
						var acquired, released []int64
						svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{a}}, cache: cache, cfg: cfg, rpmCache: rpm, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(fmt.Sprint(advanced)), concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired, releasedIDs: &released})}
						session, previous := "", ""
						if binding == "session" {
							session = "rpm-session"
							cache.sessionBindings["openai:"+session] = 1
						}
						if binding == "previous" {
							previous = "resp_rpm"
							require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, previous, 1, time.Hour))
						}
						excluded := map[int64]struct{}{99: {}}
						selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, previous, session, "gpt-5.1", excluded, OpenAIUpstreamTransportAny, false)
						allowed := count < 2 || (binding != "fresh" && count == 2)
						if allowed {
							require.NoError(t, err)
							require.NotNil(t, selection)
							require.Equal(t, 1, rpm.writes)
							require.Equal(t, count+1, rpm.counts[1])
							if selection.ReleaseFunc != nil {
								selection.ReleaseFunc()
							}
						} else {
							require.ErrorIs(t, err, ErrNoAvailableAccounts)
							require.Nil(t, selection)
							require.Zero(t, rpm.writes)
						}
						require.Equal(t, map[int64]struct{}{99: {}}, excluded, "caller exclusions must not be changed")
						require.ElementsMatch(t, acquired, released, "rejected sticky selections must release their slots")
					})
				}
			}
		}
	}
}
