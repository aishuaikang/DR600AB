package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/fpvrecord"
	"dr600ab-api/internal/model"
)

// registerFPVVideoRecordRoutes 挂载 FPV 图传观看记录接口。
func (s *Server) registerFPVVideoRecordRoutes(api fiber.Router) {
	api.Get("/fpv-video-records", s.handleFPVVideoRecords)
	api.Get("/fpv-video-records/:id", s.handleFPVVideoRecord)
	api.Delete("/fpv-video-records", s.handleDeleteFPVVideoRecords)
}

// handleFPVVideoRecords 返回已入库的 FPV 图传观看记录。
func (s *Server) handleFPVVideoRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	status, err := fpvrecord.ParseStatus(c.Query("status"))
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if s.fpvRecords == nil {
		return c.JSON(model.ListResponse[model.FPVVideoRecord]{
			Items: nil,
			Count: 0,
		})
	}
	if err := s.pruneFPVVideoRecordsByCurrentUserSettings(); err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	limit := parseLimit(c, 200)
	offset := parseOffset(c)
	items, err := s.fpvRecords.List(fpvrecord.QueryOptions{
		Limit:  limit + 1,
		Offset: offset,
		Status: status,
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
	return c.JSON(pagedListResponse(items, limit, offset))
}

// handleFPVVideoRecord 返回一条 FPV 图传观看记录详情。
func (s *Server) handleFPVVideoRecord(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.fpvRecords == nil {
		return s.respondError(
			c,
			fiber.StatusNotFound,
			"fpv_video_record_not_found",
			s.translator.T(locale, "errors", "fpv_video_record_not_found"),
			nil,
		)
	}
	record, ok, err := s.fpvRecords.Get(c.Params("id"))
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
		return s.respondError(
			c,
			fiber.StatusNotFound,
			"fpv_video_record_not_found",
			s.translator.T(locale, "errors", "fpv_video_record_not_found"),
			nil,
		)
	}
	return c.JSON(record)
}

// handleDeleteFPVVideoRecords deletes selected FPV video records.
func (s *Server) handleDeleteFPVVideoRecords(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.FPVVideoRecordDeleteRequest
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
			"empty fpv video record ids",
		)
	}
	if s.fpvRecords == nil {
		return c.JSON(model.FPVVideoRecordDeleteResponse{})
	}
	deleted, err := s.fpvRecords.Delete(ids)
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.FPVVideoRecordDeleteResponse{Deleted: deleted})
}
