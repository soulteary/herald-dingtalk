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
