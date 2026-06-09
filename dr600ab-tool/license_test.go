package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLicensePath(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "license.lic")
	if err := os.WriteFile(valid, []byte("license"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLicensePath(valid); err != nil {
		t.Fatalf("valid license path rejected: %v", err)
	}

	empty := filepath.Join(dir, "empty.lic")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLicensePath(empty); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("empty license error = %v, want empty file error", err)
	}

	wrongExt := filepath.Join(dir, "license.txt")
	if err := os.WriteFile(wrongExt, []byte("license"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLicensePath(wrongExt); err == nil || !strings.Contains(err.Error(), ".lic") {
		t.Fatalf("wrong extension error = %v, want .lic error", err)
	}
}

func TestLicenseUploadURLUsesSSHHostAndDefaultPort(t *testing.T) {
	app := NewApp()
	app.conn = &sshConnection{config: SSHConnectRequest{Host: "192.168.1.50"}}

	got := app.licenseUploadURL(LicenseUploadRequest{})
	want := "http://192.168.1.50:18080/api/v1/license/upload"
	if got != want {
		t.Fatalf("licenseUploadURL() = %q, want %q", got, want)
	}
}

func TestPostLicenseFile(t *testing.T) {
	licenseFile := filepath.Join(t.TempDir(), "license.lic")
	if err := os.WriteFile(licenseFile, []byte("license-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/license/upload" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("missing multipart file: %v", err)
		}
		defer file.Close()
		_, _ = w.Write([]byte(`{"message":"授权文件已上传并激活"}`))
	}))
	defer server.Close()

	message, err := postLicenseFile(server.URL+"/api/v1/license/upload", licenseFile)
	if err != nil {
		t.Fatal(err)
	}
	if message != "授权文件已上传并激活" {
		t.Fatalf("message = %q", message)
	}
}

func TestPostLicenseFileReturnsAPIError(t *testing.T) {
	licenseFile := filepath.Join(t.TempDir(), "license.lic")
	if err := os.WriteFile(licenseFile, []byte("license-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(licenseAPIError{Message: "授权文件无效", Code: "license_invalid"})
	}))
	defer server.Close()

	_, err := postLicenseFile(server.URL, licenseFile)
	if err == nil || !strings.Contains(err.Error(), "license_invalid") {
		t.Fatalf("error = %v, want API code", err)
	}
}
