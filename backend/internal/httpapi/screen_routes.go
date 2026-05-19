package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// registerScreenRoutes 挂载大屏公开只读接口。
func (s *Server) registerScreenRoutes(api fiber.Router) {
	api.Get("/screen/detections", s.handleScreenDetections)
	api.Get("/screen/positions", s.handleScreenPositions)
	api.Get("/screen/device-location", s.handleScreenDeviceLocation)
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
