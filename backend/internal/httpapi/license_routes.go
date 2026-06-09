package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/license"
	"dr600ab-api/internal/model"
)

// registerLicenseRoutes 挂载授权状态和授权文件上传接口。
func (s *Server) registerLicenseRoutes(api fiber.Router) {
	api.Get("/license/status", s.handleLicenseStatus)
	api.Post("/license/upload", s.handleUploadLicense)
}

func (s *Server) handleLicenseStatus(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.license == nil {
		return s.respondError(c, fiber.StatusServiceUnavailable, "license_unavailable", s.translator.T(locale, "errors", "license_unavailable"), nil)
	}
	status, err := s.license.Status()
	if err != nil || status.Code != "" {
		code := status.Code
		if code == "" {
			code = licenseErrorCode(err)
		}
		status.Message = s.licenseErrorMessage(locale, code, status)
	}
	return c.JSON(status)
}

func (s *Server) handleUploadLicense(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if s.license == nil {
		return s.respondError(c, fiber.StatusServiceUnavailable, "license_unavailable", s.translator.T(locale, "errors", "license_unavailable"), nil)
	}

	file, err := c.FormFile("file")
	if err != nil {
		return s.respondError(c, fiber.StatusBadRequest, "invalid_request", s.translator.T(locale, "errors", "invalid_request"), err.Error())
	}
	src, err := file.Open()
	if err != nil {
		return s.respondError(c, fiber.StatusInternalServerError, "internal", s.translator.T(locale, "errors", "internal"), err.Error())
	}
	defer src.Close()

	status, err := s.license.Activate(src)
	if err != nil {
		return s.respondLicenseUploadError(c, locale, err, status)
	}
	return c.JSON(model.LicenseUploadResponse{
		License: status,
		Message: s.translator.T(locale, "common", "license.uploaded"),
	})
}

func (s *Server) requireLicense(c *fiber.Ctx) error {
	if s == nil || s.license == nil {
		return nil
	}
	status, err := s.license.Status()
	if err == nil && status.Valid {
		return c.Next()
	}
	s.license.Refresh()
	return s.respondLicenseError(c, s.resolveLocale(c), err, status)
}

func (s *Server) licenseRecoveryAllowed() bool {
	if s == nil || s.license == nil {
		return false
	}
	status, err := s.license.Status()
	return errors.Is(err, license.ErrDeviceSNMissing) && status.Code == "device_sn_missing"
}

func (s *Server) respondLicenseUploadError(c *fiber.Ctx, locale string, err error, status model.LicenseInfo) error {
	code := status.Code
	if code == "" {
		code = licenseErrorCode(err)
	}
	message := s.licenseErrorMessage(locale, code, status)
	httpStatus := fiber.StatusBadRequest
	if errors.Is(err, license.ErrDeviceSNMissing) {
		httpStatus = fiber.StatusServiceUnavailable
	}
	return s.respondError(c, httpStatus, code, message, status)
}

func (s *Server) respondLicenseError(c *fiber.Ctx, locale string, err error, status model.LicenseInfo) error {
	code := status.Code
	if code == "" {
		code = licenseErrorCode(err)
	}
	message := s.licenseErrorMessage(locale, code, status)
	httpStatus := fiber.StatusForbidden
	switch {
	case errors.Is(err, license.ErrDeviceSNMissing), errors.Is(err, license.ErrLicenseNotFound):
		httpStatus = fiber.StatusServiceUnavailable
	}
	return s.respondError(c, httpStatus, code, message, status)
}

func (s *Server) licenseErrorMessage(locale, code string, status model.LicenseInfo) string {
	messageKey := code
	if messageKey == "" {
		messageKey = "license_invalid"
	}
	message := s.translator.T(locale, "errors", messageKey)
	if message == messageKey && status.Message != "" {
		return status.Message
	}
	return message
}

func licenseErrorCode(err error) string {
	switch {
	case errors.Is(err, license.ErrDeviceSNMissing):
		return "device_sn_missing"
	case errors.Is(err, license.ErrLicenseNotFound):
		return "license_not_found"
	case errors.Is(err, license.ErrLicenseExpired):
		return "license_expired"
	case errors.Is(err, license.ErrSNMismatch):
		return "license_sn_mismatch"
	case errors.Is(err, license.ErrInvalidSignature):
		return "license_invalid_signature"
	case errors.Is(err, license.ErrInvalidLicense):
		return "license_invalid"
	default:
		return "license_verification_failed"
	}
}
