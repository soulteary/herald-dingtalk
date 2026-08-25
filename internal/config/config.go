package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/soulteary/cli-kit/env"
)

// LookupModeNone 表示 to 仅支持钉钉 userid，不按手机号查询。
const LookupModeNone = "none"

// LookupModeMobile 表示 to 支持 userid 或手机号；为手机号时调用钉钉 API 查 userid 再发送。
const LookupModeMobile = "mobile"

const (
	defaultMaxRequestBodyBytes = 64 * 1024
	maxRequestBodyBytesLimit   = 1024 * 1024
)

var (
	Port       = env.Get("PORT", ":8083")
	APIKey     = env.Get("API_KEY", "")
	AppKey     = env.Get("DINGTALK_APP_KEY", "")
	AppSecret  = env.Get("DINGTALK_APP_SECRET", "")
	AgentID    = env.Get("DINGTALK_AGENT_ID", "")
	LogLevel   = env.Get("LOG_LEVEL", "info")
	IdemTTLSec = env.GetInt("IDEMPOTENCY_TTL_SECONDS", 300)
	// MaxRequestBodyBytes bounds memory used while parsing one HTTP request.
	MaxRequestBodyBytes = env.GetInt("MAX_REQUEST_BODY_BYTES", defaultMaxRequestBodyBytes)
	// LookupMode: none=to 仅 userid；mobile=to 支持 userid 或手机号（需申请 Contact.User.mobile 权限）
	LookupMode = env.Get("DINGTALK_LOOKUP_MODE", LookupModeNone)
)

// EffectiveMaxRequestBodyBytes returns a safe request body limit. Invalid or
// excessively large values fall back to the documented default.
func EffectiveMaxRequestBodyBytes() int {
	if MaxRequestBodyBytes <= 0 || MaxRequestBodyBytes > maxRequestBodyBytesLimit {
		return defaultMaxRequestBodyBytes
	}
	return MaxRequestBodyBytes
}

// ValidWith returns true when DingTalk credentials are complete, have no
// surrounding whitespace, and agentID is a positive base-10 integer.
func ValidWith(appKey, appSecret, agentID string) bool {
	return validateCredentialsWith(appKey, appSecret, agentID) == nil
}

// ValidateWith validates all DingTalk settings without exposing credential
// values in errors.
func ValidateWith(appKey, appSecret, agentID, lookupMode string) error {
	if err := validateCredentialsWith(appKey, appSecret, agentID); err != nil {
		return err
	}
	if lookupMode != LookupModeNone && lookupMode != LookupModeMobile {
		return fmt.Errorf("DINGTALK_LOOKUP_MODE must be %q or %q", LookupModeNone, LookupModeMobile)
	}
	return nil
}

// Validate validates the process-level DingTalk configuration.
func Validate() error {
	return ValidateWith(AppKey, AppSecret, AgentID, LookupMode)
}

// Valid returns true when the process-level DingTalk configuration is ready.
func Valid() bool {
	return Validate() == nil
}

func validateCredentialsWith(appKey, appSecret, agentID string) error {
	credentials := []struct {
		name  string
		value string
	}{
		{name: "DINGTALK_APP_KEY", value: appKey},
		{name: "DINGTALK_APP_SECRET", value: appSecret},
		{name: "DINGTALK_AGENT_ID", value: agentID},
	}
	for _, credential := range credentials {
		if credential.value == "" {
			return fmt.Errorf("%s is required", credential.name)
		}
		if strings.TrimSpace(credential.value) != credential.value {
			return fmt.Errorf("%s must not contain surrounding whitespace", credential.name)
		}
	}

	parsedAgentID, err := strconv.ParseInt(agentID, 10, 64)
	if err != nil || parsedAgentID <= 0 {
		return fmt.Errorf("DINGTALK_AGENT_ID must be a positive base-10 integer")
	}
	return nil
}
