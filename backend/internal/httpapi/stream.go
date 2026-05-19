package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

// handleStream 保持服务端事件流开启，用于推送运行时更新。
func (s *Server) handleStream(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)
	if err := s.requireDeveloper(c, locale); err != nil {
		return err
	}

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

// handleScreenStream 保持大屏公开只读事件流开启，只推送大屏相关事件。
func (s *Server) handleScreenStream(c *fiber.Ctx) error {
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
				if evt.Type != "screen.detection.updated" && evt.Type != "screen.position.updated" && evt.Type != "screen.strike.updated" {
					continue
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

// writeEvent 写入一个命名 SSE 事件，并携带 JSON 格式的 model.Event。
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

// writeComment 写入 SSE 注释，用于连接提示和保活消息。
func writeComment(w *bufio.Writer, value string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", value); err != nil {
		return err
	}
	return w.Flush()
}
