package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
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
	if config.APIKey != "" && c.Get("X-API-Key") != config.APIKey {
		log.Warn().Str("client_ip", c.IP()).Msg("resolve unauthorized: invalid or missing API key")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok": false, "error_code": "unauthorized", "error_message": "invalid or missing API key",
		})
	}
	var req ResolveRequest
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("resolve invalid_request: body parse error")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "invalid_request", "error_message": err.Error(),
		})
	}
	if req.AuthCode == "" {
		log.Warn().Msg("resolve invalid_request: auth_code is required")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "invalid_request", "error_message": "auth_code is required",
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
		log.Warn().Err(err).Str("auth_code", req.AuthCode).Msg("resolve failed: oauth2 error")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok": false, "error_code": "resolve_failed", "error_message": err.Error(),
		})
	}
	log.Info().Str("userid", userid).Msg("resolve ok")
	return c.JSON(fiber.Map{"ok": true, "userid": userid})
}
