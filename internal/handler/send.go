package handler

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit"
	"github.com/soulteary/provider-kit"
)

// 仅数字且长度 11 视为手机号（用于 DINGTALK_LOOKUP_MODE=mobile 时解析 to）
var mobileLike = regexp.MustCompile(`^\d{11}$`)

const maxRequestTimeoutSeconds = 30

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
		return sendError(c, fiber.StatusBadRequest, "invalid_request", err.Error())
	}
	if req.Channel != "" && req.Channel != provider.ChannelDingTalk.String() {
		log.Warn().Str("channel", req.Channel).Msg("send invalid_request: unsupported channel")
		return sendError(c, fiber.StatusBadRequest, "invalid_request", "channel must be dingtalk")
	}
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > maxRequestTimeoutSeconds {
		log.Warn().Int("timeout_seconds", req.TimeoutSeconds).Msg("send invalid_request: timeout out of range")
		return sendError(c, fiber.StatusBadRequest, "invalid_request", "timeout_seconds must be between 0 and 30")
	}
	if req.To == "" {
		log.Warn().Msg("send invalid_destination: to is required")
		return sendError(c, fiber.StatusBadRequest, "invalid_destination", "to is required")
	}
	requestCtx := context.Context(c.Context())
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(requestCtx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = c.Get("Idempotency-Key")
	}
	if req.IdempotencyKey != "" {
		if cached, hit := idemStore.Get(req.IdempotencyKey); hit {
			log.Debug().Str("to", req.To).Bool("cached_ok", cached.OK).Str("message_id", cached.MessageID).Msg("send idempotent hit")
			return c.JSON(provider.HTTPSendResponse{
				OK: cached.OK, MessageID: cached.MessageID, Provider: provider.ChannelDingTalk.String(),
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
	destUserID := req.To
	if config.LookupMode == config.LookupModeMobile && mobileLike.MatchString(req.To) {
		resolved, err := dingtalkClient.GetUserIDByMobile(requestCtx, req.To)
		if err != nil {
			if isTimeoutError(err) {
				log.Warn().Err(err).Str("to", req.To).Msg("send timeout: mobile lookup timed out")
				return sendError(c, fiber.StatusGatewayTimeout, "timeout", "dingtalk request timed out")
			}
			log.Warn().Err(err).Str("to", req.To).Msg("send invalid_destination: mobile lookup failed")
			return sendError(c, fiber.StatusBadRequest, "invalid_destination", "mobile lookup failed: "+err.Error())
		}
		destUserID = resolved
		log.Debug().Str("mobile", req.To).Str("userid", destUserID).Msg("send: resolved mobile to userid")
	}
	taskID, err := dingtalkClient.SendWorkNotify(requestCtx, destUserID, content)
	if err != nil {
		log.Warn().Err(err).Str("to", destUserID).Msg("send_failed: dingtalk API error")
		if req.IdempotencyKey != "" {
			idemStore.Set(req.IdempotencyKey, false, "")
		}
		if isTimeoutError(err) {
			return sendError(c, fiber.StatusGatewayTimeout, "timeout", "dingtalk request timed out")
		}
		return sendError(c, fiber.StatusBadGateway, "send_failed", err.Error())
	}
	if req.IdempotencyKey != "" {
		idemStore.Set(req.IdempotencyKey, true, taskID)
	}
	log.Info().Str("to", req.To).Str("message_id", taskID).Msg("send ok")
	return c.JSON(provider.HTTPSendResponse{
		OK: true, MessageID: taskID, Provider: provider.ChannelDingTalk.String(),
	})
}

func isTimeoutError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func sendError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(provider.HTTPSendResponse{
		OK: false, ErrorCode: code, ErrorMessage: message,
	})
}
