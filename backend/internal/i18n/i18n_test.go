package i18n

import "testing"

func TestTranslatorLoadsMeta(t *testing.T) {
	tr, err := New("zh-CN")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	meta := tr.Meta()
	if meta.Default != "zh-CN" {
		t.Fatalf("default locale = %q, want zh-CN", meta.Default)
	}
	if len(meta.Supported) < 2 {
		t.Fatalf("supported locales = %v, want at least 2", meta.Supported)
	}
	if len(meta.Namespaces) == 0 {
		t.Fatal("namespaces should not be empty")
	}
}

func TestTranslatorFallsBackToDefaultLocale(t *testing.T) {
	tr, err := New("zh-CN")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got := tr.T("fr-FR", "common", "session.started")
	if got != "侦测会话已启动" {
		t.Fatalf("fallback translation = %q, want Chinese fallback", got)
	}
}
