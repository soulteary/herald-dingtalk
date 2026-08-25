package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestLimitConcurrencyRejectsExcessRequestsAndRecovers(t *testing.T) {
	app := fiber.New()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	app.Use(LimitConcurrency(1))
	app.Get("/work", func(c fiber.Ctx) error {
		entered <- struct{}{}
		<-release
		return c.SendStatus(fiber.StatusNoContent)
	})

	firstDone := make(chan *http.Response, 1)
	go func() {
		resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/work", nil), fiber.TestConfig{Timeout: 5 * time.Second})
		firstDone <- resp
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter the handler")
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/work", nil))
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	var body struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ErrorCode != "rate_limited" {
		t.Fatalf("error_code = %q, want rate_limited", body.ErrorCode)
	}

	close(release)
	select {
	case first := <-firstDone:
		if first == nil {
			t.Fatal("first request failed")
		}
		_ = first.Body.Close()
		if first.StatusCode != http.StatusNoContent {
			t.Fatalf("first status = %d, want 204", first.StatusCode)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}

	after, err := app.Test(httptest.NewRequest(http.MethodGet, "/work", nil))
	if err != nil {
		t.Fatalf("request after release: %v", err)
	}
	_ = after.Body.Close()
	if after.StatusCode != http.StatusNoContent {
		t.Fatalf("status after release = %d, want 204", after.StatusCode)
	}
}

func TestLimitConcurrencyCanBeDisabled(t *testing.T) {
	app := fiber.New()
	app.Use(LimitConcurrency(0))
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}
