package router

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/logger-kit"
)

type configSnapshot struct {
	apiKey, appKey, appSecret, agentID, lookupMode string
	maxConcurrentRequests                          int
}

func snapshotConfig(t *testing.T) {
	t.Helper()
	snapshot := configSnapshot{
		apiKey: config.APIKey, appKey: config.AppKey,
		appSecret: config.AppSecret, agentID: config.AgentID, lookupMode: config.LookupMode,
		maxConcurrentRequests: config.MaxConcurrentRequests,
	}
	t.Cleanup(func() {
		config.APIKey = snapshot.apiKey
		config.AppKey = snapshot.appKey
		config.AppSecret = snapshot.appSecret
		config.AgentID = snapshot.agentID
		config.LookupMode = snapshot.lookupMode
		config.MaxConcurrentRequests = snapshot.maxConcurrentRequests
	})
}

func testLogger() *logger.Logger {
	return logger.New(logger.Config{Level: logger.ErrorLevel, Output: io.Discard})
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
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "caller-request-id")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Request-ID"); got != "caller-request-id" {
		t.Fatalf("X-Request-ID = %q, want caller-request-id", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestSetupEmitsSafeCorrelatedAccessLog(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = "api-key-secret"
	config.AppKey, config.AppSecret, config.AgentID = "", "", ""
	config.LookupMode = config.LookupModeNone

	var output bytes.Buffer
	log := logger.New(logger.Config{Level: logger.DebugLevel, Output: &output})
	app := fiber.New()
	Setup(app, log)
	req := httptest.NewRequest(http.MethodPost, "/v1/send?to=query-recipient-secret", bytes.NewBufferString(`{"to":"body-recipient-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "api-key-secret")
	req.Header.Set("X-Request-ID", "request-123")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	for _, secret := range []string{"api-key-secret", "body-recipient-secret", "query-recipient-secret"} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Fatalf("access logs exposed %q", secret)
		}
	}

	var accessLog map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(output.Bytes()))
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode log: %v", err)
		}
		if entry["message"] == "HTTP request" {
			accessLog = entry
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if accessLog == nil {
		t.Fatal("structured access log was not emitted")
	}
	if accessLog["method"] != http.MethodPost || accessLog["path"] != "/v1/send" {
		t.Fatalf("unexpected request fields: %#v", accessLog)
	}
	if accessLog["status"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("status = %#v, want 503", accessLog["status"])
	}
	if accessLog["request_id"] != "request-123" {
		t.Fatalf("request_id = %#v, want request-123", accessLog["request_id"])
	}
}

func TestSetupRoutesConfiguredRequestsToHandlers(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = ""
	config.AppKey, config.AppSecret, config.AgentID = "key", "secret", "1"
	config.LookupMode = config.LookupModeNone

	app := fiber.New()
	Setup(app, testLogger())
	for _, path := range []string{"/v1/send", "/v1/resolve"} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "text/plain")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("POST %s status = %d, want 415", path, resp.StatusCode)
		}
	}
}

func TestSetupReturnsProviderDownForResolve(t *testing.T) {
	snapshotConfig(t)
	config.APIKey = ""
	config.AppKey, config.AppSecret, config.AgentID = "", "", ""
	config.LookupMode = config.LookupModeNone

	app := fiber.New()
	Setup(app, testLogger())
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/resolve", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
