package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/deceptionreport"
	"dr600ab-api/internal/model"
)

// registerDeceptionReportRoutes 挂载诱骗报告接口。
func (s *Server) registerDeceptionReportRoutes(api fiber.Router) {
	api.Get("/deception-reports", s.handleDeceptionReports)
	api.Get("/deception-reports/:id", s.handleDeceptionReport)
	api.Delete("/deception-reports/:id", s.handleDeleteFailedDeceptionReport)
}

// handleDeceptionReports 返回诱骗报告摘要列表。
func (s *Server) handleDeceptionReports(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	status, err := deceptionreport.ParseStatus(c.Query("status"))
	if err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}
	if s.reports == nil {
		return c.JSON(model.ListResponse[model.DeceptionReportSummary]{
			Items: nil,
			Count: 0,
		})
	}
	items, err := s.reports.List(deceptionreport.QueryOptions{
		Limit:  parseLimit(c, 200),
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
	return c.JSON(model.ListResponse[model.DeceptionReportSummary]{
		Items: items,
		Count: len(items),
	})
}

// handleDeceptionReport 返回单条诱骗报告详情。
func (s *Server) handleDeceptionReport(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.reports == nil {
		return s.respondError(c, fiber.StatusNotFound, "deception_report_not_found", s.translator.T(locale, "errors", "deception_report_not_found"), nil)
	}
	report, err := s.reports.Get(c.Params("id"))
	if err != nil {
		if errors.Is(err, deceptionreport.ErrNotFound) {
			return s.respondError(c, fiber.StatusNotFound, "deception_report_not_found", s.translator.T(locale, "errors", "deception_report_not_found"), nil)
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

// handleDeleteFailedDeceptionReport 删除启动失败的诱骗报告。
func (s *Server) handleDeleteFailedDeceptionReport(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.reports == nil {
		return c.JSON(model.DeceptionReportDeleteResponse{})
	}
	deleted, err := s.reports.DeleteFailed(c.Params("id"))
	if err != nil {
		if errors.Is(err, deceptionreport.ErrNotFound) {
			return s.respondError(c, fiber.StatusNotFound, "deception_report_not_found", s.translator.T(locale, "errors", "deception_report_not_found"), nil)
		}
		if errors.Is(err, deceptionreport.ErrNotFailed) {
			return s.respondError(c, fiber.StatusConflict, "deception_report_delete_not_failed", s.translator.T(locale, "errors", "deception_report_delete_not_failed"), nil)
		}
		return s.respondError(
			c,
			fiber.StatusInternalServerError,
			"internal",
			s.translator.T(locale, "errors", "internal"),
			err.Error(),
		)
	}
	return c.JSON(model.DeceptionReportDeleteResponse{Deleted: deleted})
}
