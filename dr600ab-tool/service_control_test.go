package main

import (
	"strings"
	"testing"
)

func TestBuildStopAllServicesScript(t *testing.T) {
	script := buildStopAllServicesScript("/opt/dr600ab prod")

	for _, want := range []string{
		"INSTALL_DIR='/opt/dr600ab prod'",
		"$SUDO systemctl stop dr600ab-kiosk.service",
		"$SUDO systemctl stop dr600ab.service",
		"backend_pids=\"$(pgrep -x dr600ab",
		"stop_pid_list \"$backend_pids\"",
		"pgrep -f '[d]r600ab-kiosk-start|[c]hromium.*127[.]0[.]0[.]1:18080'",
		"stop_pid_list \"$kiosk_pids\"",
		"$SUDO kill -TERM \"$pid\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q\n%s", want, script)
		}
	}

	for _, forbidden := range []string{
		"systemctl disable",
		"rm -f /etc/systemd/system",
		"pkill",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("script contains %q\n%s", forbidden, script)
		}
	}
}
