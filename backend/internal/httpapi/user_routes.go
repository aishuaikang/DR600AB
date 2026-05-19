package httpapi

import (
	"math"

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
	if s.userSettings == nil {
		return c.JSON(req)
	}
	if err := s.userSettings.SaveUser(req); err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(req)
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
