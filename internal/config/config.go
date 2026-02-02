package config

import (
	"github.com/soulteary/cli-kit/env"
)

var (
	Port       = env.Get("PORT", ":8083")
	APIKey     = env.Get("API_KEY", "")
	AppKey     = env.Get("DINGTALK_APP_KEY", "")
	AppSecret  = env.Get("DINGTALK_APP_SECRET", "")
	AgentID    = env.Get("DINGTALK_AGENT_ID", "")
	LogLevel   = env.Get("LOG_LEVEL", "info")
	IdemTTLSec = env.GetInt("IDEMPOTENCY_TTL_SECONDS", 300)
)

// ValidWith returns true when all three DingTalk credentials are non-empty.
func ValidWith(appKey, appSecret, agentID string) bool {
	return appKey != "" && appSecret != "" && agentID != ""
}

// Valid returns true when configured DingTalk credentials are set.
func Valid() bool {
	return ValidWith(AppKey, AppSecret, AgentID)
}
