package httpapi

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/webassets"
)

const frontendIndexPath = "dist/index.html"

// registerFrontendRoutes serves the embedded frontend build after all API routes.
func (s *Server) registerFrontendRoutes() {
	dist, err := fs.Sub(webassets.FS, "dist")
	if err != nil {
		return
	}

	s.app.Get("/", func(c *fiber.Ctx) error {
		return s.sendFrontendIndex(c)
	})
	s.app.Get("/*", func(c *fiber.Ctx) error {
		requestPath := strings.TrimPrefix(c.Path(), "/")
		if requestPath == "" {
			return s.sendFrontendIndex(c)
		}
		if strings.HasPrefix(requestPath, "api/") || requestPath == "healthz" {
			return fiber.ErrNotFound
		}

		cleanPath := path.Clean(requestPath)
		if cleanPath == "." || strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
			return fiber.ErrNotFound
		}

		file, err := dist.Open(cleanPath)
		if err == nil {
			defer file.Close()
			info, statErr := file.Stat()
			if statErr == nil && !info.IsDir() {
				return sendEmbeddedFile(c, file, cleanPath)
			}
		}

		return s.sendFrontendIndex(c)
	})
}

func (s *Server) sendFrontendIndex(c *fiber.Ctx) error {
	file, err := webassets.FS.Open(frontendIndexPath)
	if err != nil {
		return fiber.ErrNotFound
	}
	defer file.Close()
	return sendEmbeddedFile(c, file, "index.html")
}

func sendEmbeddedFile(c *fiber.Ctx, file fs.File, name string) error {
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if name == "index.html" || strings.HasSuffix(name, "/index.html") {
		c.Set(fiber.HeaderCacheControl, "no-cache, no-store, must-revalidate")
		c.Set(fiber.HeaderPragma, "no-cache")
		c.Set(fiber.HeaderExpires, "0")
	} else if strings.HasPrefix(name, "assets/") {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		c.Set(fiber.HeaderContentType, contentType)
	} else {
		c.Set(fiber.HeaderContentType, http.DetectContentType(data))
	}
	return c.Send(data)
}
