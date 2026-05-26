package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// registerCompassRoutes 挂载三维电子罗盘串口开发调试接口。
func (s *Server) registerCompassRoutes(api fiber.Router) {
	api.Get("/compass/session", s.handleCurrentCompassSession)
	api.Get("/compass/settings", s.handleCompassSettings)
	api.Put("/compass/settings", s.handleUpdateCompassSettings)
	api.Get("/compass/records", s.handleCompassRecords)
}

// handleCurrentCompassSession 返回当前三维电子罗盘串口会话状态。
func (s *Server) handleCurrentCompassSession(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	return c.JSON(s.compass.Current(locale))
}

// handleCompassSettings 在存在持久化设置时返回三维电子罗盘串口设置。
func (s *Server) handleCompassSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	settings, ok, err := s.compass.Settings()
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	if !ok {
		return c.JSON(fiber.Map{})
	}
	return c.JSON(settings)
}

// handleUpdateCompassSettings 校验设置，并启动或更新三维电子罗盘串口会话。
func (s *Server) handleUpdateCompassSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	var req model.CompassSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if strings.TrimSpace(req.PortName) == "" {
		response, err := s.compass.ClearSettings(locale)
		if err != nil {
			return s.respondError(c, fiber.StatusInternalServerError, "internal", err.Error(), nil)
		}
		return c.JSON(response)
	}

	response, err := s.compass.Start(req, locale)
	if err != nil {
		code := "compass_port_open_failed"
		status := fiber.StatusBadRequest
		if strings.HasPrefix(err.Error(), s.translator.T(locale, "errors", "internal")) {
			code = "internal"
			status = fiber.StatusInternalServerError
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}

// handleCompassRecords 返回三维电子罗盘角度记录。
func (s *Server) handleCompassRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	items := s.compass.Records(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.CompassRecord]{
		Items: items,
		Count: len(items),
	})
}
