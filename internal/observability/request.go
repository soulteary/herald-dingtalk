package observability

import (
	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/logger-kit/v3"
	loggerfiber "github.com/soulteary/logger-kit/v3/fiberadapter"
)

// RequestLogger adds the request ID installed by logger-kit middleware to
// application log events. Direct handler tests without middleware remain safe.
func RequestLogger(c fiber.Ctx, base *logger.Logger) *logger.Logger {
	if requestID := loggerfiber.RequestID(c); requestID != "" {
		return base.WithStr("request_id", requestID)
	}
	return base
}
