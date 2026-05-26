package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigStoresRememberedPasswordOutsideConfigFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotService, gotAccount, gotPassword string
	restoreKeyring := stubKeyring(t)
	keyringSet = func(service, account, password string) error {
		gotService = service
		gotAccount = account
		gotPassword = password
		return nil
	}
	defer restoreKeyring()

	app := NewApp()
	err := app.SaveConfig(AppConfig{
		SSH: &SavedSSHConfig{
			Host:             " board.local ",
			Port:             2200,
			User:             " root ",
			RememberPassword: true,
			Password:         "secret-pass",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotService != keyringService || gotAccount != "root@board.local:2200" || gotPassword != "secret-pass" {
		t.Fatalf("unexpected keyring write: service=%q account=%q password=%q", gotService, gotAccount, gotPassword)
	}

	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-pass") || strings.Contains(string(data), "password") {
		t.Fatalf("config file leaked password: %s", data)
	}
}

func TestLoadConfigReadsRememberedPasswordFromKeyring(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restoreKeyring := stubKeyring(t)
	keyringGet = func(service, account string) (string, error) {
		if service != keyringService || account != "root@board.local:22" {
			t.Fatalf("unexpected keyring read: service=%q account=%q", service, account)
		}
		return "loaded-pass", nil
	}
	defer restoreKeyring()

	configFile, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(`{"ssh":{"host":"board.local","port":22,"user":"root","rememberPassword":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewApp().LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSH == nil || cfg.SSH.Password != "loaded-pass" {
		t.Fatalf("password not loaded: %+v", cfg.SSH)
	}
}

func TestSaveConfigDeletesPasswordWhenRememberDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotAccount string
	restoreKeyring := stubKeyring(t)
	keyringDelete = func(service, account string) error {
		gotAccount = account
		return nil
	}
	defer restoreKeyring()

	err := NewApp().SaveConfig(AppConfig{
		SSH: &SavedSSHConfig{
			Host: "board.local",
			Port: 22,
			User: "root",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAccount != "root@board.local:22" {
		t.Fatalf("unexpected deleted account %q", gotAccount)
	}
}

func stubKeyring(t *testing.T) func() {
	t.Helper()
	originalGet := keyringGet
	originalSet := keyringSet
	originalDelete := keyringDelete
	keyringGet = func(_, _ string) (string, error) { return "", nil }
	keyringSet = func(_, _, _ string) error { return nil }
	keyringDelete = func(_, _ string) error { return nil }
	return func() {
		keyringGet = originalGet
		keyringSet = originalSet
		keyringDelete = originalDelete
	}
}
