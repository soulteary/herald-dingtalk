package middleware

import (
	"crypto/sha256"
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/observability"
	"github.com/soulteary/logger-kit"
)

const apiKeyHeader = "X-API-Key"

// RequireAPIKey protects a route group when an API key is configured.
// Hashing both values before comparing keeps the comparison length fixed and
// avoids leaking the configured key through simple timing differences.
func RequireAPIKey(expected string, log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if expected == "" || apiKeyMatches(expected, c.Get(apiKeyHeader)) {
			return c.Next()
		}

		observability.RequestLogger(c, log).Warn().Str("client_ip", c.IP()).Msg("request unauthorized: invalid or missing API key")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":            false,
			"error_code":    "unauthorized",
			"error_message": "invalid or missing API key",
		})
	}
}

func apiKeyMatches(expected, provided string) bool {
	expectedHash := sha256.Sum256([]byte(expected))
	providedHash := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) == 1
}
