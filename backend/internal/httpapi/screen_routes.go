package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/model"
)

// registerScreenRoutes 挂载大屏公开接口。
func (s *Server) registerScreenRoutes(api fiber.Router) {
	api.Get("/screen/detections", s.handleScreenDetections)
	api.Get("/screen/positions", s.handleScreenPositions)
	api.Get("/screen/device-location", s.handleScreenDeviceLocation)
	api.Get("/screen/strike", s.handleScreenStrike)
	api.Post("/screen/strike", s.handleSetScreenStrike)
	api.Get("/screen/stream", s.handleScreenStream)
}

// handleScreenDetections 返回大屏使用的合并侦测目标列表。
func (s *Server) handleScreenDetections(c *fiber.Ctx) error {
	items := s.detection.ScreenDetections(parseLimit(c, 100))
	return c.JSON(model.ListResponse[model.ScreenDetectionTarget]{
		Items: items,
		Count: len(items),
	})
}

// handleScreenPositions 返回大屏使用的合并定位目标列表。
func (s *Server) handleScreenPositions(c *fiber.Ctx) error {
	items := s.detection.ScreenPositions(parseLimit(c, 100))
	return c.JSON(model.ListResponse[model.ScreenPositionTarget]{
		Items: items,
		Count: len(items),
	})
}

// handleScreenDeviceLocation 返回大屏地图使用的设备位置。
func (s *Server) handleScreenDeviceLocation(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	userSettings := model.UserSettings{}
	if s.userSettings != nil {
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
		if ok {
			userSettings = settings
		}
	}

	fix, updatedAt := s.gps.LatestFix()
	return c.JSON(screenDeviceLocationResponse(fix, updatedAt, userSettings))
}

// handleScreenStrike 返回大屏干扰控制状态。
func (s *Server) handleScreenStrike(c *fiber.Ctx) error {
	return c.JSON(s.interference.ScreenStrikeState())
}

// handleSetScreenStrike 更新大屏干扰控制状态。
func (s *Server) handleSetScreenStrike(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}

	var req model.ScreenStrikeRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	state, err := s.interference.SetScreenStrike(req, locale)
	if err != nil {
		code := interference.ErrorCode(err)
		if code != "" {
			return s.respondError(c, fiber.StatusBadRequest, code, err.Error(), nil)
		}
		return s.respondError(c, fiber.StatusInternalServerError, "gpio_update_failed", err.Error(), nil)
	}

	messageKey := "strike.stopped"
	if req.Enabled {
		messageKey = "strike.started"
	}
	return c.JSON(model.ScreenStrikeResponse{
		State:   state,
		Message: s.translator.T(locale, "common", messageKey),
	})
}

func screenDeviceLocationResponse(
	fix *model.GPSFix,
	gpsUpdatedAt *time.Time,
	userSettings model.UserSettings,
) model.ScreenDeviceLocationResponse {
	if validGPSFix(fix) {
		return model.ScreenDeviceLocationResponse{
			Source:    "gps",
			Point:     &model.GeoPoint{Latitude: fix.Latitude, Longitude: fix.Longitude},
			UpdatedAt: gpsUpdatedAt,
			Valid:     true,
		}
	}
	if validGeoPoint(userSettings.ManualDeviceLocation) {
		return model.ScreenDeviceLocationResponse{
			Source: "manual",
			Point:  userSettings.ManualDeviceLocation,
			Valid:  true,
		}
	}
	return model.ScreenDeviceLocationResponse{
		Source: "none",
		Valid:  false,
	}
}

func validGPSFix(fix *model.GPSFix) bool {
	return fix != nil &&
		fix.Valid &&
		validGeoPoint(&model.GeoPoint{Latitude: fix.Latitude, Longitude: fix.Longitude})
}
