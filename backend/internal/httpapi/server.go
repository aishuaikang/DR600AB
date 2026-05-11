package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"dr600ab-api/internal/config"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/model"
)

type Server struct {
	app          *fiber.App
	cfg          config.Config
	translator   *i18n.Translator
	detection    *detection.Service
	interference *interference.Service
}

func New(cfg config.Config, translator *i18n.Translator, detectionSvc *detection.Service, interferenceSvc *interference.Service) *Server {
	s := &Server{
		cfg:          cfg,
		translator:   translator,
		detection:    detectionSvc,
		interference: interferenceSvc,
	}
	s.app = fiber.New(fiber.Config{
		AppName: "dr600ab-api",
	})
	s.routes()
	return s
}

func (s *Server) App() *fiber.App {
	return s.app
}

func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

func (s *Server) Shutdown() error {
	s.detection.Stop("")
	s.interference.Shutdown()
	return s.app.Shutdown()
}

func (s *Server) routes() {
	s.app.Use(recover.New())
	s.app.Use(logger.New())
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: strings.Join(s.cfg.CORSAllowedOrigins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, X-Locale",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
	}))

	s.app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"ok":   true,
			"time": time.Now(),
		})
	})

	api := s.app.Group("/api/v1")
	api.Get("/meta/locales", s.handleLocales)
	api.Get("/serial/ports", s.handlePorts)
	api.Get("/detection/settings", s.handleDetectionSettings)
	api.Get("/detection/session", s.handleCurrentSession)
	api.Post("/detection/session", s.handleStartSession)
	api.Put("/detection/settings", s.handleUpdateDetectionSettings)
	api.Delete("/detection/session", s.handleStopSession)
	api.Get("/detection/stream", s.handleStream)
	api.Get("/detection/records", s.handleDetectionRecords)
	api.Get("/parsed/records", s.handleParsedRecords)
	api.Get("/fpv/records", s.handleFPVRecords)
	api.Get("/interference/channels", s.handleInterferenceChannels)
	api.Post("/interference/channels/:id/state", s.handleSetChannelState)
}

func (s *Server) handleLocales(c *fiber.Ctx) error {
	return c.JSON(s.translator.Meta())
}

func (s *Server) handlePorts(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	ports, err := s.detection.ListPorts()
	if err != nil {
		return s.respondError(c, fiber.StatusInternalServerError, "internal", s.translator.T(locale, "errors", "internal"), err.Error())
	}
	return c.JSON(fiber.Map{
		"ports":         ports,
		"activeSession": s.detection.Current(locale),
	})
}

func (s *Server) handleCurrentSession(c *fiber.Ctx) error {
	return c.JSON(s.detection.Current(s.resolveLocale(c)))
}

func (s *Server) handleStartSession(c *fiber.Ctx) error {
	return s.handleUpdateDetectionSettings(c)
}

func (s *Server) handleDetectionSettings(c *fiber.Ctx) error {
	settings, ok, err := s.detection.Settings()
	if err != nil {
		locale := s.resolveLocale(c)
		return s.respondError(c, fiber.StatusInternalServerError, "internal", s.translator.T(locale, "errors", "internal"), err.Error())
	}
	if !ok {
		return c.JSON(fiber.Map{})
	}
	return c.JSON(settings)
}

func (s *Server) handleUpdateDetectionSettings(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.DetectionSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(c, fiber.StatusBadRequest, "invalid_request", s.translator.T(locale, "errors", "invalid_request"), err.Error())
	}
	if strings.TrimSpace(req.RxPortName) == "" && strings.TrimSpace(req.PortName) == "" {
		return s.respondError(c, fiber.StatusBadRequest, "port_required", s.translator.T(locale, "errors", "port_required"), nil)
	}

	response, err := s.detection.Start(req, locale)
	if err != nil {
		code := "port_open_failed"
		status := fiber.StatusBadRequest
		if strings.HasPrefix(err.Error(), s.translator.T(locale, "errors", "internal")) {
			code = "internal"
			status = fiber.StatusInternalServerError
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}
	return c.JSON(response)
}

func (s *Server) handleStopSession(c *fiber.Ctx) error {
	return c.JSON(s.detection.Stop(s.resolveLocale(c)))
}

func (s *Server) handleStream(c *fiber.Ctx) error {
	ctx := c.Context()

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		events, unsubscribe := s.detection.Subscribe(s.cfg.EventBufferSize)
		defer unsubscribe()

		_ = writeComment(w, "connected")
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case evt, ok := <-events:
				if !ok {
					return
				}
				if err := writeEvent(w, evt); err != nil {
					return
				}
			case <-ticker.C:
				if err := writeComment(w, "ping"); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
	return nil
}

func (s *Server) handleDetectionRecords(c *fiber.Ctx) error {
	items := s.detection.Records(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.DetectionRecord]{Items: items, Count: len(items)})
}

func (s *Server) handleParsedRecords(c *fiber.Ctx) error {
	items := s.detection.Parsed(parseLimit(c, 200))
	return c.JSON(model.ListResponse[model.ParsedMessage]{Items: items, Count: len(items)})
}

func (s *Server) handleFPVRecords(c *fiber.Ctx) error {
	items := s.detection.FPV(parseLimit(c, 100))
	return c.JSON(model.ListResponse[model.FpvRecord]{Items: items, Count: len(items)})
}

func (s *Server) handleInterferenceChannels(c *fiber.Ctx) error {
	channels := s.interference.ListChannels()
	return c.JSON(fiber.Map{
		"channels": channels,
		"count":    len(channels),
	})
}

func (s *Server) handleSetChannelState(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	var req model.GpioChannelStateRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(c, fiber.StatusBadRequest, "invalid_request", s.translator.T(locale, "errors", "invalid_request"), err.Error())
	}

	channel, err := s.interference.SetState(c.Params("id"), req.Enabled, locale)
	if err != nil {
		status := fiber.StatusInternalServerError
		code := "gpio_update_failed"
		if channel.Reserved {
			status = fiber.StatusConflict
			code = "channel_reserved"
		}
		if channel.ID == "" {
			status = fiber.StatusNotFound
			code = "channel_not_found"
		}
		return s.respondError(c, status, code, err.Error(), nil)
	}

	messageKey := "gpio.disabled"
	if req.Enabled {
		messageKey = "gpio.enabled"
	}
	return c.JSON(model.GpioChannelStateResponse{
		Channel: channel,
		Message: s.translator.T(locale, "common", messageKey),
	})
}

func (s *Server) respondError(c *fiber.Ctx, status int, code, message string, details any) error {
	return c.Status(status).JSON(model.ApiError{
		Code:    code,
		Message: message,
		Details: details,
	})
}

func (s *Server) resolveLocale(c *fiber.Ctx) string {
	if locale := c.Query("locale"); locale != "" {
		return s.translator.Normalize(locale)
	}
	if locale := c.Get("X-Locale"); locale != "" {
		return s.translator.Normalize(locale)
	}
	if accept := c.Get(fiber.HeaderAcceptLanguage); accept != "" {
		for _, part := range strings.Split(accept, ",") {
			tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
			if tag != "" {
				return s.translator.Normalize(tag)
			}
		}
	}
	return s.translator.Normalize("")
}

func parseLimit(c *fiber.Ctx, fallback int) int {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

func writeEvent(w *bufio.Writer, evt model.Event) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", evt.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return w.Flush()
}

func writeComment(w *bufio.Writer, value string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", value); err != nil {
		return err
	}
	return w.Flush()
}
