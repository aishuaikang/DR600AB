package httpapi

import "github.com/gofiber/fiber/v2"

// registerMetaRoutes 挂载前端使用的元数据接口。
func (s *Server) registerMetaRoutes(api fiber.Router) {
	api.Get("/meta/locales", s.handleLocales)
}

// handleLocales 返回后端支持的本地化元数据。
func (s *Server) handleLocales(c *fiber.Ctx) error {
	return c.JSON(s.translator.Meta())
}
