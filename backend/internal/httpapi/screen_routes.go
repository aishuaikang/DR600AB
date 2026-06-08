package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/fpv"
	"dr600ab-api/internal/fpvrecord"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/model"
)

const fpvVideoHeartbeatInterval = time.Second

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
	api.Get("/screen/fpv/video", s.handleScreenFPVVideo)
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

// handleScreenFPVVideo starts a single FPV playback session and streams frames.
func (s *Server) handleScreenFPVVideo(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	frequency, err := parseFPVFrequency(c)
	if err != nil {
		return s.respondError(c, fiber.StatusBadRequest, "invalid_request", "频点参数无效", err.Error())
	}
	if s.fpv == nil {
		return s.respondError(c, fiber.StatusInternalServerError, "internal", s.translator.T(locale, "errors", "internal"), nil)
	}

	playback, err := s.fpv.BeginPlayback(frequency)
	if err != nil {
		if errors.Is(err, fpv.ErrPlaybackBusy) {
			return s.respondError(c, fiber.StatusConflict, "fpv_video_busy", "FPV 图传正在播放", nil)
		}
		return s.respondError(c, fiber.StatusBadRequest, "invalid_request", "频点参数无效", err.Error())
	}

	if err := s.startFPVPlayback(playback); err != nil {
		s.fpv.EndPlayback(playback)
		if errors.Is(err, detection.ErrCommandSerialOffline) {
			return s.respondError(c, fiber.StatusServiceUnavailable, "detection_serial_offline", "侦测串口未连接", nil)
		}
		return s.respondError(c, fiber.StatusInternalServerError, "fpv_control_failed", "图传控制命令发送失败", err.Error())
	}

	sessionRecord := s.startFPVVideoRecord(c.Query("targetId"), playback.Frequency)
	serverDone := c.Context().Done()
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			status := s.fpv.Snapshot()
			frame := s.fpv.LastFrame()
			if err := s.stopFPVPlayback(); err != nil {
				fmt.Printf("停止 FPV 图传命令失败: %v\n", err)
			}
			s.fpv.EndPlayback(playback)
			s.finishFPVVideoRecord(sessionRecord, status, frame)
		}()

		events, unsubscribe := s.fpv.Subscribe(s.cfg.EventBufferSize)
		defer unsubscribe()

		_ = writeComment(w, "connected")
		_ = writeJSONSSE(w, "status", s.fpv.Snapshot())
		if frame := s.fpv.LastFrame(); frame != nil {
			_ = writeJSONSSE(w, "frame", frame)
		}

		ticker := time.NewTicker(fpvVideoHeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case message, ok := <-events:
				if !ok {
					return
				}
				if err := writeSSEPayload(w, message.Name, message.Data); err != nil {
					return
				}
			case <-ticker.C:
				if err := writeComment(w, "ping"); err != nil {
					return
				}
			// fasthttp RequestCtx.Done is a server shutdown signal; client
			// disconnects are detected by Flush errors from frame/ping writes.
			case <-serverDone:
				return
			}
		}
	})
	return nil
}

func (s *Server) startFPVVideoRecord(targetID string, frequency float64) model.FPVVideoRecord {
	record := model.FPVVideoRecord{
		ID:        fpvrecord.NewRecordID(),
		Frequency: frequency,
		StartedAt: time.Now(),
		Status:    model.FPVVideoRecordStatusCompleted,
		CreatedAt: time.Now(),
	}
	targetID = strings.TrimSpace(targetID)
	if s.detection == nil {
		return record
	}
	target := s.lookupFPVVideoTarget(targetID, frequency)
	if target.ID == "" {
		return record
	}
	record.TargetID = target.ID
	record.Serial = target.Serial
	record.Model = target.Model
	record.DisplayModel = target.DisplayModel
	if record.DisplayModel == "" {
		record.DisplayModel = model.DisplayModelName(target.Model)
	}
	record.Device = target.Device
	record.Frequency = target.Frequency
	record.RSSI = target.RSSI
	record.LastRecord = target.LastRecord
	return record
}

func (s *Server) lookupFPVVideoTarget(targetID string, frequency float64) model.ScreenDetectionTarget {
	targets := s.detection.ScreenDetections(200)
	if targetID != "" {
		for _, target := range targets {
			if target.ID == targetID {
				return target
			}
		}
	}
	if frequency <= 0 {
		return model.ScreenDetectionTarget{}
	}
	roundedFrequency := int(math.Round(frequency))
	for _, target := range targets {
		if int(math.Round(target.Frequency)) == roundedFrequency {
			return target
		}
	}
	return model.ScreenDetectionTarget{}
}

func (s *Server) finishFPVVideoRecord(record model.FPVVideoRecord, status fpv.Status, frame *fpv.Frame) {
	if s.fpvRecords == nil || strings.TrimSpace(record.ID) == "" || record.StartedAt.IsZero() {
		return
	}
	record.EndedAt = time.Now()
	record.FrameCount = status.FrameCount
	if frame != nil {
		record.LastFrameRows = frame.Rows
		record.LastFrameCols = frame.Cols
		if frame.Num > record.FrameCount {
			record.FrameCount = frame.Num
		}
		if receivedAt := parseFPVFrameTime(frame.ReceivedAt); receivedAt != nil {
			record.LastFrameAt = receivedAt
		}
	}
	if err := s.fpvRecords.Insert(record); err != nil {
		fmt.Printf("写入 FPV 图传记录失败: %v\n", err)
	}
}

func (s *Server) startFPVPlayback(playback fpv.Playback) error {
	if err := s.detection.SendCommands(fmt.Sprintf("start -imag %s\r\n", s.fpv.Address())); err != nil {
		return err
	}
	if err := s.detection.SendCommands(fmt.Sprintf("start -band %d,%d\r\n", playback.BandStart, playback.BandEnd)); err != nil {
		if stopErr := s.stopFPVPlayback(); stopErr != nil {
			return errors.Join(err, stopErr)
		}
		return err
	}
	return nil
}

func (s *Server) stopFPVPlayback() error {
	return s.detection.SendCommands(
		"start -imag 0\r\n",
		"start -freq 1\r\n",
	)
}

func parseFPVFrequency(c *fiber.Ctx) (float64, error) {
	raw := strings.TrimSpace(c.Query("frequency"))
	if raw == "" {
		return 0, fmt.Errorf("frequency is required")
	}
	frequency, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(frequency) || math.IsInf(frequency, 0) || frequency <= 0 {
		return 0, fmt.Errorf("invalid frequency: %q", raw)
	}
	return frequency, nil
}

func parseFPVFrameTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func writeJSONSSE(w *bufio.Writer, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeSSEPayload(w, event, payload)
}

// handleScreenPositions 返回大屏使用的合并定位目标列表。
func (s *Server) handleScreenPositions(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	deviceLocation, err := s.currentScreenDeviceLocation()
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	var location *model.ScreenDeviceLocationResponse
	if deviceLocation.Valid {
		location = &deviceLocation
	}
	items := s.detection.ScreenPositionsWithDeviceLocation(parseLimit(c, 100), location)
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
