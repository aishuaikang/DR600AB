package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDBKeyFromFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "db.key")
	if err := os.WriteFile(keyPath, []byte(" file-key \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("API_DB_KEY", "")
	t.Setenv("API_DB_KEY_FILE", keyPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DBKey != "file-key" {
		t.Fatalf("DBKey = %q, want file-key", cfg.DBKey)
	}
}

func TestLoadDirectDBKeyWinsOverFile(t *testing.T) {
	t.Setenv("API_DB_KEY", "direct-key")
	t.Setenv("API_DB_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DBKey != "direct-key" {
		t.Fatalf("DBKey = %q, want direct-key", cfg.DBKey)
	}
}

func TestLoadReadsLicensePath(t *testing.T) {
	t.Setenv("API_LICENSE_PATH", "/tmp/dr600ab-license.lic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LicensePath != "/tmp/dr600ab-license.lic" {
		t.Fatalf("LicensePath = %q, want /tmp/dr600ab-license.lic", cfg.LicensePath)
	}
}

func TestLoadErrorsWhenDBKeyFileMissing(t *testing.T) {
	t.Setenv("API_DB_KEY", "")
	t.Setenv("API_DB_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))

	if _, err := Load(); err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
}

func TestLoadErrorsWhenDBKeyFileEmpty(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "db.key")
	if err := os.WriteFile(keyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("API_DB_KEY", "")
	t.Setenv("API_DB_KEY_FILE", keyPath)

	if _, err := Load(); err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
}
