package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/logger-kit"
)

// ResolveRequest body for POST /v1/resolve.
type ResolveRequest struct {
	AuthCode string `json:"auth_code"`
}

// ResolveResponse body for POST /v1/resolve.
type ResolveResponse struct {
	UserID string `json:"userid"`
}

// ResolveHandler handles POST /v1/resolve: OAuth2 auth_code -> userid.
// Optional: useful when Stargate uses DingTalk OAuth2 login link and needs to resolve code to userid.
func ResolveHandler(c *fiber.Ctx, dingtalkClient *dingtalk.Client, log *logger.Logger) error {
	if !hasJSONContentType(c) {
		log.Warn().Msg("resolve unsupported_media_type: application/json required")
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"ok": false, "error_code": "unsupported_media_type", "error_message": "Content-Type must be application/json",
		})
	}

	var req ResolveRequest
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("resolve invalid_request: body parse error")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "invalid_request", "error_message": err.Error(),
		})
	}
	if !validBoundedToken(req.AuthCode, maxAuthCodeLength) {
		log.Warn().Int("auth_code_length", len(req.AuthCode)).Msg("resolve invalid_request: invalid auth_code")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "invalid_request", "error_message": "auth_code must be 1-4096 bytes without surrounding whitespace or control characters",
		})
	}
	if dingtalkClient == nil {
		log.Warn().Msg("resolve 503: dingtalk not configured")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok": false, "error_code": "provider_down", "error_message": "dingtalk not configured",
		})
	}
	userid, err := dingtalkClient.ResolveAuthCode(c.Context(), req.AuthCode)
	if err != nil {
		log.Warn().Err(err).Msg("resolve failed: oauth2 error")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "resolve_failed", "error_message": err.Error(),
		})
	}
	log.Info().Msg("resolve ok")
	return c.JSON(fiber.Map{"ok": true, "userid": userid})
}
