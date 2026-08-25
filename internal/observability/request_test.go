package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/logger-kit/v2"
)

func TestRequestLoggerAddsMiddlewareRequestID(t *testing.T) {
	var output bytes.Buffer
	base := logger.New(logger.Config{Level: logger.InfoLevel, Output: &output})
	config := logger.DefaultMiddlewareConfig()
	config.Logger = base
	config.GenerateRequestID = func() string { return "generated-request-id" }

	app := fiber.New()
	app.Use(logger.FiberMiddleware(config))
	app.Get("/", func(c fiber.Ctx) error {
		RequestLogger(c, base).Info().Msg("application event")
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !bytes.Contains(output.Bytes(), []byte(`"request_id":"generated-request-id"`)) {
		t.Fatalf("request ID missing from application log: %s", output.String())
	}
}

func TestRequestLoggerFallsBackWithoutMiddleware(t *testing.T) {
	var output bytes.Buffer
	base := logger.New(logger.Config{Level: logger.InfoLevel, Output: &output})
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		if got := RequestLogger(c, base); got != base {
			t.Fatal("RequestLogger must return the base logger without request context")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}
