package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/logger-kit"
)

func TestRequireAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		provided string
		want     int
	}{
		{name: "disabled", want: http.StatusNoContent},
		{name: "matching key", expected: "correct-secret", provided: "correct-secret", want: http.StatusNoContent},
		{name: "missing key", expected: "correct-secret", want: http.StatusUnauthorized},
		{name: "wrong key", expected: "correct-secret", provided: "wrong", want: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			log := logger.New(logger.Config{Level: logger.ErrorLevel})
			app.Use(RequireAPIKey(tt.expected, log))
			app.Post("/protected", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

			req := httptest.NewRequest(http.MethodPost, "/protected", nil)
			if tt.provided != "" {
				req.Header.Set(apiKeyHeader, tt.provided)
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

func TestAPIKeyMatches(t *testing.T) {
	if !apiKeyMatches("same", "same") {
		t.Fatal("matching keys must be accepted")
	}
	if apiKeyMatches("same", "different") || apiKeyMatches("same", "") {
		t.Fatal("different keys must be rejected")
	}
}
