package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/interferencereport"
	"dr600ab-api/internal/model"
)

// registerInterferenceReportRoutes 挂载干扰报告接口。
func (s *Server) registerInterferenceReportRoutes(api fiber.Router) {
	api.Get("/interference-reports", s.handleInterferenceReports)
	api.Get("/interference-reports/:id", s.handleInterferenceReport)
	api.Delete("/interference-reports/:id", s.handleDeleteFailedInterferenceReport)
}

// handleInterferenceReports 返回干扰报告摘要列表。
func (s *Server) handleInterferenceReports(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	status, err := interferencereport.ParseStatus(c.Query("status"))
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if s.interferenceReports == nil {
		return c.JSON(model.ListResponse[model.InterferenceReportSummary]{
			Items: nil,
			Count: 0,
		})
	}
	limit := parseLimit(c, 200)
	offset := parseOffset(c)
	items, err := s.interferenceReports.List(interferencereport.QueryOptions{
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

// handleInterferenceReport 返回单条干扰报告详情。
func (s *Server) handleInterferenceReport(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.interferenceReports == nil {
		return s.respondError(c, fiber.StatusNotFound, "interference_report_not_found", s.translator.T(locale, "errors", "interference_report_not_found"), nil)
	}
	report, err := s.interferenceReports.Get(c.Params("id"))
	if err != nil {
		if errors.Is(err, interferencereport.ErrNotFound) {
			return s.respondError(c, fiber.StatusNotFound, "interference_report_not_found", s.translator.T(locale, "errors", "interference_report_not_found"), nil)
		}
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(report)
}

// handleDeleteFailedInterferenceReport 删除启动失败的干扰报告。
func (s *Server) handleDeleteFailedInterferenceReport(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.interferenceReports == nil {
		return c.JSON(model.InterferenceReportDeleteResponse{})
	}
	deleted, err := s.interferenceReports.DeleteFailed(c.Params("id"))
	if err != nil {
		if errors.Is(err, interferencereport.ErrNotFound) {
			return s.respondError(c, fiber.StatusNotFound, "interference_report_not_found", s.translator.T(locale, "errors", "interference_report_not_found"), nil)
		}
		if errors.Is(err, interferencereport.ErrNotFailed) {
			return s.respondError(c, fiber.StatusConflict, "interference_report_delete_not_failed", s.translator.T(locale, "errors", "interference_report_delete_not_failed"), nil)
		}
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.InterferenceReportDeleteResponse{Deleted: deleted})
}
