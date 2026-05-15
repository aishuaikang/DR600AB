// Package httpapi 将后端服务接入 HTTP 和服务端事件 API。
package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/config"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/gps"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/network"
)

// Server 持有 Fiber 应用以及对外暴露的后端服务。
type Server struct {
	app          *fiber.App
	cfg          config.Config
	translator   *i18n.Translator
	detection    *detection.Service
	interference *interference.Service
	developer    *developer.Service
	gps          *gps.Service
	network      *network.Service
}

// New 创建 Server，并注册中间件和 API 路由。
func New(
	cfg config.Config,
	translator *i18n.Translator,
	detectionSvc *detection.Service,
	interferenceSvc *interference.Service,
	developerSvc *developer.Service,
	gpsSvc *gps.Service,
	networkSvc *network.Service,
) *Server {
	s := &Server{
		cfg:          cfg,
		translator:   translator,
		detection:    detectionSvc,
		interference: interferenceSvc,
		developer:    developerSvc,
		gps:          gpsSvc,
		network:      networkSvc,
	}
	s.app = fiber.New(fiber.Config{
		AppName: "dr600ab-api",
	})
	s.routes()
	return s
}

// Listen 在指定地址启动 HTTP 服务。
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

// Shutdown 关闭运行中的服务并停止 HTTP 服务。
func (s *Server) Shutdown() error {
	s.detection.Stop("")
	s.gps.Stop("")
	s.interference.Shutdown()
	return s.app.Shutdown()
}
