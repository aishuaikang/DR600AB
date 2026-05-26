package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/model"
)

// registerScreenRoutes 挂载大屏公开接口。
func (s *Server) registerScreenRoutes(api fiber.Router) {
	api.Get("/screen/status", s.handleScreenStatus)
	api.Get("/screen/detections", s.handleScreenDetections)
	api.Get("/screen/positions", s.handleScreenPositions)
	api.Get("/screen/device-location", s.handleScreenDeviceLocation)
	api.Get("/screen/strike", s.handleScreenStrike)
	api.Post("/screen/strike", s.handleSetScreenStrike)
	api.Get("/screen/deception", s.handleScreenDeception)
	api.Post("/screen/deception", s.handleSetScreenDeception)
	api.Get("/screen/deception/status", s.handleScreenDeceptionStatus)
	api.Get("/screen/stream", s.handleScreenStream)
}

// handleScreenStatus 返回大屏依赖的串口能力配置和运行状态。
func (s *Server) handleScreenStatus(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	status, err := s.screenStatus(locale)
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(status)
}

// handleScreenDetections 返回大屏使用的合并侦测目标列表。
func (s *Server) handleScreenDetections(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	items := s.detection.ScreenDetections(parseLimit(c, 100))
	if err := s.maybePruneIntrusionsByCurrentUserSettings(); err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.ListResponse[model.ScreenDetectionTarget]{
		Items: items,
		Count: len(items),
	})
}

// handleScreenPositions 返回大屏使用的合并定位目标列表。
func (s *Server) handleScreenPositions(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	items := s.detection.ScreenPositions(parseLimit(c, 100))
	if err := s.maybePruneIntrusionsByCurrentUserSettings(); err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.ListResponse[model.ScreenPositionTarget]{
		Items: items,
		Count: len(items),
	})
}

// handleScreenDeviceLocation 返回大屏地图使用的设备位置。
func (s *Server) handleScreenDeviceLocation(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	response, err := s.currentScreenDeviceLocation()
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(response)
}

// handleScreenStrike 返回大屏干扰控制状态。
func (s *Server) handleScreenStrike(c *fiber.Ctx) error {
	return c.JSON(s.interference.ScreenStrikeState())
}

// handleSetScreenStrike 更新大屏干扰控制状态。
func (s *Server) handleSetScreenStrike(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

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

// handleScreenDeception 返回大屏诱骗控制状态。
func (s *Server) handleScreenDeception(c *fiber.Ctx) error {
	return c.JSON(s.deception.ScreenState())
}

// handleScreenDeceptionStatus 返回大屏诱骗设备完整只读状态。
func (s *Server) handleScreenDeceptionStatus(c *fiber.Ctx) error {
	return c.JSON(s.deception.ScreenDeviceStatus(s.resolveLocale(c)))
}

// handleSetScreenDeception 更新大屏诱骗控制状态。
func (s *Server) handleSetScreenDeception(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.ScreenDeceptionRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	devicePoint, deviceAltitudeM, hasDevicePoint, err := s.screenDeviceLocation()
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	state, err := s.deception.SetScreenDeception(req, devicePoint, deviceAltitudeM, hasDevicePoint, locale)
	if err != nil {
		code := deception.ErrorCode(err)
		if code != "" {
			return s.respondError(c, fiber.StatusBadRequest, code, err.Error(), nil)
		}
		return s.respondError(c, fiber.StatusInternalServerError, "deception_control_failed", err.Error(), nil)
	}

	messageKey := "deception.stopped"
	if req.Enabled {
		messageKey = "deception.started"
	}
	return c.JSON(model.ScreenDeceptionResponse{
		State:   state,
		Message: s.translator.T(locale, "common", messageKey),
	})
}

func (s *Server) screenDeviceLocation() (model.GeoPoint, float64, bool, error) {
	response, err := s.currentScreenDeviceLocation()
	if err != nil || !response.Valid || response.Point == nil {
		return model.GeoPoint{}, 0, false, err
	}
	altitudeM := 0.0
	if s.gps != nil && response.Source == "gps" {
		if fix, _ := s.gps.LatestFix(); validGPSFix(fix) {
			altitudeM = fix.AltitudeM
		}
	}
	return *response.Point, altitudeM, true, nil
}

func (s *Server) currentScreenDeviceLocation() (model.ScreenDeviceLocationResponse, error) {
	userSettings := model.UserSettings{}
	if s.userSettings != nil {
		settings, ok, err := s.userSettings.LoadUser()
		if err != nil {
			return model.ScreenDeviceLocationResponse{}, err
		}
		if ok {
			userSettings = settings
		}
	}
	var fix *model.GPSFix
	var updatedAt *time.Time
	if s.gps != nil {
		fix, updatedAt = s.gps.LatestFix()
	}
	return screenDeviceLocationResponse(fix, updatedAt, userSettings), nil
}

func (s *Server) screenStatus(locale string) (model.ScreenRuntimeStatus, error) {
	detectionSettings, detectionConfigured, err := s.detection.Configured()
	if err != nil {
		return model.ScreenRuntimeStatus{}, err
	}
	deceptionSettings, deceptionConfigured, err := s.deception.Configured()
	if err != nil {
		return model.ScreenRuntimeStatus{}, err
	}
	compassSettings, compassConfigured, err := s.compass.Configured()
	if err != nil {
		return model.ScreenRuntimeStatus{}, err
	}

	detectionSession := s.detection.Current(locale)
	deceptionSession := s.deception.Current(locale)
	compassSession := s.compass.Current(locale)

	return model.ScreenRuntimeStatus{
		Detection: screenDetectionCapabilityStatus(
			detectionSettings,
			detectionConfigured,
			detectionSession,
		),
		Deception: screenDeceptionCapabilityStatus(
			deceptionSettings,
			deceptionConfigured,
			deceptionSession,
		),
		Compass: screenCompassCapabilityStatus(
			compassSettings,
			compassConfigured,
			compassSession,
		),
	}, nil
}

func screenDetectionCapabilityStatus(
	settings model.DetectionSessionRequest,
	configured bool,
	session model.DetectionSessionResponse,
) model.ScreenSerialCapabilityStatus {
	hasSessionPort := session.PortName != "" || session.RxPortName != "" || session.TxPortName != ""
	status := model.ScreenSerialCapabilityStatus{
		Configured: configured || hasSessionPort,
		Active:     session.Active,
		State:      session.State,
		LastError:  session.LastError,
	}
	if status.Configured {
		status.PortName = settings.PortName
		status.RxPortName = settings.RxPortName
		status.TxPortName = settings.TxPortName
		if status.RxPortName == "" {
			status.RxPortName = settings.PortName
		}
		if status.TxPortName == "" {
			status.TxPortName = status.RxPortName
		}
	} else {
		status.State = "unconfigured"
	}
	if session.PortName != "" {
		status.PortName = session.PortName
	}
	if session.RxPortName != "" {
		status.RxPortName = session.RxPortName
	}
	if session.TxPortName != "" {
		status.TxPortName = session.TxPortName
	}
	return status
}

func screenDeceptionCapabilityStatus(
	settings model.DeceptionSessionRequest,
	configured bool,
	session model.DeceptionSessionResponse,
) model.ScreenSerialCapabilityStatus {
	status := model.ScreenSerialCapabilityStatus{
		Configured: configured || session.PortName != "",
		Active:     session.Active,
		State:      session.State,
		LastError:  session.LastError,
	}
	if status.Configured {
		status.PortName = settings.PortName
	} else {
		status.State = "unconfigured"
	}
	if session.PortName != "" {
		status.PortName = session.PortName
	}
	return status
}

func screenCompassCapabilityStatus(
	settings model.CompassSessionRequest,
	configured bool,
	session model.CompassSessionResponse,
) model.ScreenSerialCapabilityStatus {
	status := model.ScreenSerialCapabilityStatus{
		Configured: configured || session.PortName != "",
		Active:     session.Active,
		State:      session.State,
		LastError:  session.LastError,
	}
	if status.Configured {
		status.PortName = settings.PortName
	} else {
		status.State = "unconfigured"
	}
	if session.PortName != "" {
		status.PortName = session.PortName
	}
	status.HeadingDeg = cloneFloat64Ptr(session.LastHeading)
	status.HeadingUpdatedAt = cloneTimePtr(session.LastUpdatedAt)
	return status
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
