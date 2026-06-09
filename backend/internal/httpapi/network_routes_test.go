package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkRoutesAllowLicensedUserWithoutDeveloperSession(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	path := filepath.Join(t.TempDir(), "license.lic")
	server := newLicenseTestServer(t, path, deviceSN)
	raw, err := server.license.Generate(deviceSN, 24*time.Hour, "test", time.Now())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	protected := server.app.Group("/api/v1", server.requireLicense)
	server.registerNetworkRoutes(protected)

	req, err := http.NewRequest(http.MethodPut, "/api/v1/network/interfaces/eth0", strings.NewReader("{"))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d without developer session", resp.StatusCode, http.StatusBadRequest)
	}
}
