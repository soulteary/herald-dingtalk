package router

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/soulteary/health-kit/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/handler"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	internalmiddleware "github.com/soulteary/herald-dingtalk/internal/middleware"
	"github.com/soulteary/herald-dingtalk/internal/observability"
	"github.com/soulteary/logger-kit/v2"
	"github.com/soulteary/provider-kit"
)

// Setup mounts routes. dingtalkClient and idemStore can be nil if config invalid (send will return 503).
func Setup(app *fiber.App, log *logger.Logger) {
	accessLogConfig := logger.DefaultMiddlewareConfig()
	accessLogConfig.Logger = log
	accessLogConfig.IncludeHeaders = false
	accessLogConfig.IncludeQuery = false
	accessLogConfig.IncludeBody = false
	app.Use(logger.FiberMiddleware(accessLogConfig))
	app.Use(recover.New())
	app.Use(helmet.New())

	idemStore := idempotency.NewStore(config.IdemTTLSec)
	var dingtalkClient *dingtalk.Client
	if config.Valid() {
		dingtalkClient = dingtalk.NewClient(config.AppKey, config.AppSecret, config.AgentID)
	}
	v1 := app.Group(
		"/v1",
		internalmiddleware.RequireAPIKey(config.APIKey, log),
		internalmiddleware.LimitConcurrency(config.EffectiveMaxConcurrentRequests()),
	)
	v1.Post("/send", func(c fiber.Ctx) error {
		if dingtalkClient == nil {
			observability.RequestLogger(c, log).Warn().Msg("send 503: dingtalk not configured")
			return c.Status(fiber.StatusServiceUnavailable).JSON(provider.HTTPSendResponse{
				OK: false, ErrorCode: "provider_down", ErrorMessage: "dingtalk not configured",
			})
		}
		return handler.SendHandler(c, dingtalkClient, idemStore, log)
	})
	v1.Post("/resolve", func(c fiber.Ctx) error {
		if dingtalkClient == nil {
			observability.RequestLogger(c, log).Warn().Msg("resolve 503: dingtalk not configured")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"ok": false, "error_code": "provider_down", "error_message": "dingtalk not configured",
			})
		}
		return handler.ResolveHandler(c, dingtalkClient, log)
	})
	app.Get("/healthz", health.SimpleFiberHandler("herald-dingtalk"))
	app.Get("/readyz", readinessHandler)
}

func readinessHandler(c fiber.Ctx) error {
	if !config.Valid() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status":  "not_ready",
			"service": "herald-dingtalk",
			"reason":  "dingtalk not configured",
		})
	}
	return c.JSON(fiber.Map{"status": "ready", "service": "herald-dingtalk"})
}
