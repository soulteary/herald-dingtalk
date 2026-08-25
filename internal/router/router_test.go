package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/logger-kit"
)

type configSnapshot struct {
	apiKey, appKey, appSecret, agentID, lookupMode string
}

func snapshotConfig(t *testing.T) {
	t.Helper()
	snapshot := configSnapshot{
		apiKey: config.APIKey, appKey: config.AppKey,
		appSecret: config.AppSecret, agentID: config.AgentID, lookupMode: config.LookupMode,
	}
	t.Cleanup(func() {
		config.APIKey = snapshot.apiKey
		config.AppKey = snapshot.appKey
		config.AppSecret = snapshot.appSecret
		config.AgentID = snapshot.agentID
		config.LookupMode = snapshot.lookupMode
	})
}

func testLogger() *logger.Logger {
	return logger.New(logger.Config{Level: logger.ErrorLevel})
}

func TestSetupProtectsV1Routes(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = "correct-secret"
	config.AppKey, config.AppSecret, config.AgentID = "", "", ""

	app := fiber.New()
	Setup(app, testLogger())

	for _, tt := range []struct {
		name string
		key  string
		want int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong", key: "wrong", want: http.StatusUnauthorized},
		{name: "matching", key: "correct-secret", want: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/send", nil)
			if tt.key != "" {
				req.Header.Set("X-API-Key", tt.key)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestReadinessReflectsConfiguration(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = ""
	config.AppKey, config.AppSecret, config.AgentID = "", "", ""
	config.LookupMode = config.LookupModeNone

	app := fiber.New()
	Setup(app, testLogger())

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if err != nil {
		t.Fatalf("app.Test not ready: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("not configured status = %d, want 503", resp.StatusCode)
	}

	config.AppKey, config.AppSecret, config.AgentID = "key", "secret", "1"
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if err != nil {
		t.Fatalf("app.Test ready: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configured status = %d, want 200", resp.StatusCode)
	}
}

func TestSetupRejectsSemanticallyInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		lookupMode string
	}{
		{name: "invalid agent ID", agentID: "not-a-number", lookupMode: config.LookupModeNone},
		{name: "invalid lookup mode", agentID: "1", lookupMode: "phone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshotConfig(t)
			config.APIKey = ""
			config.AppKey, config.AppSecret, config.AgentID = "key", "secret", tt.agentID
			config.LookupMode = tt.lookupMode

			app := fiber.New()
			Setup(app, testLogger())

			for _, endpoint := range []struct {
				method string
				path   string
			}{
				{method: http.MethodGet, path: "/readyz"},
				{method: http.MethodPost, path: "/v1/send"},
			} {
				resp, err := app.Test(httptest.NewRequest(endpoint.method, endpoint.path, nil))
				if err != nil {
					t.Fatalf("%s %s: %v", endpoint.method, endpoint.path, err)
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusServiceUnavailable {
					t.Fatalf("%s %s status = %d, want 503", endpoint.method, endpoint.path, resp.StatusCode)
				}
			}
		})
	}
}

func TestSetupAddsOperationalHeaders(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = ""

	app := fiber.New()
	Setup(app, testLogger())
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID must be set")
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
