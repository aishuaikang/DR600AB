package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// registerDetectionRoutes 挂载串口会话、事件流和记录接口。
func (s *Server) registerDetectionRoutes(api fiber.Router) {
	api.Get("/serial/ports", s.handlePorts)
	api.Get("/detection/settings", s.handleDetectionSettings)
	api.Get("/detection/session", s.handleCurrentSession)
	api.Post("/detection/session", s.handleStartSession)
	api.Put("/detection/settings", s.handleUpdateDetectionSettings)
	api.Delete("/detection/session", s.handleStopSession)
	api.Get("/detection/stream", s.handleStream)
	api.Get("/detection/records", s.handleDetectionRecords)
	api.Get("/parsed/records", s.handleParsedRecords)
	api.Get("/fpv/records", s.handleFPVRecords)
}

// handlePorts 返回可用串口和当前会话状态。
func (s *Server) handlePorts(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	ports, err := s.detection.ListPorts()
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(fiber.Map{
		"ports":         ports,
		"activeSession": s.detection.Current(locale),
	})
}

// handleCurrentSession 返回当前侦测会话响应。
func (s *Server) handleCurrentSession(c *fiber.Ctx) error {
	return c.JSON(s.detection.Current(s.resolveLocale(c)))
}

// handleStartSession 使用与设置更新相同的请求体启动侦测。
func (s *Server) handleStartSession(c *fiber.Ctx) error {
	return s.handleUpdateDetectionSettings(c)
}

// handleDetectionSettings 在存在持久化设置时返回侦测设置。
func (s *Server) handleDetectionSettings(c *fiber.Ctx) error {
	settings, ok, err := s.detection.Settings()
	if err != nil {
		locale := s.resolveLocale(c)
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

// handleUpdateDetectionSettings 校验设置，并启动或更新侦测会话。
func (s *Server) handleUpdateDetectionSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.DetectionSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if strings.TrimSpace(req.RxPortName) == "" && strings.TrimSpace(req.PortName) == "" {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"port_required",
			s.translator.T(locale, "errors", "port_required"),
			nil,
		)
	}

	response, err := s.detection.Start(req, locale)
	if err != nil {
		code := "port_open_failed"
		status := fiber.StatusBadRequest
		if strings.HasPrefix(err.Error(), s.translator.T(locale, "errors", "internal")) {
			code = "internal"
			status = fiber.StatusInternalServerError
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}

// handleStopSession 停止当前侦测会话。
func (s *Server) handleStopSession(c *fiber.Ctx) error {
	return c.JSON(s.detection.Stop(s.resolveLocale(c)))
}

// handleDetectionRecords 返回标准化侦测列表行。
func (s *Server) handleDetectionRecords(c *fiber.Ctx) error {
	items := s.detection.Records(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.DetectionRecord]{
		Items: items,
		Count: len(items),
	})
}

// handleParsedRecords 返回原始解析结果行。
func (s *Server) handleParsedRecords(c *fiber.Ctx) error {
	items := s.detection.Parsed(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.ParsedMessage]{
		Items: items,
		Count: len(items),
	})
}

// handleFPVRecords 返回已归类到图传频段的侦测记录。
func (s *Server) handleFPVRecords(c *fiber.Ctx) error {
	items := s.detection.FPV(parseLimit(c, 100))
	return c.JSON(model.ListResponse[model.FpvRecord]{
		Items: items,
		Count: len(items),
	})
}
