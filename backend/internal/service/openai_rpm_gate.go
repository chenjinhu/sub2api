package service

import "context"

// openAIAccountRPMExceeded 判断 OpenAI OAuth/SetupToken 账号是否已超过账号级
// base_rpm 软限流阈值。计数单位是调度准入和 WebSocket 后续轮次，
// 不代表上游实际收到的请求数；检查与递增不是原子操作，并发时允许少量超出。
//
// fail-open：rpmCache 未注入（nil）、base_rpm 未配置（<=0）、或 GetRPM 出错时
// 一律返回 false（不拦截），保持向后兼容。isSticky=true 时允许进入 sticky-only
// 黄区，粘性会话可继续，非粘性新会话被拒。
func (s *OpenAIGatewayService) openAIAccountRPMExceeded(ctx context.Context, account *Account, isSticky bool) bool {
	if s == nil || account == nil || !account.IsOpenAIOAuthLike() {
		return false
	}
	baseRPM := account.GetBaseRPM()
	if baseRPM <= 0 {
		return false
	}
	var currentRPM int
	if s.rpmCache != nil {
		if count, err := s.rpmCache.GetRPM(ctx, account.ID); err == nil {
			currentRPM = count
		}
	}
	switch account.CheckRPMSchedulability(currentRPM) {
	case WindowCostSchedulable:
		return false
	case WindowCostStickyOnly:
		return !isSticky
	case WindowCostNotSchedulable:
		return true
	}
	return false
}

// AdmitOpenAIAccountRPM checks and counts a scheduling admission. WebSocket
// connections must call it for subsequent turns because they do not reselect.
// Cache failures fail open, matching the existing account RPM policy.
func (s *OpenAIGatewayService) AdmitOpenAIAccountRPM(ctx context.Context, account *Account, isSticky bool) bool {
	if s.openAIAccountRPMExceeded(ctx, account, isSticky) {
		return false
	}
	if s != nil && s.rpmCache != nil && account.IsOpenAIOAuthLike() && account.GetBaseRPM() > 0 {
		_, _ = s.rpmCache.IncrementRPM(ctx, account.ID)
	}
	return true
}
