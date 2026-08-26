package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
)

func mapDingTalkError(err error, invalidRequestCode string) (int, string, string) {
	failureCode := "send_failed"
	if invalidRequestCode != "" {
		failureCode = invalidRequestCode
	}
	switch dingtalk.ClassifyError(err) {
	case dingtalk.ErrorCategoryInvalidDestination:
		return fiber.StatusBadRequest, "invalid_destination", "dingtalk destination is not available"
	case dingtalk.ErrorCategoryInvalidRequest:
		if invalidRequestCode != "" {
			return fiber.StatusBadRequest, invalidRequestCode, "invalid or expired dingtalk authorization code"
		}
		return fiber.StatusBadGateway, failureCode, "dingtalk rejected the provider request"
	case dingtalk.ErrorCategoryRateLimited:
		return fiber.StatusTooManyRequests, "rate_limited", "dingtalk rate limit exceeded"
	case dingtalk.ErrorCategoryProviderDown:
		return fiber.StatusServiceUnavailable, "provider_down", "dingtalk credentials or service are unavailable"
	case dingtalk.ErrorCategoryTimeout:
		return fiber.StatusGatewayTimeout, "timeout", "dingtalk request timed out"
	default:
		return fiber.StatusBadGateway, failureCode, "dingtalk request failed"
	}
}
