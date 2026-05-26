package httpapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// registerOfflineMapRoutes 将已安装的离线地图瓦片暴露到 /map。
func (s *Server) registerOfflineMapRoutes() {
	mapPath := strings.TrimSpace(s.cfg.OfflineMapPath)
	if mapPath == "" {
		return
	}

	s.app.Static("/map", mapPath, fiber.Static{
		CacheDuration: 24 * time.Hour,
		MaxAge:        int((24 * time.Hour).Seconds()),
	})
	s.app.Get("/map/*", func(c *fiber.Ctx) error {
		return fiber.ErrNotFound
	})
}
