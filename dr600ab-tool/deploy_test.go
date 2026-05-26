package main

import (
	"strings"
	"testing"
)

func TestBuildDeployScript(t *testing.T) {
	script := buildDeployScript(DeployRequest{
		InstallDir: "/opt/dr600ab prod",
		FullUpdate: true,
	}, "/tmp/pkg/dr600ab.tar.gz", "/tmp/task", "ask")

	for _, want := range []string{
		"FULL_UPDATE='1'",
		"INSTALL_DIR='/opt/dr600ab prod'",
		"KIOSK_USER='ask'",
		"tar -xzf \"$REMOTE_PACKAGE\" -C \"$EXTRACT_DIR\"",
		"$SUDO install -m 0755 \"$BINARY\" \"$INSTALL_DIR/dr600ab\"",
		"/etc/systemd/system/dr600ab.service",
		"/etc/systemd/system/dr600ab-kiosk.service",
		"API_INTRUSION_DB_PATH=$INSTALL_DIR/data/intrusions.db",
		"API_DECEPTION_REPORT_DB_PATH=$INSTALL_DIR/data/deception-reports.db",
		"API_INTERFERENCE_REPORT_DB_PATH=$INSTALL_DIR/data/interference-reports.db",
		"API_OFFLINE_MAP_PATH=$INSTALL_DIR/static/map",
		"$SUDO systemctl restart dr600ab.service",
		"$SUDO systemctl restart dr600ab-kiosk.service",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q\n%s", want, script)
		}
	}
}

func TestBuildDeployScriptIncrementalPreservesDataDirs(t *testing.T) {
	script := buildDeployScript(DeployRequest{
		InstallDir: "/opt/dr600ab",
		FullUpdate: false,
	}, "/tmp/pkg/dr600ab.tar.gz", "/tmp/task", "root")

	for _, want := range []string{
		"FULL_UPDATE='0'",
		"$SUDO mkdir -p \"$INSTALL_DIR/data\" \"$INSTALL_DIR/backend/data\" \"$INSTALL_DIR/static/map\"",
		"clear_chromium_cache \"$CHROMIUM_USER_DATA_DIR\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q\n%s", want, script)
		}
	}
}

func TestBuildDeployScriptFullUpdateClearsInstallDir(t *testing.T) {
	script := buildDeployScript(DeployRequest{
		InstallDir: "/opt/dr600ab",
		FullUpdate: true,
	}, "/tmp/pkg/dr600ab.tar.gz", "/tmp/task", "")

	for _, want := range []string{
		"SERVICE_USER='root'",
		"KIOSK_USER='root'",
		`$SUDO find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q\n%s", want, script)
		}
	}
}

func TestValidateFirmwarePackagePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "linux arm64", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-linux-arm64.tar.gz"},
		{name: "linux amd64", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-linux-amd64.tar.gz"},
		{name: "dist subdir accepted", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/packages/dr600ab-linux-arm64.tar.gz"},
		{name: "darwin rejected", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-darwin-arm64.tar.gz", wantErr: true},
		{name: "windows rejected", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-windows-amd64.zip", wantErr: true},
		{name: "generic release accepted", path: "/tmp/release.tar.gz"},
		{name: "tgz rejected", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-linux-arm64.tgz", wantErr: true},
		{name: "wrong suffix rejected", path: "/Users/ask/Desktop/spbatc/dr600ab/dist/dr600ab-linux-arm64.txt", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFirmwarePackagePath(tt.path)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
