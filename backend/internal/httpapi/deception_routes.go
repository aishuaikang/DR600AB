package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/model"
)

// registerDeceptionRoutes 挂载 GNSS 诱骗设备开发调试接口。
func (s *Server) registerDeceptionRoutes(api fiber.Router) {
	api.Get("/deception/session", s.handleCurrentDeceptionSession)
	api.Get("/deception/settings", s.handleDeceptionSettings)
	api.Put("/deception/settings", s.handleUpdateDeceptionSettings)
	api.Post("/deception/session/start", s.handleStartDeceptionSession)
	api.Post("/deception/session/stop", s.handleStopDeceptionSession)
	api.Get("/deception/records", s.handleDeceptionRecords)
	api.Get("/deception/query/:item", s.handleDeceptionQuery)
}

// handleCurrentDeceptionSession 返回当前 GNSS 诱骗设备串口会话状态。
func (s *Server) handleCurrentDeceptionSession(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	return c.JSON(s.deception.Current(locale))
}

// handleDeceptionSettings 在存在持久化设置时返回 GNSS 诱骗设备串口设置。
func (s *Server) handleDeceptionSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	settings, ok, err := s.deception.Settings()
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

// handleUpdateDeceptionSettings 校验设置，并启动或更新 GNSS 诱骗设备串口会话。
func (s *Server) handleUpdateDeceptionSettings(c *fiber.Ctx) error {
	return s.startDeceptionFromBody(c)
}

// handleStartDeceptionSession 启动 GNSS 诱骗设备串口会话。
func (s *Server) handleStartDeceptionSession(c *fiber.Ctx) error {
	return s.startDeceptionFromBody(c)
}

func (s *Server) startDeceptionFromBody(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}

	var req model.DeceptionSessionRequest
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
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"deception_port_required",
			s.translator.T(locale, "errors", "deception_port_required"),
			nil,
		)
	}

	response, err := s.deception.Start(req, locale)
	if err != nil {
		code := "deception_port_open_failed"
		status := fiber.StatusBadRequest
		if strings.HasPrefix(err.Error(), s.translator.T(locale, "errors", "internal")) {
			code = "internal"
			status = fiber.StatusInternalServerError
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}

// handleStopDeceptionSession 停止 GNSS 诱骗设备串口会话。
func (s *Server) handleStopDeceptionSession(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	return c.JSON(s.deception.Stop(locale))
}

// handleDeceptionRecords 返回 GNSS 诱骗设备协议交互记录。
func (s *Server) handleDeceptionRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	items := s.deception.Records(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.DeceptionRecord]{
		Items: items,
		Count: len(items),
	})
}

// handleDeceptionQuery 查询 GNSS 诱骗设备状态项。
func (s *Server) handleDeceptionQuery(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}
	response, err := s.deception.Query(c.Params("item"), locale)
	if err != nil {
		code := "deception_query_failed"
		status := fiber.StatusInternalServerError
		if errorCode := deception.ErrorCode(err); errorCode != "" {
			code = errorCode
			status = fiber.StatusBadRequest
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}
