package httpapi

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// respondError 写入通用 JSON API 错误响应。
func (s *Server) respondError(
	c *fiber.Ctx,
	status int,
	code string,
	message string,
	details any,
) error {
	return c.Status(status).JSON(model.ApiError{
		Code:    code,
		Message: message,
		Details: details,
	})
}

// resolveLocale 从查询参数、请求头、Accept-Language 或默认值中选择语言。
func (s *Server) resolveLocale(c *fiber.Ctx) string {
	if locale := c.Query("locale"); locale != "" {
		return s.translator.Normalize(locale)
	}
	if locale := c.Get("X-Locale"); locale != "" {
		return s.translator.Normalize(locale)
	}
	if accept := c.Get(fiber.HeaderAcceptLanguage); accept != "" {
		for _, part := range strings.Split(accept, ",") {
			tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
			if tag != "" {
				return s.translator.Normalize(tag)
			}
		}
	}
	return s.translator.Normalize("")
}

// parseLimit 读取正数 limit 查询参数，失败时使用回退值。
func parseLimit(c *fiber.Ctx, fallback int) int {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}
