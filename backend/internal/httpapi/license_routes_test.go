package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/license"
	"dr600ab-api/internal/model"
)

func TestLicenseProtectedRouteRejectsMissingLicense(t *testing.T) {
	server := newLicenseTestServer(t, filepath.Join(t.TempDir(), "license.lic"), "SL67CB3FC848FA0E795P")
	api := server.app.Group("/api/v1")
	api.Get("/protected", server.requireLicense, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestUploadLicenseActivatesProtectedRoutes(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	path := filepath.Join(t.TempDir(), "license.lic")
	server := newLicenseTestServer(t, path, deviceSN)
	server.registerLicenseRoutes(server.app.Group("/api/v1"))
	api := server.app.Group("/api/v1")
	api.Get("/protected", server.requireLicense, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	raw, err := server.license.Generate(deviceSN, 24*time.Hour, "test", time.Now())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	req := newMultipartLicenseRequest(t, "/api/v1/license/upload", raw)
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test(upload) error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req, err = http.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	if err != nil {
		t.Fatalf("NewRequest(protected) error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test(protected) error = %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("protected status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestLicenseStatusReturnsCurrentDeviceSNWhenMissing(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	server := newLicenseTestServer(t, filepath.Join(t.TempDir(), "missing.lic"), deviceSN)
	server.registerLicenseRoutes(server.app.Group("/api/v1"))

	req, err := http.NewRequest(http.MethodGet, "/api/v1/license/status", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body model.LicenseInfo
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Valid || body.DeviceSN != deviceSN || body.Code != "license_not_found" {
		t.Fatalf("body = %#v, want missing license status with device SN", body)
	}
}

func TestLicenseRecoveryRoutesOnlyExposeDetectionSerialSetup(t *testing.T) {
	server := newLicenseTestServer(t, filepath.Join(t.TempDir(), "license.lic"), "")
	api := server.app.Group("/api/v1")
	api.Get("/detection/settings", func(c *fiber.Ctx) error {
		if !server.requireDeveloperOrLicenseRecovery(c, server.resolveLocale(c)) {
			return nil
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	protected := api.Group("", server.requireLicense)
	protected.Get("/gps/settings", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/v1/detection/settings", nil)
	if err != nil {
		t.Fatalf("NewRequest(detection) error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test(detection) error = %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("detection settings status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	req, err = http.NewRequest(http.MethodGet, "/api/v1/gps/settings", nil)
	if err != nil {
		t.Fatalf("NewRequest(gps) error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test(gps) error = %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("gps settings status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestLicenseRecoveryRoutesStillRequireDeveloperWhenDeviceSNExists(t *testing.T) {
	server := newLicenseTestServer(t, filepath.Join(t.TempDir(), "license.lic"), "SL67CB3FC848FA0E795P")
	if err := server.developer.Validate(""); err == nil {
		t.Fatalf("developer Validate(empty) unexpectedly succeeded")
	}
	if server.licenseRecoveryAllowed() {
		t.Fatalf("licenseRecoveryAllowed() unexpectedly succeeded")
	}
	api := server.app.Group("/api/v1")
	api.Get("/detection/settings", func(c *fiber.Ctx) error {
		if !server.requireDeveloperOrLicenseRecovery(c, server.resolveLocale(c)) {
			return nil
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/v1/detection/settings", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireDeveloperStopsHandlerWhenSessionInvalid(t *testing.T) {
	server := newLicenseTestServer(t, filepath.Join(t.TempDir(), "license.lic"), "SL67CB3FC848FA0E795P")
	api := server.app.Group("/api/v1")
	api.Get("/developer-only", func(c *fiber.Ctx) error {
		if !server.requireDeveloper(c, server.resolveLocale(c)) {
			return nil
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/v1/developer-only", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func newLicenseTestServer(t *testing.T, path string, deviceSN string) *Server {
	t.Helper()
	developerSvc, err := developer.NewService("test-license-developer-secret", time.Minute)
	if err != nil {
		t.Fatalf("developer.NewService() error = %v", err)
	}
	server := &Server{
		app:        fiber.New(),
		translator: mustTranslator(t),
		developer:  developerSvc,
		license:    license.NewService(path, func() (string, error) { return deviceSN, nil }),
	}
	return server
}

func newMultipartLicenseRequest(t *testing.T, path string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "license.lic")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write(content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, path, &body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
