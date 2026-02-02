package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit"
	"github.com/soulteary/provider-kit"
)

// SendHandler handles POST /v1/send from Herald.
func SendHandler(c *fiber.Ctx, dingtalkClient *dingtalk.Client, idemStore *idempotency.Store, log *logger.Logger) error {
	if config.APIKey != "" && c.Get("X-API-Key") != config.APIKey {
		log.Warn().Str("client_ip", c.IP()).Msg("send unauthorized: invalid or missing API key")
		return c.Status(fiber.StatusUnauthorized).JSON(provider.HTTPSendResponse{
			OK: false, ErrorCode: "unauthorized", ErrorMessage: "invalid or missing API key",
		})
	}
	var req provider.HTTPSendRequest
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("send invalid_request: body parse error")
		return c.Status(fiber.StatusBadRequest).JSON(provider.HTTPSendResponse{
			OK: false, ErrorCode: "invalid_request", ErrorMessage: err.Error(),
		})
	}
	if req.To == "" {
		log.Warn().Msg("send invalid_destination: to is required")
		return c.Status(fiber.StatusBadRequest).JSON(provider.HTTPSendResponse{
			OK: false, ErrorCode: "invalid_destination", ErrorMessage: "to is required",
		})
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = c.Get("Idempotency-Key")
	}
	if req.IdempotencyKey != "" {
		if cached, hit := idemStore.Get(req.IdempotencyKey); hit {
			log.Debug().Str("to", req.To).Bool("cached_ok", cached.OK).Str("message_id", cached.MessageID).Msg("send idempotent hit")
			return c.JSON(provider.HTTPSendResponse{
				OK: cached.OK, MessageID: cached.MessageID, Provider: "dingtalk",
			})
		}
	}
	content := req.Body
	if content == "" && len(req.Params) > 0 {
		if code, ok := req.Params["code"]; ok {
			content = "验证码：" + code
		}
	}
	if content == "" {
		content = "您有一条验证消息，请查看。"
	}
	taskID, err := dingtalkClient.SendWorkNotify(c.Context(), req.To, content)
	if err != nil {
		log.Warn().Err(err).Str("to", req.To).Msg("send_failed: dingtalk API error")
		errCode := "send_failed"
		errMsg := err.Error()
		if req.IdempotencyKey != "" {
			idemStore.Set(req.IdempotencyKey, false, "")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(provider.HTTPSendResponse{
			OK: false, ErrorCode: errCode, ErrorMessage: errMsg,
		})
	}
	if req.IdempotencyKey != "" {
		idemStore.Set(req.IdempotencyKey, true, taskID)
	}
	log.Info().Str("to", req.To).Str("message_id", taskID).Msg("send ok")
	return c.JSON(provider.HTTPSendResponse{
		OK: true, MessageID: taskID, Provider: "dingtalk",
	})
}
