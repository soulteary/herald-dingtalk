package config

import (
	"github.com/soulteary/cli-kit/env"
)

// LookupModeNone 表示 to 仅支持钉钉 userid，不按手机号查询。
const LookupModeNone = "none"

// LookupModeMobile 表示 to 支持 userid 或手机号；为手机号时调用钉钉 API 查 userid 再发送。
const LookupModeMobile = "mobile"

var (
	Port       = env.Get("PORT", ":8083")
	APIKey     = env.Get("API_KEY", "")
	AppKey     = env.Get("DINGTALK_APP_KEY", "")
	AppSecret  = env.Get("DINGTALK_APP_SECRET", "")
	AgentID    = env.Get("DINGTALK_AGENT_ID", "")
	LogLevel   = env.Get("LOG_LEVEL", "info")
	IdemTTLSec = env.GetInt("IDEMPOTENCY_TTL_SECONDS", 300)
	// LookupMode: none=to 仅 userid；mobile=to 支持 userid 或手机号（需申请 Contact.User.mobile 权限）
	LookupMode = env.Get("DINGTALK_LOOKUP_MODE", LookupModeNone)
)

// ValidWith returns true when all three DingTalk credentials are non-empty.
func ValidWith(appKey, appSecret, agentID string) bool {
	return appKey != "" && appSecret != "" && agentID != ""
}

// Valid returns true when configured DingTalk credentials are set.
func Valid() bool {
	return ValidWith(AppKey, AppSecret, AgentID)
}
