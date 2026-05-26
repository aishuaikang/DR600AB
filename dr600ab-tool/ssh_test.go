package main

import "testing"

func TestNormalizeSSHParams(t *testing.T) {
	tests := []struct {
		name    string
		req     SSHConnectRequest
		want    SSHConnectRequest
		wantErr bool
	}{
		{
			name: "host with port",
			req:  SSHConnectRequest{Host: "192.168.1.10:2200", User: "root"},
			want: SSHConnectRequest{Host: "192.168.1.10", Port: 2200, User: "root"},
		},
		{
			name: "default port",
			req:  SSHConnectRequest{Host: "board.local", User: "ask"},
			want: SSHConnectRequest{Host: "board.local", Port: 22, User: "ask"},
		},
		{
			name:    "empty host",
			req:     SSHConnectRequest{User: "root"},
			wantErr: true,
		},
		{
			name:    "invalid port",
			req:     SSHConnectRequest{Host: "board.local", Port: 70000, User: "root"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSSHParams(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got.Password = ""
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReconnectRequestUsesLiveConnectionPassword(t *testing.T) {
	app := NewApp()
	app.conn = &sshConnection{
		config: SSHConnectRequest{
			Host:     "board.local",
			Port:     2200,
			User:     "root",
			Password: "live-pass",
		},
	}

	got, err := app.reconnectRequest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "board.local" || got.Port != 2200 || got.User != "root" || got.Password != "live-pass" {
		t.Fatalf("unexpected reconnect request: %+v", got)
	}
}

func TestReconnectRequestLoadsRememberedPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restoreKeyring := stubKeyring(t)
	keyringGet = func(service, account string) (string, error) {
		if service != keyringService || account != "root@board.local:2222" {
			t.Fatalf("unexpected keyring read: service=%q account=%q", service, account)
		}
		return "saved-pass", nil
	}
	defer restoreKeyring()

	app := NewApp()
	if err := app.SaveConfig(AppConfig{
		SSH: &SavedSSHConfig{
			Host:             "board.local",
			Port:             2222,
			User:             "root",
			RememberPassword: true,
			Password:         "saved-pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := app.reconnectRequest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "board.local" || got.Port != 2222 || got.User != "root" || got.Password != "saved-pass" || !got.RememberPassword {
		t.Fatalf("unexpected reconnect request: %+v", got)
	}
}

func TestReconnectRequestRequiresPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restoreKeyring := stubKeyring(t)
	defer restoreKeyring()

	app := NewApp()
	if err := app.SaveConfig(AppConfig{
		SSH: &SavedSSHConfig{
			Host: "board.local",
			Port: 22,
			User: "root",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.reconnectRequest(); err == nil {
		t.Fatalf("expected missing password error")
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote("/opt/dr600ab path/it's")
	want := "'/opt/dr600ab path/it'\\''s'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRemoteJoin(t *testing.T) {
	got := remoteJoin("/opt/dr600ab", "static", "map", "dt")
	want := "/opt/dr600ab/static/map/dt"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
