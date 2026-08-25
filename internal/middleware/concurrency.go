package middleware

import "github.com/gofiber/fiber/v3"

// LimitConcurrency rejects excess in-flight requests instead of allowing an
// unbounded queue to consume process resources. Non-positive limits disable it.
func LimitConcurrency(max int) fiber.Handler {
	if max <= 0 {
		return func(c fiber.Ctx) error { return c.Next() }
	}

	slots := make(chan struct{}, max)
	return func(c fiber.Ctx) error {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			return c.Next()
		default:
			c.Set(fiber.HeaderRetryAfter, "1")
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"ok":            false,
				"error_code":    "rate_limited",
				"error_message": "too many concurrent requests",
			})
		}
	}
}
