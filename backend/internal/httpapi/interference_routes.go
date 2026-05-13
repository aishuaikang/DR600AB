package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// registerInterferenceRoutes 挂载 GPIO 通道接口。
func (s *Server) registerInterferenceRoutes(api fiber.Router) {
	api.Get("/interference/channels", s.handleInterferenceChannels)
	api.Post("/interference/channels/:id/state", s.handleSetChannelState)
}

// handleInterferenceChannels 返回全部已配置 GPIO 通道。
func (s *Server) handleInterferenceChannels(c *fiber.Ctx) error {
	channels := s.interference.ListChannels()
	return c.JSON(fiber.Map{
		"channels": channels,
		"count":    len(channels),
	})
}

// handleSetChannelState 启用或禁用一个 GPIO 输出通道。
func (s *Server) handleSetChannelState(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.GpioChannelStateRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	channel, err := s.interference.SetState(c.Params("id"), req.Enabled, locale)
	if err != nil {
		status := fiber.StatusInternalServerError
		code := "gpio_update_failed"
		if channel.Reserved {
			status = fiber.StatusConflict
			code = "channel_reserved"
		}
		if channel.ID == "" {
			status = fiber.StatusNotFound
			code = "channel_not_found"
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}

	messageKey := "gpio.disabled"
	if req.Enabled {
		messageKey = "gpio.enabled"
	}
	return c.JSON(model.GpioChannelStateResponse{
		Channel: channel,
		Message: s.translator.T(locale, "common", messageKey),
	})
}
