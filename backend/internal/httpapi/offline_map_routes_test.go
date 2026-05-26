package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/config"
)

func TestOfflineMapRoutesServeTiles(t *testing.T) {
	mapRoot := t.TempDir()
	tilePath := filepath.Join(mapRoot, "dt", "14", "13520", "6851.jpg")
	if err := os.MkdirAll(filepath.Dir(tilePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(tilePath, []byte("tile"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := &Server{
		app: fiber.New(),
		cfg: config.Config{OfflineMapPath: mapRoot},
	}
	server.registerOfflineMapRoutes()

	req, err := http.NewRequest(http.MethodGet, "/map/dt/14/13520/6851.jpg", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "tile" {
		t.Fatalf("body = %q, want tile", body)
	}
}

func TestOfflineMapRoutesMissingTileDoesNotFallThrough(t *testing.T) {
	server := &Server{
		app: fiber.New(),
		cfg: config.Config{OfflineMapPath: t.TempDir()},
	}
	server.registerOfflineMapRoutes()
	server.app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendString("frontend")
	})

	req, err := http.NewRequest(http.MethodGet, "/map/dt/14/13520/missing.jpg", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
