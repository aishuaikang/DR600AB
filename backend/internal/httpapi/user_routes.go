package httpapi

import (
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// registerUserRoutes 挂载公开用户设置接口。
func (s *Server) registerUserRoutes(api fiber.Router) {
	api.Get("/user/settings", s.handleUserSettings)
	api.Put("/user/settings", s.handleUpdateUserSettings)
}

// handleUserSettings 返回公开用户设置。
func (s *Server) handleUserSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.userSettings == nil {
		return c.JSON(model.UserSettings{})
	}

	settings, ok, err := s.userSettings.LoadUser()
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
		return c.JSON(model.UserSettings{})
	}
	return c.JSON(settings)
}

// handleUpdateUserSettings 保存或清空公开用户设置。
func (s *Server) handleUpdateUserSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.UserSettings
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if req.ManualDeviceLocation != nil && !validGeoPoint(req.ManualDeviceLocation) {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"user_invalid_location",
			s.translator.T(locale, "errors", "user_invalid_location"),
			nil,
		)
	}
	req.ScreenStrikeChannelLabels = normalizeScreenStrikeChannelLabels(req.ScreenStrikeChannelLabels)
	req.DeviceSN = ""
	if s.userSettings == nil {
		return c.JSON(req)
	}
	saved, err := s.userSettings.SaveEditableUser(req)
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(saved)
}

func validGeoPoint(point *model.GeoPoint) bool {
	if point == nil {
		return false
	}
	latitudeInvalid := math.IsNaN(point.Latitude) || math.IsInf(point.Latitude, 0)
	longitudeInvalid := math.IsNaN(point.Longitude) || math.IsInf(point.Longitude, 0)
	return !latitudeInvalid &&
		!longitudeInvalid &&
		point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180
}

func normalizeScreenStrikeChannelLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	const maxLabels = 3
	const maxLabelLength = 24
	normalized := make([]string, 0, maxLabels)
	for _, label := range labels {
		if len(normalized) == maxLabels {
			break
		}
		value := strings.TrimSpace(label)
		if len([]rune(value)) > maxLabelLength {
			value = string([]rune(value)[:maxLabelLength])
		}
		normalized = append(normalized, value)
	}
	for len(normalized) > 0 && normalized[len(normalized)-1] == "" {
		normalized = normalized[:len(normalized)-1]
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
