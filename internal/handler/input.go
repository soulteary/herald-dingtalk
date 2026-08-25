package handler

import (
	"mime"
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"
)

const (
	maxDestinationLength = 256
	maxAuthCodeLength    = 4096
)

func hasJSONContentType(c *fiber.Ctx) bool {
	mediaType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	return err == nil && mediaType == fiber.MIMEApplicationJSON
}

func validBoundedToken(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || strings.TrimSpace(value) != value {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) == -1
}
