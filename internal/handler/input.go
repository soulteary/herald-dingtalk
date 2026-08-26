package handler

import (
	"context"
	"mime"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v3"
)

const (
	maxDestinationLength         = 256
	maxAuthCodeLength            = 4096
	defaultRequestTimeoutSeconds = 25
)

// requestContext applies an end-to-end deadline to every provider operation.
// Fiber's Ctx satisfies context.Context for value lookup, but it is not itself
// cancellable. Deriving from c.Context() preserves any context installed by
// middleware and keeps the provider call bounded when the request omits an
// explicit timeout.
func requestContext(c fiber.Ctx, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultRequestTimeoutSeconds
	}
	return context.WithTimeout(c.Context(), time.Duration(timeoutSeconds)*time.Second)
}

func hasJSONContentType(c fiber.Ctx) bool {
	mediaType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	return err == nil && mediaType == fiber.MIMEApplicationJSON
}

func validBoundedToken(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || strings.TrimSpace(value) != value {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) == -1
}
