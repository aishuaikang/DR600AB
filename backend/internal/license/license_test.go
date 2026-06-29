package license

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceActivateValidLicense(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	path := filepath.Join(t.TempDir(), "license.lic")
	service := NewService(path, func() (string, error) { return deviceSN, nil })
	raw, err := service.Generate(deviceSN, 24*time.Hour, "test", time.Now())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	status, err := service.Activate(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !status.Valid || status.DeviceSN != deviceSN || !service.IsValid() {
		t.Fatalf("status = %#v, runtime valid = %v; want valid for %s", status, service.IsValid(), deviceSN)
	}
	if saved, err := os.ReadFile(path); err != nil || !bytes.Equal(saved, raw) {
		t.Fatalf("saved license = %q, %v; want uploaded content", saved, err)
	}
}

func TestServiceActivateRejectsSNMismatchWithoutReplacingCurrentFile(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	path := filepath.Join(t.TempDir(), "license.lic")
	service := NewService(path, func() (string, error) { return deviceSN, nil })
	valid, err := service.Generate(deviceSN, 24*time.Hour, "test", time.Now())
	if err != nil {
		t.Fatalf("Generate(valid) error = %v", err)
	}
	if status, err := service.Activate(bytes.NewReader(valid)); err != nil || !status.Valid {
		t.Fatalf("Activate(valid) = %#v, %v; want valid", status, err)
	}
	mismatch, err := service.Generate("SL6FFFFFFFFFFFFFFFFP", 24*time.Hour, "test", time.Now())
	if err != nil {
		t.Fatalf("Generate(mismatch) error = %v", err)
	}

	status, err := service.Activate(bytes.NewReader(mismatch))
	if !errors.Is(err, ErrSNMismatch) {
		t.Fatalf("Activate(mismatch) error = %v, want ErrSNMismatch", err)
	}
	if status.Valid || status.Code != "license_sn_mismatch" {
		t.Fatalf("status = %#v, want mismatch invalid status", status)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(current, valid) {
		t.Fatal("active license file was replaced by invalid upload")
	}
	if !service.IsValid() {
		t.Fatal("runtime state should keep previous valid license")
	}
}

func TestServiceStatusIncludesDeviceSNWhenLicenseMissing(t *testing.T) {
	deviceSN := "SL67CB3FC848FA0E795P"
	service := NewService(filepath.Join(t.TempDir(), "missing.lic"), func() (string, error) { return deviceSN, nil })

	status, err := service.Status()
	if !errors.Is(err, ErrLicenseNotFound) {
		t.Fatalf("Status() error = %v, want ErrLicenseNotFound", err)
	}
	if status.Valid || status.DeviceSN != deviceSN || status.Code != "license_not_found" {
		t.Fatalf("status = %#v, want invalid status with device SN", status)
	}
}
