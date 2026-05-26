package httpapi

import (
	"math"
	"strings"
	"time"

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
		return c.JSON(model.UserSettingsWithDefaults(model.UserSettings{}))
	}
	return c.JSON(model.UserSettingsWithDefaults(settings))
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
	if !validIntrusionRetentionDays(req.IntrusionRetentionDays) {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			"invalid intrusion retention days",
		)
	}
	req.ScreenStrikeChannelLabels = normalizeScreenStrikeChannelLabels(req.ScreenStrikeChannelLabels)
	req.Whitelist = normalizeUserWhitelist(req.Whitelist, time.Now())
	req.DeviceSN = ""
	req.DeviceHardwareID = ""
	req = model.UserSettingsWithDefaults(req)
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
	saved = model.UserSettingsWithDefaults(saved)
	if err := s.pruneIntrusionsByUserSettings(saved); err != nil {
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
		point.Longitude <= 180 &&
		!(point.Latitude == 0 && point.Longitude == 0)
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

func normalizeUserWhitelist(items []model.WhitelistItem, now time.Time) []model.WhitelistItem {
	if len(items) == 0 {
		return nil
	}

	const maxItems = 500
	normalized := make([]model.WhitelistItem, 0, min(len(items), maxItems))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if len(normalized) == maxItems {
			break
		}
		serial := truncateRunes(strings.TrimSpace(item.Serial), 128)
		if serial == "" {
			continue
		}
		key := strings.ToLower(serial)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		createdAt := item.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		normalized = append(normalized, model.WhitelistItem{
			Serial:    serial,
			Model:     truncateRunes(strings.TrimSpace(item.Model), 64),
			Source:    truncateRunes(strings.TrimSpace(item.Source), 32),
			CreatedAt: createdAt,
		})
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func truncateRunes(value string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
}

func validIntrusionRetentionDays(days *int) bool {
	if days == nil {
		return true
	}
	return *days >= 0
}

const intrusionPruneInterval = time.Minute

func (s *Server) pruneIntrusionsByUserSettings(settings model.UserSettings) error {
	if s.intrusions == nil {
		return nil
	}
	days := model.UserSettingsIntrusionRetentionDays(settings)
	_, err := s.intrusions.PruneRetention(days, time.Now())
	return err
}

func (s *Server) pruneIntrusionsByCurrentUserSettings() error {
	if s == nil || s.intrusions == nil {
		return nil
	}
	settings := model.UserSettings{}
	if s.userSettings != nil {
		loaded, ok, err := s.userSettings.LoadUser()
		if err != nil {
			return err
		}
		if ok {
			settings = loaded
		}
	}
	return s.pruneIntrusionsByUserSettings(model.UserSettingsWithDefaults(settings))
}

func (s *Server) maybePruneIntrusionsByCurrentUserSettings() error {
	if s == nil || s.intrusions == nil {
		return nil
	}
	now := time.Now()
	s.intrusionPruneMu.Lock()
	if !s.lastIntrusionPruneRun.IsZero() && now.Sub(s.lastIntrusionPruneRun) < intrusionPruneInterval {
		s.intrusionPruneMu.Unlock()
		return nil
	}
	s.intrusionPruneMu.Unlock()

	if err := s.pruneIntrusionsByCurrentUserSettings(); err != nil {
		return err
	}

	s.intrusionPruneMu.Lock()
	s.lastIntrusionPruneRun = now
	s.intrusionPruneMu.Unlock()
	return nil
}
