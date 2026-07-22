package httpapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/systemtime"
)

type setSystemTimezoneRequest struct {
	Timezone string `json:"timezone"`
}

type setSystemNTPRequest struct {
	Enabled *bool `json:"enabled"`
}

type setSystemManualTimeRequest struct {
	DateTime string `json:"datetime"`
}

// registerSystemTimeRoutes 挂载本机时间、时区和 NTP 设置接口。
func (s *Server) registerSystemTimeRoutes(api fiber.Router) {
	api.Get("/system/time", s.handleSystemTimeInfo)
	api.Get("/system/timezones", s.handleSystemTimezones)
	api.Get("/system/timezone", s.handleSystemTimezone)
	api.Put("/system/timezone", s.handleSetSystemTimezone)
	api.Put("/system/time/ntp", s.handleSetSystemNTP)
	api.Put("/system/time/manual", s.handleSetSystemManualTime)
}

func (s *Server) handleSystemTimeInfo(c *fiber.Ctx) error {
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	info, err := s.systemTime.GetInfo(c.Context())
	if err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(info)
}

func (s *Server) handleSystemTimezones(c *fiber.Ctx) error {
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	zones, err := s.systemTime.ListTimezones(c.Context())
	if err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(zones)
}

func (s *Server) handleSystemTimezone(c *fiber.Ctx) error {
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	info, err := s.systemTime.GetInfo(c.Context())
	if err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(fiber.Map{
		"timezone":   info.Timezone,
		"utc_offset": info.UTCOffset,
	})
}

func (s *Server) handleSetSystemTimezone(c *fiber.Ctx) error {
	var req setSystemTimezoneRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Timezone) == "" {
		return s.respondError(c, fiber.StatusBadRequest, "system_time_invalid_timezone", s.systemTimeMessage(c, "system_time_invalid_timezone"), nil)
	}
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	if err := s.systemTime.SetTimezone(c.Context(), req.Timezone); err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(fiber.Map{"message": s.systemTimeMessage(c, "system_time_timezone_updated")})
}

func (s *Server) handleSetSystemNTP(c *fiber.Ctx) error {
	var req setSystemNTPRequest
	if err := c.BodyParser(&req); err != nil || req.Enabled == nil {
		return s.respondError(c, fiber.StatusBadRequest, "system_time_invalid_request", s.systemTimeMessage(c, "system_time_invalid_request"), nil)
	}
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	if err := s.systemTime.SetNTPEnabled(c.Context(), *req.Enabled); err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(fiber.Map{"message": s.systemTimeMessage(c, "system_time_ntp_updated")})
}

func (s *Server) handleSetSystemManualTime(c *fiber.Ctx) error {
	var req setSystemManualTimeRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.DateTime) == "" {
		return s.respondError(c, fiber.StatusBadRequest, "system_time_invalid_manual_time", s.systemTimeMessage(c, "system_time_invalid_manual_time"), nil)
	}
	if s.systemTime == nil {
		return s.respondSystemTimeError(c, errors.New("system time service unavailable"))
	}
	if err := s.systemTime.SetManualTime(c.Context(), req.DateTime); err != nil {
		return s.respondSystemTimeError(c, err)
	}
	return c.JSON(fiber.Map{"message": s.systemTimeMessage(c, "system_time_manual_updated")})
}

func (s *Server) respondSystemTimeError(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "system_time_operation_failed"
	messageKey := code
	switch {
	case errors.Is(err, systemtime.ErrUnsupported):
		status = fiber.StatusNotImplemented
		code = "system_time_unsupported"
		messageKey = code
	case errors.Is(err, systemtime.ErrInvalidTimezone):
		status = fiber.StatusBadRequest
		code = "system_time_invalid_timezone"
		messageKey = code
	case errors.Is(err, systemtime.ErrInvalidManualTime):
		status = fiber.StatusBadRequest
		code = "system_time_invalid_manual_time"
		messageKey = code
	}
	return s.respondError(c, status, code, s.systemTimeMessage(c, messageKey), err.Error())
}

func (s *Server) systemTimeMessage(c *fiber.Ctx, key string) string {
	return s.translator.T(s.resolveLocale(c), "errors", key)
}
