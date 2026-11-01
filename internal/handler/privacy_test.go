package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit/v3"
)

type privacyTransport func(*http.Request) (*http.Response, error)

func (f privacyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderFailuresDoNotLogCredentials(t *testing.T) {
	oldMode := config.LookupMode
	config.LookupMode = config.LookupModeNone
	t.Cleanup(func() { config.LookupMode = oldMode })
	for _, scenario := range []string{"token transport", "send transport", "upstream message", "malformed body"} {
		t.Run(scenario, func(t *testing.T) {
			const sensitive = "private-canary-value-938217"
			var logs bytes.Buffer
			log := logger.New(logger.Config{Level: logger.InfoLevel, Output: &logs})
			client := dingtalk.NewClientWithHTTP(sensitive, sensitive, "1", &http.Client{Transport: privacyTransport(func(r *http.Request) (*http.Response, error) {
				if scenario == "token transport" {
					return nil, errors.New("connection reset")
				}
				body := `{"access_token":"` + sensitive + `","expires_in":7200}`
				if !strings.Contains(r.URL.Path, "gettoken") {
					if scenario == "send transport" {
						return nil, errors.New("connection reset")
					}
					body = `{"errcode":500,"errmsg":"` + sensitive + `"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			app := fiber.New()
			app.Post("/send", func(c fiber.Ctx) error { return SendHandler(c, client, idempotency.NewStore(300), log) })
			body := `{"to":"user","body":"hello"}`
			if scenario == "malformed body" {
				body = `{"params":{"` + sensitive + `":[]}}`
			}
			req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode < 400 {
				t.Fatalf("failure returned success: %d", resp.StatusCode)
			}
			if strings.Contains(logs.String(), sensitive) {
				t.Error("credential or payload fragment leaked to logs")
			}
			if strings.Contains(out["error_message"].(string), sensitive) {
				t.Error("payload fragment leaked to HTTP response")
			}
		})
	}
}

func TestMobileAndResolveProviderLogsAreRedacted(t *testing.T) {
	oldMode := config.LookupMode
	config.LookupMode = config.LookupModeMobile
	t.Cleanup(func() { config.LookupMode = oldMode })
	for _, operation := range []string{"mobile", "resolve"} {
		t.Run(operation, func(t *testing.T) {
			const sensitive = "private-upstream-body-938217"
			var logs bytes.Buffer
			log := logger.New(logger.Config{Level: logger.InfoLevel, Output: &logs})
			client := dingtalk.NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: privacyTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New(sensitive) })})
			app := fiber.New()
			app.Post("/", func(c fiber.Ctx) error {
				if operation == "resolve" {
					return ResolveHandler(c, client, log)
				}
				return SendHandler(c, client, idempotency.NewStore(300), log)
			})
			body := `{"to":"13800138000","body":"hello"}`
			if operation == "resolve" {
				body = `{"auth_code":"test-auth-code"}`
			}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadGateway {
				t.Fatalf("status=%d, want 502", resp.StatusCode)
			}
			if strings.Contains(logs.String(), sensitive) || !strings.Contains(logs.String(), "error_category") {
				t.Fatal("expected redacted error category in provider failure log")
			}
		})
	}
}
