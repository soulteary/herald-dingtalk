package config

import "testing"

func TestValidWith(t *testing.T) {
	tests := []struct {
		name    string
		appKey  string
		secret  string
		agentID string
		want    bool
	}{
		{"all empty", "", "", "", false},
		{"only appKey", "key", "", "", false},
		{"only secret", "", "secret", "", false},
		{"only agentID", "", "", "1", false},
		{"all set", "key", "secret", "1", true},
		{"blank appKey", " ", "secret", "1", false},
		{"secret surrounding whitespace", "key", " secret", "1", false},
		{"agentID surrounding whitespace", "key", "secret", "1 ", false},
		{"agentID zero", "key", "secret", "0", false},
		{"agentID negative", "key", "secret", "-1", false},
		{"agentID not numeric", "key", "secret", "agent", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidWith(tt.appKey, tt.secret, tt.agentID)
			if got != tt.want {
				t.Errorf("ValidWith(%q, %q, %q) = %v, want %v", tt.appKey, tt.secret, tt.agentID, got, tt.want)
			}
		})
	}
}

func TestValidUsesConfiguredCredentials(t *testing.T) {
	originalAppKey, originalSecret, originalAgentID, originalLookupMode := AppKey, AppSecret, AgentID, LookupMode
	t.Cleanup(func() {
		AppKey, AppSecret, AgentID = originalAppKey, originalSecret, originalAgentID
		LookupMode = originalLookupMode
	})

	AppKey, AppSecret, AgentID = "key", "secret", "1"
	LookupMode = LookupModeNone
	if !Valid() {
		t.Fatal("Valid() = false with complete credentials")
	}
	AgentID = ""
	if Valid() {
		t.Fatal("Valid() = true with a missing agent ID")
	}
	AgentID = "1"
	LookupMode = "unsupported"
	if Valid() {
		t.Fatal("Valid() = true with an unsupported lookup mode")
	}
}

func TestValidateWith(t *testing.T) {
	for _, mode := range []string{LookupModeNone, LookupModeMobile} {
		if err := ValidateWith("key", "secret", "1", mode); err != nil {
			t.Fatalf("ValidateWith mode %q: %v", mode, err)
		}
	}

	tests := []struct {
		name       string
		appKey     string
		appSecret  string
		agentID    string
		lookupMode string
		want       string
	}{
		{name: "missing app key", appSecret: "secret", agentID: "1", lookupMode: LookupModeNone, want: "DINGTALK_APP_KEY is required"},
		{name: "invalid agent ID", appKey: "key", appSecret: "secret", agentID: "abc", lookupMode: LookupModeNone, want: "DINGTALK_AGENT_ID must be a positive base-10 integer"},
		{name: "invalid lookup mode", appKey: "key", appSecret: "secret", agentID: "1", lookupMode: "phone", want: `DINGTALK_LOOKUP_MODE must be "none" or "mobile"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWith(tt.appKey, tt.appSecret, tt.agentID, tt.lookupMode)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLookupModeConstants(t *testing.T) {
	if LookupModeNone != "none" {
		t.Errorf("LookupModeNone = %q, want none", LookupModeNone)
	}
	if LookupModeMobile != "mobile" {
		t.Errorf("LookupModeMobile = %q, want mobile", LookupModeMobile)
	}
	// LookupMode is read from env at init; ensure it's one of the supported values when unset or set
	switch LookupMode {
	case LookupModeNone, LookupModeMobile:
	default:
		t.Logf("LookupMode = %q (from env); expected none or mobile in production", LookupMode)
	}
}

func TestEffectiveMaxRequestBodyBytes(t *testing.T) {
	original := MaxRequestBodyBytes
	t.Cleanup(func() { MaxRequestBodyBytes = original })

	for _, tt := range []struct {
		name string
		in   int
		want int
	}{
		{name: "configured", in: 8192, want: 8192},
		{name: "zero", in: 0, want: defaultMaxRequestBodyBytes},
		{name: "negative", in: -1, want: defaultMaxRequestBodyBytes},
		{name: "too large", in: maxRequestBodyBytesLimit + 1, want: defaultMaxRequestBodyBytes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			MaxRequestBodyBytes = tt.in
			if got := EffectiveMaxRequestBodyBytes(); got != tt.want {
				t.Fatalf("EffectiveMaxRequestBodyBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}
