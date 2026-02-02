package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/health-kit"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/handler"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit"
	"github.com/soulteary/provider-kit"
)

// Setup mounts routes. dingtalkClient and idemStore can be nil if config invalid (send will return 503).
func Setup(app *fiber.App, log *logger.Logger) {
	idemStore := idempotency.NewStore(config.IdemTTLSec)
	var dingtalkClient *dingtalk.Client
	if config.Valid() {
		dingtalkClient = dingtalk.NewClient(config.AppKey, config.AppSecret, config.AgentID)
	}
	v1 := app.Group("/v1")
	v1.Post("/send", func(c *fiber.Ctx) error {
		if dingtalkClient == nil {
			log.Warn().Msg("send 503: dingtalk not configured")
			return c.Status(fiber.StatusServiceUnavailable).JSON(provider.HTTPSendResponse{
				OK: false, ErrorCode: "provider_down", ErrorMessage: "dingtalk not configured",
			})
		}
		return handler.SendHandler(c, dingtalkClient, idemStore, log)
	})
	app.Get("/healthz", health.SimpleFiberHandler("herald-dingtalk"))
}
