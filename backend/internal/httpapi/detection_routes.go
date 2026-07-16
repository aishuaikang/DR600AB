package httpapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/model"
)

// registerDetectionRoutes 挂载串口会话、事件流和记录接口。
func (s *Server) registerDetectionRoutes(api fiber.Router) {
	s.registerDetectionRecoveryRoutes(api)
	s.registerDetectionSessionRoutes(api)
	s.registerDetectionRecordRoutes(api)
}

// registerDetectionRecoveryRoutes 挂载授权恢复所需的串口配置接口。
func (s *Server) registerDetectionRecoveryRoutes(api fiber.Router) {
	api.Get("/serial/ports", s.handlePorts)
	api.Get("/detection/settings", s.handleDetectionSettings)
	api.Get("/detection/session", s.handleCurrentSession)
	api.Put("/detection/settings", s.handleUpdateDetectionSettings)
}

// registerDetectionSessionRoutes 挂载授权后可用的设备会话配置接口。
func (s *Server) registerDetectionSessionRoutes(api fiber.Router) {
	api.Get("/gps/settings", s.handleGPSSettings)
	api.Put("/gps/settings", s.handleUpdateGPSSettings)
}

func (s *Server) registerDetectionRecordRoutes(api fiber.Router) {
	api.Get("/detection/stream", s.handleStream)
	api.Get("/detection/records", s.handleDetectionRecords)
	api.Post("/detection/commands", s.handleSendDetectionCommand)
	api.Get("/parsed/records", s.handleParsedRecords)
}

// registerGPSReadRoutes 挂载普通授权用户可访问的 GPS 只读接口。
func (s *Server) registerGPSReadRoutes(api fiber.Router) {
	api.Get("/gps/session", s.handleCurrentGPSSession)
	api.Get("/gps/records", s.handleGPSRecords)
	api.Get("/gps/stream", s.handleGPSStream)
}

// handleCurrentGPSSession 返回当前 GPS 会话响应。
func (s *Server) handleCurrentGPSSession(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	return c.JSON(s.gps.Current(locale))
}

// handleGPSSettings 在存在持久化设置时返回 GPS 设置。
func (s *Server) handleGPSSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloper(c, locale) {
		return nil
	}
	settings, ok, err := s.gps.Settings()
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

// handleUpdateGPSSettings 校验设置，并启动或更新 GPS 会话。
func (s *Server) handleUpdateGPSSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloper(c, locale) {
		return nil
	}
	var req model.GPSSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if strings.TrimSpace(req.DataPortName) == "" && strings.TrimSpace(req.PortName) == "" {
		response, err := s.gps.ClearSettings(locale)
		if err != nil {
			return s.respondError(c, fiber.StatusInternalServerError, "internal", err.Error(), nil)
		}
		return c.JSON(response)
	}

	response, err := s.gps.Start(req, locale)
	if err != nil {
		code := "gps_port_open_failed"
		status := fiber.StatusBadRequest
		if strings.HasPrefix(err.Error(), s.translator.T(locale, "errors", "internal")) {
			code = "internal"
			status = fiber.StatusInternalServerError
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}

// handleGPSRecords 返回最新 GPS NMEA 记录。
func (s *Server) handleGPSRecords(c *fiber.Ctx) error {
	items := s.gps.Records(parseLimit(c, 100))
	return c.JSON(model.ListResponse[model.GPSRecord]{
		Items: items,
		Count: len(items),
	})
}

// handlePorts 返回可用串口和当前会话状态。
func (s *Server) handlePorts(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloperOrLicenseRecovery(c, locale) {
		return nil
	}
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
	gpsSession := s.gps.Current(locale)
	if gpsSession.Active {
		for index := range ports {
			if ports[index].Name == gpsSession.DataPortName || ports[index].Name == gpsSession.ControlPortName {
				ports[index].Active = true
			}
		}
	}
	deceptionSession := s.deception.Current(locale)
	if deceptionSession.Active {
		for index := range ports {
			if ports[index].Name == deceptionSession.PortName {
				ports[index].Active = true
			}
		}
	}
	compassSession := s.compass.Current(locale)
	if compassSession.Active {
		for index := range ports {
			if ports[index].Name == compassSession.PortName {
				ports[index].Active = true
			}
		}
	}
	return c.JSON(fiber.Map{
		"ports":         ports,
		"activeSession": s.detection.Current(locale),
	})
}

// handleCurrentSession 返回当前侦测会话响应。
func (s *Server) handleCurrentSession(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloperOrLicenseRecovery(c, locale) {
		return nil
	}
	return c.JSON(s.detection.Current(locale))
}

// handleDetectionSettings 在存在持久化设置时返回侦测设置。
func (s *Server) handleDetectionSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloperOrLicenseRecovery(c, locale) {
		return nil
	}
	settings, ok, err := s.detection.Settings()
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

// handleUpdateDetectionSettings 校验设置，并启动或更新侦测会话。
func (s *Server) handleUpdateDetectionSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloperOrLicenseRecovery(c, locale) {
		return nil
	}
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
		response, err := s.detection.ClearSettings(locale)
		if err != nil {
			return s.respondError(c, fiber.StatusInternalServerError, "internal", err.Error(), nil)
		}
		return c.JSON(response)
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

// handleDetectionRecords 返回标准化侦测列表行。
func (s *Server) handleDetectionRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloper(c, locale) {
		return nil
	}
	items := s.detection.Records(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.DetectionRecord]{
		Items: items,
		Count: len(items),
	})
}

// handleSendDetectionCommand 向当前侦测 TX 串口发送一条调试命令。
func (s *Server) handleSendDetectionCommand(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloper(c, locale) {
		return nil
	}

	var req model.DetectionCommandRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	command := strings.TrimSpace(req.Command)
	if command == "" {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			nil,
		)
	}
	if strings.ContainsAny(command, "\r\n") {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			"命令必须是单行文本",
		)
	}

	if err := s.detection.SendCommands(command); err != nil {
		switch {
		case errors.Is(err, detection.ErrCommandModeConflict):
			return s.respondError(c, fiber.StatusConflict, "command_mode_busy", "设备控制命令正在执行，请先停止当前模式", nil)
		case errors.Is(err, detection.ErrCommandSerialOffline):
			return s.respondError(c, fiber.StatusServiceUnavailable, "detection_serial_offline", "侦测串口未连接", nil)
		default:
			return s.respondError(c, fiber.StatusInternalServerError, "detection_command_failed", "侦测调试命令发送失败", err.Error())
		}
	}

	return c.JSON(model.DetectionCommandResponse{
		Command: command,
		Message: s.translator.T(locale, "common", "detection.command_sent"),
	})
}

// handleParsedRecords 返回原始解析结果行。
func (s *Server) handleParsedRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if !s.requireDeveloper(c, locale) {
		return nil
	}
	items := s.detection.Parsed(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.ParsedMessage]{
		Items: items,
		Count: len(items),
	})
}
