package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	maxRequestTimeoutSeconds = 30
	maxIdempotencyKeyLength  = 256
)

type fingerprintPayload struct {
	Channel  string            `json:"channel"`
	To       string            `json:"to"`
	Template string            `json:"template,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Subject  string            `json:"subject,omitempty"`
	Body     string            `json:"body,omitempty"`
	Locale   string            `json:"locale,omitempty"`
}

// SendHandler handles POST /v1/send from Herald.
func SendHandler(c *fiber.Ctx, dingtalkClient *dingtalk.Client, idemStore *idempotency.Store, log *logger.Logger) error {
	if config.APIKey != "" && c.Get("X-API-Key") != config.APIKey {
		log.Warn().Str("client_ip", c.IP()).Msg("send unauthorized: invalid or missing API key")
		return sendError(c, fiber.StatusUnauthorized, "unauthorized", "invalid or missing API key")
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
	req.Channel = provider.ChannelDingTalk.String()
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > maxRequestTimeoutSeconds {
		log.Warn().Int("timeout_seconds", req.TimeoutSeconds).Msg("send invalid_request: timeout out of range")
		return sendError(c, fiber.StatusBadRequest, "invalid_request", "timeout_seconds must be between 0 and 30")
	}
	if req.To == "" {
		log.Warn().Msg("send invalid_destination: to is required")
		return sendError(c, fiber.StatusBadRequest, "invalid_destination", "to is required")
	}

	headerKey := c.Get("Idempotency-Key")
	if req.IdempotencyKey != "" && headerKey != "" && req.IdempotencyKey != headerKey {
		log.Warn().Msg("send idempotency_conflict: header and body keys differ")
		return sendError(c, fiber.StatusConflict, "idempotency_conflict", "header and body idempotency keys differ")
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = headerKey
	}
	if len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		log.Warn().Int("key_length", len(req.IdempotencyKey)).Msg("send invalid_request: idempotency key too long")
		return sendError(c, fiber.StatusBadRequest, "invalid_request", "idempotency key must not exceed 256 bytes")
	}

	requestCtx := context.Context(c.Context())
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(requestCtx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	fingerprint, err := requestFingerprint(req)
	if err != nil {
		log.Warn().Err(err).Msg("send invalid_request: fingerprint failed")
		return sendError(c, fiber.StatusBadRequest, "invalid_request", "request cannot be fingerprinted")
	}
	result, outcome, err := idemStore.Do(requestCtx, req.IdempotencyKey, fingerprint, func() idempotency.Result {
		return sendOnce(requestCtx, req, dingtalkClient, log)
	})
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			log.Warn().Msg("send idempotency_conflict: key reused with different request")
			return sendError(c, fiber.StatusConflict, "idempotency_conflict", "idempotency key reused with a different request")
		}
		if isTimeoutError(err) {
			return sendError(c, fiber.StatusGatewayTimeout, "timeout", "request timed out while waiting for an identical send")
		}
		log.Error().Err(err).Msg("send_failed: idempotency coordination failed")
		return sendError(c, fiber.StatusInternalServerError, "send_failed", "idempotency coordination failed")
	}
	if outcome == idempotency.OutcomeCached || outcome == idempotency.OutcomeShared {
		log.Debug().Str("to", req.To).Str("outcome", string(outcome)).
			Bool("cached_ok", result.Response.OK).Str("message_id", result.Response.MessageID).
			Msg("send idempotent hit")
	}
	return writeSendResult(c, result)
}

func sendOnce(ctx context.Context, req provider.HTTPSendRequest, dingtalkClient *dingtalk.Client, log *logger.Logger) idempotency.Result {
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
		resolved, err := dingtalkClient.GetUserIDByMobile(ctx, req.To)
		if err != nil {
			if isTimeoutError(err) {
				log.Warn().Err(err).Str("to", req.To).Msg("send timeout: mobile lookup timed out")
				return failureResult(fiber.StatusGatewayTimeout, "timeout", "dingtalk request timed out")
			}
			log.Warn().Err(err).Str("to", req.To).Msg("send invalid_destination: mobile lookup failed")
			return failureResult(fiber.StatusBadRequest, "invalid_destination", "mobile lookup failed: "+err.Error())
		}
		destUserID = resolved
		log.Debug().Str("mobile", req.To).Str("userid", destUserID).Msg("send: resolved mobile to userid")
	}

	taskID, err := dingtalkClient.SendWorkNotify(ctx, destUserID, content)
	if err != nil {
		log.Warn().Err(err).Str("to", destUserID).Msg("send_failed: dingtalk API error")
		if isTimeoutError(err) {
			return failureResult(fiber.StatusGatewayTimeout, "timeout", "dingtalk request timed out")
		}
		return failureResult(fiber.StatusBadGateway, "send_failed", err.Error())
	}

	log.Info().Str("to", req.To).Str("message_id", taskID).Msg("send ok")
	return idempotency.Result{
		StatusCode: fiber.StatusOK,
		Response: provider.HTTPSendResponse{
			OK: true, MessageID: taskID, Provider: provider.ChannelDingTalk.String(),
		},
	}
}

func requestFingerprint(req provider.HTTPSendRequest) (string, error) {
	payload, err := json.Marshal(fingerprintPayload{
		Channel: req.Channel, To: req.To, Template: req.Template, Params: req.Params,
		Subject: req.Subject, Body: req.Body, Locale: req.Locale,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum), nil
}

func isTimeoutError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func failureResult(status int, code, message string) idempotency.Result {
	return idempotency.Result{
		StatusCode: status,
		Response: provider.HTTPSendResponse{
			OK: false, ErrorCode: code, ErrorMessage: message,
		},
	}
}

func writeSendResult(c *fiber.Ctx, result idempotency.Result) error {
	return c.Status(result.StatusCode).JSON(result.Response)
}

func sendError(c *fiber.Ctx, status int, code, message string) error {
	return writeSendResult(c, failureResult(status, code, message))
}
