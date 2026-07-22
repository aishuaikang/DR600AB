package systemtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	state   bool
	calls   []command
	handler func(command) (string, error)
}

func (f *fakeRunner) run(_ context.Context, request command) (string, error) {
	f.calls = append(f.calls, request)
	if f.handler != nil {
		return f.handler(request)
	}
	return "", nil
}

func TestGetInfo(t *testing.T) {
	runner := &fakeRunner{}
	runner.handler = func(request command) (string, error) {
		switch strings.Join(append([]string{request.name}, request.args...), " ") {
		case "date +%Y-%m-%d %H:%M:%S":
			return "2026-07-22 10:20:30\n", nil
		case "date +%:z":
			return "+08:00\n", nil
		case "timedatectl show --property=NTP --value":
			return "yes\n", nil
		case "timedatectl show --property=NTPSynchronized --value":
			return "yes\n", nil
		case "timedatectl show --property=Timezone --value":
			return "Asia/Shanghai\n", nil
		default:
			return "", errors.New("unexpected command")
		}
	}
	service := newService("linux", runner)
	service.readFile = func(string) ([]byte, error) { return nil, errors.New("not found") }
	service.evalSymlinks = func(string) (string, error) { return "", errors.New("not found") }

	info, err := service.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo returned error: %v", err)
	}
	if info.Platform != "linux" || !info.TimeManagementSupported || info.CurrentTime != "2026-07-22 10:20:30" || info.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected info: %#v", info)
	}
	if info.UTCOffset != "+08:00" || !info.NTPEnabled || !info.NTPSynced {
		t.Fatalf("unexpected status fields: %#v", info)
	}
}

func TestGetInfoUnsupported(t *testing.T) {
	service := newService("darwin", &fakeRunner{})
	info, err := service.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo returned error: %v", err)
	}
	if info.Platform != "darwin" || info.TimeManagementSupported {
		t.Fatalf("unexpected unsupported info: %#v", info)
	}
}

func TestListTimezonesSortsAndDeduplicates(t *testing.T) {
	runner := &fakeRunner{handler: func(request command) (string, error) {
		if request.name == "timedatectl" && len(request.args) == 1 && request.args[0] == "list-timezones" {
			return "UTC\nAsia/Shanghai\nUTC\nAmerica/New_York\n", nil
		}
		return "", errors.New("unexpected command")
	}}
	service := newService("linux", runner)
	zones, err := service.ListTimezones(context.Background())
	if err != nil {
		t.Fatalf("ListTimezones returned error: %v", err)
	}
	want := []string{"America/New_York", "Asia/Shanghai", "UTC"}
	if strings.Join(zones, ",") != strings.Join(want, ",") {
		t.Fatalf("zones = %#v, want %#v", zones, want)
	}
}

func TestSetTimezoneValidatesAndUsesTimedatectl(t *testing.T) {
	runner := &fakeRunner{}
	service := newService("linux", runner)
	if err := service.SetTimezone(context.Background(), " Asia/Shanghai "); err != nil {
		t.Fatalf("SetTimezone returned error: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "timedatectl" || strings.Join(runner.calls[0].args, " ") != "set-timezone Asia/Shanghai" {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}

	runner.calls = nil
	if err := service.SetTimezone(context.Background(), "Asia/Shanghai; reboot"); !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("invalid timezone error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid timezone executed commands: %#v", runner.calls)
	}
}

func TestSetTimezoneFallbackWritesTimezoneToStdin(t *testing.T) {
	runner := &fakeRunner{handler: func(request command) (string, error) {
		if request.name == "timedatectl" {
			return "timedatectl unavailable", errors.New("command unavailable")
		}
		if request.name == "ln" {
			return "", nil
		}
		if request.name == "tee" {
			if request.stdin != "Asia/Shanghai\n" || strings.Join(request.args, " ") != "/etc/timezone" {
				return "", errors.New("unexpected tee request")
			}
			return "", nil
		}
		return "", errors.New("unexpected command")
	}}
	service := newService("linux", runner)
	service.timezoneRoot = t.TempDir()
	zoneFile := filepath.Join(service.timezoneRoot, "Asia", "Shanghai")
	if err := os.MkdirAll(filepath.Dir(zoneFile), 0o755); err != nil {
		t.Fatalf("create timezone directory: %v", err)
	}
	if err := os.WriteFile(zoneFile, []byte("zoneinfo"), 0o644); err != nil {
		t.Fatalf("create timezone file: %v", err)
	}
	if err := service.SetTimezone(context.Background(), "Asia/Shanghai"); err != nil {
		t.Fatalf("SetTimezone fallback returned error: %v", err)
	}
}

func TestSetTimezoneFallbackRejectsMissingZone(t *testing.T) {
	runner := &fakeRunner{handler: func(request command) (string, error) {
		if request.name == "timedatectl" || request.name == "sudo" {
			return "timezone command unavailable", errors.New("command unavailable")
		}
		return "", nil
	}}
	service := newService("linux", runner)
	service.timezoneRoot = t.TempDir()

	err := service.SetTimezone(context.Background(), "Asia/Shanghai")
	if !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("missing timezone error = %v, want ErrInvalidTimezone", err)
	}
	for _, call := range runner.calls {
		if call.name == "ln" || call.name == "tee" {
			t.Fatalf("missing timezone attempted filesystem mutation: %#v", runner.calls)
		}
	}
}

func TestSetNTPEnabledVerifiesState(t *testing.T) {
	runner := &fakeRunner{}
	runner.handler = func(request command) (string, error) {
		if request.name == "timedatectl" && len(request.args) == 2 && request.args[0] == "set-ntp" {
			runner.state = request.args[1] == "true"
			return "", nil
		}
		if request.name == "timedatectl" && len(request.args) == 3 && request.args[0] == "show" && request.args[1] == "--property=NTP" {
			if runner.state {
				return "yes\n", nil
			}
			return "no\n", nil
		}
		return "", errors.New("optional command unavailable")
	}
	service := newService("linux", runner)
	if err := service.SetNTPEnabled(context.Background(), true); err != nil {
		t.Fatalf("SetNTPEnabled returned error: %v", err)
	}
	if !runner.state {
		t.Fatal("NTP state was not enabled")
	}
}

func TestSetNTPEnabledUsesActiveKnownService(t *testing.T) {
	runner := &fakeRunner{}
	runner.handler = func(request command) (string, error) {
		if request.name == "systemctl" && len(request.args) == 2 && request.args[0] == "is-active" && request.args[1] == "chrony.service" {
			return "active\n", nil
		}
		if request.name == "systemctl" && len(request.args) == 2 && (request.args[0] == "enable" || request.args[0] == "restart") && request.args[1] == "chrony.service" {
			return "", nil
		}
		if request.name == "timedatectl" && len(request.args) == 2 && request.args[0] == "set-ntp" {
			runner.state = request.args[1] == "true"
			return "", nil
		}
		if request.name == "timedatectl" && len(request.args) == 3 && request.args[0] == "show" && request.args[1] == "--property=NTP" {
			if runner.state {
				return "yes\n", nil
			}
			return "no\n", nil
		}
		return "", errors.New("optional command unavailable")
	}

	service := newService("linux", runner)
	if err := service.SetNTPEnabled(context.Background(), true); err != nil {
		t.Fatalf("SetNTPEnabled returned error: %v", err)
	}
	var enabledChrony, enabledTimesyncd bool
	for _, call := range runner.calls {
		if call.name != "systemctl" || len(call.args) < 2 || call.args[0] != "enable" {
			continue
		}
		switch call.args[1] {
		case "chrony.service":
			enabledChrony = true
		case "systemd-timesyncd.service":
			enabledTimesyncd = true
		}
	}
	if !enabledChrony || enabledTimesyncd {
		t.Fatalf("unexpected NTP service selection: chrony=%v timesyncd=%v calls=%#v", enabledChrony, enabledTimesyncd, runner.calls)
	}
}

func TestSetManualTimeDisablesNTP(t *testing.T) {
	runner := &fakeRunner{}
	runner.handler = func(request command) (string, error) {
		if request.name == "timedatectl" && len(request.args) == 2 && request.args[0] == "set-ntp" {
			runner.state = request.args[1] == "true"
			return "", nil
		}
		if request.name == "timedatectl" && len(request.args) == 3 && request.args[0] == "show" && request.args[1] == "--property=NTP" {
			if runner.state {
				return "yes\n", nil
			}
			return "no\n", nil
		}
		if request.name == "timedatectl" && len(request.args) == 2 && request.args[0] == "set-time" {
			return "", nil
		}
		return "", errors.New("optional command unavailable")
	}
	runner.state = true
	service := newService("linux", runner)
	if err := service.SetManualTime(context.Background(), "2026-07-22 10:20:30"); err != nil {
		t.Fatalf("SetManualTime returned error: %v", err)
	}
	if runner.state {
		t.Fatal("manual time did not disable NTP")
	}

	runner.calls = nil
	if err := service.SetManualTime(context.Background(), "2026-02-30 10:20:30"); !errors.Is(err, ErrInvalidManualTime) {
		t.Fatalf("invalid manual time error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid manual time executed commands: %#v", runner.calls)
	}
}

func TestFormatUTCOffset(t *testing.T) {
	if got := formatUTCOffset(8 * 60 * 60); got != "+08:00" {
		t.Fatalf("formatUTCOffset(+8h) = %q", got)
	}
	if got := formatUTCOffset(-5 * 60 * 60); got != "-05:00" {
		t.Fatalf("formatUTCOffset(-5h) = %q", got)
	}
}
