package httpapi

import (
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/model"
)

// registerIntrusionRoutes 挂载目标入侵历史接口。
func (s *Server) registerIntrusionRoutes(api fiber.Router) {
	api.Get("/intrusions", s.handleIntrusionRecords)
	api.Delete("/intrusions", s.handleDeleteIntrusionRecords)
}

// handleIntrusionRecords 返回已消失目标的入侵历史。
func (s *Server) handleIntrusionRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	targetType, err := intrusion.ParseTargetType(c.Query("type"))
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if s.intrusions == nil {
		return c.JSON(model.ListResponse[model.IntrusionRecord]{
			Items: nil,
			Count: 0,
		})
	}
	s.refreshIntrusionArchive()
	if err := s.pruneIntrusionsByCurrentUserSettings(); err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	items, err := s.intrusions.List(intrusion.QueryOptions{
		Limit:      parseLimit(c, 200),
		TargetType: targetType,
	})
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.ListResponse[model.IntrusionRecord]{
		Items: items,
		Count: len(items),
	})
}

// handleDeleteIntrusionRecords deletes selected intrusion records.
func (s *Server) handleDeleteIntrusionRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.IntrusionDeleteRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	ids := normalizedIntrusionRecordIDs(req.IDs)
	if len(ids) == 0 {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			"empty intrusion record ids",
		)
	}
	if s.intrusions == nil {
		return c.JSON(model.IntrusionDeleteResponse{})
	}
	deleted, err := s.intrusions.Delete(ids)
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.IntrusionDeleteResponse{Deleted: deleted})
}

func (s *Server) refreshIntrusionArchive() {
	if s.detection == nil {
		return
	}
	_ = s.detection.ScreenDetections(0)
	_ = s.detection.ScreenPositions(0)
}

func normalizedIntrusionRecordIDs(ids []string) []string {
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(normalized, id) {
			continue
		}
		normalized = append(normalized, id)
	}
	return normalized
}
