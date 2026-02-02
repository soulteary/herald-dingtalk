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
