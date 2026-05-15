package httpapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/model"
)

// registerDeveloperRoutes 挂载短时开发者登录接口。
func (s *Server) registerDeveloperRoutes(api fiber.Router) {
	api.Post("/developer/session", s.handleDeveloperLogin)
	api.Delete("/developer/session", s.handleDeveloperLogout)
}

// handleDeveloperLogin 使用动态码换取短时开发者会话。
func (s *Server) handleDeveloperLogin(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.DeveloperLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	token, expiresAt, err := s.developer.Login(req.Code)
	if err != nil {
		status := fiber.StatusInternalServerError
		code := "internal"
		message := s.translator.T(locale, "errors", "internal")
		if errors.Is(err, developer.ErrNotConfigured) {
			status = fiber.StatusServiceUnavailable
			code = "developer_not_configured"
			message = s.translator.T(locale, "errors", "developer_not_configured")
		}
		if errors.Is(err, developer.ErrInvalidCode) {
			status = fiber.StatusUnauthorized
			code = "developer_invalid_code"
			message = s.translator.T(locale, "errors", "developer_invalid_code")
		}
		return s.respondError(c, status, code, message, nil)
	}

	return c.JSON(model.DeveloperSessionResponse{
		Token:     token,
		ExpiresAt: expiresAt.UnixMilli(),
		Message:   s.translator.T(locale, "common", "developer.login"),
	})
}

// handleDeveloperLogout 删除当前开发者会话。
func (s *Server) handleDeveloperLogout(c *fiber.Ctx) error {
	s.developer.Logout(strings.TrimSpace(c.Get("X-Developer-Token")))
	return c.SendStatus(fiber.StatusNoContent)
}
