package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit/v2"
	"github.com/soulteary/provider-kit"
)

type sendRequestFailure struct {
	status  int
	code    string
	message string
}

func (failure *sendRequestFailure) write(c fiber.Ctx) error {
	return sendError(c, failure.status, failure.code, failure.message)
}

func validateSendRequest(c fiber.Ctx, log *logger.Logger) (provider.HTTPSendRequest, *sendRequestFailure) {
	if !hasJSONContentType(c) {
		log.Warn().Msg("send unsupported_media_type: application/json required")
		return provider.HTTPSendRequest{}, &sendRequestFailure{
			status: fiber.StatusUnsupportedMediaType, code: "unsupported_media_type", message: "Content-Type must be application/json",
		}
	}

	var req provider.HTTPSendRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Err(err).Msg("send invalid_request: body parse error")
		return provider.HTTPSendRequest{}, &sendRequestFailure{status: fiber.StatusBadRequest, code: "invalid_request", message: err.Error()}
	}
	if req.Channel != "" && req.Channel != provider.ChannelDingTalk.String() {
		log.Warn().Str("channel", req.Channel).Msg("send invalid_request: unsupported channel")
		return provider.HTTPSendRequest{}, &sendRequestFailure{status: fiber.StatusBadRequest, code: "invalid_request", message: "channel must be dingtalk"}
	}
	req.Channel = provider.ChannelDingTalk.String()
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > maxRequestTimeoutSeconds {
		log.Warn().Int("timeout_seconds", req.TimeoutSeconds).Msg("send invalid_request: timeout out of range")
		return provider.HTTPSendRequest{}, &sendRequestFailure{status: fiber.StatusBadRequest, code: "invalid_request", message: "timeout_seconds must be between 0 and 30"}
	}
	if !validBoundedToken(req.To, maxDestinationLength) {
		log.Warn().Int("destination_length", len(req.To)).Msg("send invalid_destination: invalid to value")
		return provider.HTTPSendRequest{}, &sendRequestFailure{
			status: fiber.StatusBadRequest, code: "invalid_destination", message: "to must be 1-256 bytes without surrounding whitespace or control characters",
		}
	}
	return req, nil
}

func parseIdempotencyKey(c fiber.Ctx, req *provider.HTTPSendRequest, log *logger.Logger) *sendRequestFailure {
	headerKey := c.Get("Idempotency-Key")
	if req.IdempotencyKey != "" && headerKey != "" && req.IdempotencyKey != headerKey {
		log.Warn().Msg("send idempotency_conflict: header and body keys differ")
		return &sendRequestFailure{status: fiber.StatusConflict, code: "idempotency_conflict", message: "header and body idempotency keys differ"}
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = headerKey
	}
	if len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		log.Warn().Int("key_length", len(req.IdempotencyKey)).Msg("send invalid_request: idempotency key too long")
		return &sendRequestFailure{status: fiber.StatusBadRequest, code: "invalid_request", message: "idempotency key must not exceed 256 bytes"}
	}
	if req.IdempotencyKey != "" && !validBoundedToken(req.IdempotencyKey, maxIdempotencyKeyLength) {
		log.Warn().Msg("send invalid_request: invalid idempotency key")
		return &sendRequestFailure{
			status: fiber.StatusBadRequest, code: "invalid_request", message: "idempotency key must not contain surrounding whitespace or control characters",
		}
	}
	return nil
}

type sendContexts struct {
	request          context.Context
	cancel           context.CancelFunc
	operationBase    context.Context
	operationTimeout time.Duration
}

func establishSendContexts(c fiber.Ctx, req provider.HTTPSendRequest) sendContexts {
	requestCtx, cancel := requestContext(c, req.TimeoutSeconds)
	operationBase := c.Context()
	if req.IdempotencyKey != "" {
		// The shared operation must not inherit the first caller's cancellation:
		// another caller may still be waiting on the same idempotency key.
		operationBase = context.WithoutCancel(operationBase)
	}
	return sendContexts{
		request: requestCtx, cancel: cancel, operationBase: operationBase, operationTimeout: requestTimeout(req.TimeoutSeconds),
	}
}

func (contexts sendContexts) operation() (context.Context, context.CancelFunc) {
	return context.WithTimeout(contexts.operationBase, contexts.operationTimeout)
}

func mapSendOutcome(c fiber.Ctx, result idempotency.Result, outcome idempotency.Outcome, err error, log *logger.Logger) error {
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
		log.Debug().Str("outcome", string(outcome)).
			Bool("cached_ok", result.Response.OK).Str("message_id", result.Response.MessageID).
			Msg("send idempotent hit")
	}
	return writeSendResult(c, result)
}
