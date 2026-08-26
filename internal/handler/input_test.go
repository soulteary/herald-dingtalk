package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestRequestContextAppliesDefaultAndExplicitDeadlines(t *testing.T) {
	for _, tt := range []struct {
		name           string
		timeoutSeconds int
		wantSeconds    int
	}{
		{name: "default", timeoutSeconds: 0, wantSeconds: defaultRequestTimeoutSeconds},
		{name: "explicit", timeoutSeconds: 3, wantSeconds: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error {
				parent, parentCancel := context.WithCancel(context.Background())
				defer parentCancel()
				c.SetContext(parent)

				ctx, cancel := requestContext(c, tt.timeoutSeconds)
				defer cancel()
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("request context has no deadline")
				}
				remaining := time.Until(deadline)
				want := time.Duration(tt.wantSeconds) * time.Second
				if remaining < want-time.Second || remaining > want {
					t.Fatalf("remaining deadline = %s, want approximately %s", remaining, want)
				}
				parentCancel()
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("request context did not inherit parent cancellation")
				}
				return c.SendStatus(fiber.StatusNoContent)
			})

			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
		})
	}
}

func TestHasJSONContentType(t *testing.T) {
	for _, tt := range []struct {
		contentType string
		want        bool
	}{
		{contentType: "application/json", want: true},
		{contentType: "application/json; charset=utf-8", want: true},
		{contentType: "text/plain", want: false},
		{contentType: "", want: false},
		{contentType: "not a media type", want: false},
	} {
		app := fiber.New()
		app.Post("/", func(c fiber.Ctx) error {
			if got := hasJSONContentType(c); got != tt.want {
				t.Errorf("hasJSONContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
			return c.SendStatus(fiber.StatusNoContent)
		})
		req := httptest.NewRequest("POST", "/", nil)
		if tt.contentType != "" {
			req.Header.Set(fiber.HeaderContentType, tt.contentType)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

func TestValidBoundedToken(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		max   int
		want  bool
	}{
		{name: "valid", value: "user-123", max: 8, want: true},
		{name: "empty", value: "", max: 8, want: false},
		{name: "too long", value: strings.Repeat("x", 9), max: 8, want: false},
		{name: "leading whitespace", value: " user", max: 8, want: false},
		{name: "trailing whitespace", value: "user ", max: 8, want: false},
		{name: "control character", value: "user\n", max: 8, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := validBoundedToken(tt.value, tt.max); got != tt.want {
				t.Fatalf("validBoundedToken(%q, %d) = %v, want %v", tt.value, tt.max, got, tt.want)
			}
		})
	}
}
