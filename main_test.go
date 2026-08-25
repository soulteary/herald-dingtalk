package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
)

func TestNewAppEnforcesBodyLimit(t *testing.T) {
	original := config.MaxRequestBodyBytes
	config.MaxRequestBodyBytes = 32
	t.Cleanup(func() { config.MaxRequestBodyBytes = original })

	app := newApp()
	app.Post("/echo", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(strings.Repeat("x", 33)))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		t.Fatalf("expected body limit error, got status %d", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "body size exceeds") {
		t.Fatalf("error = %q, want body size limit error", err)
	}
}
