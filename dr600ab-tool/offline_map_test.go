package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPrepareOfflineMapPackageNormalizesLayout(t *testing.T) {
	source := filepath.Join(t.TempDir(), "map.zip")
	writeTestZip(t, source, map[string]string{
		"root/dt/12/345/678.jpeg": "tile",
	})

	prepared, count, cleanup, err := prepareOfflineMapPackage(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if count != 1 {
		t.Fatalf("got %d tiles, want 1", count)
	}
	reader, err := zip.OpenReader(prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	if !slices.Contains(names, "dt/12/345/678.jpg") {
		t.Fatalf("normalized zip missing tile, names=%v", names)
	}
}

func TestPrepareOfflineMapPackageRejectsTraversal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bad.zip")
	writeTestZip(t, source, map[string]string{
		"../dt/1/2/3.jpg": "bad",
	})
	if _, _, cleanup, err := prepareOfflineMapPackage(source); err == nil {
		cleanup()
		t.Fatalf("expected traversal error")
	}
}

func TestBuildOfflineMapInstallScript(t *testing.T) {
	script := buildOfflineMapInstallScript("/opt/dr600ab", "/opt/dr600ab/static/map/.uploads/1/offline-map.zip", "1", true)
	for _, want := range []string{"KEEP_BACKUP='1'", "unzip -q", "mv \"$STAGING_DIR/dt\" \"$CURRENT_DT\""} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q", want)
		}
	}
}

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(out)
	for name, content := range files {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
