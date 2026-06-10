package httpapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
	"dr600ab-api/internal/network"
)

// registerNetworkRoutes 挂载系统网口配置接口。
func (s *Server) registerNetworkRoutes(api fiber.Router) {
	api.Get("/network/interfaces", s.handleNetworkInterfaces)
	api.Put("/network/interfaces/:name", s.handleUpdateNetworkInterface)
	api.Put("/network/priorities", s.handleUpdateNetworkInterfacePriorities)
	api.Get("/network/wifi", s.handleWiFiNetworks)
	api.Post("/network/wifi/connect", s.handleConnectWiFi)
	api.Post("/network/wifi/disconnect", s.handleDisconnectWiFi)
	api.Post("/network/cellular/connect", s.handleConnectCellular)
}

// handleNetworkInterfaces 返回当前系统网口状态。
func (s *Server) handleNetworkInterfaces(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	interfaces, err := s.network.ListInterfaces(c.Context())
	if err != nil {
		if errors.Is(err, network.ErrBackendUnavailable) {
			message := s.translator.T(locale, "errors", "network_backend_unavailable")
			return c.JSON(model.NetworkInterfacesResponse{
				Interfaces: []model.NetworkInterface{},
				Count:      0,
				Backend:    "networkmanager",
				Available:  false,
				ReadOnly:   true,
				Message:    message,
			})
		}
		return s.respondNetworkError(c, locale, err)
	}
	return c.JSON(model.NetworkInterfacesResponse{
		Interfaces: interfaces,
		Count:      len(interfaces),
		Backend:    "networkmanager",
		Available:  true,
		ReadOnly:   false,
	})
}

// handleUpdateNetworkInterfacePriorities 批量更新网口路由优先级。
func (s *Server) handleUpdateNetworkInterfacePriorities(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.NetworkPriorityBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	items, err := s.network.UpdateInterfacePriorities(c.Context(), req)
	if err != nil {
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.NetworkPriorityBatchResponse{
		Interfaces: items,
		Message:    s.translator.T(locale, "common", "network.priority_updated"),
	})
}

// handleUpdateNetworkInterface 更新指定网口 IPv4 配置。
func (s *Server) handleUpdateNetworkInterface(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.NetworkInterfaceUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	item, err := s.network.UpdateInterface(c.Context(), c.Params("name"), req)
	if err != nil {
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.NetworkInterfaceUpdateResponse{
		Interface: item,
		Message:   s.translator.T(locale, "common", "network.updated"),
	})
}

// handleWiFiNetworks 返回附近无线网络。
func (s *Server) handleWiFiNetworks(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	networks, err := s.network.ScanWiFi(c.Context())
	if err != nil {
		if errors.Is(err, network.ErrBackendUnavailable) || errors.Is(err, network.ErrWiFiUnavailable) {
			message := s.translator.T(locale, "errors", "wifi_unavailable")
			if errors.Is(err, network.ErrBackendUnavailable) {
				message = s.translator.T(locale, "errors", "network_backend_unavailable")
			}
			return c.JSON(model.WiFiNetworksResponse{
				Networks:  []model.WiFiNetwork{},
				Count:     0,
				Available: false,
				ReadOnly:  true,
				Message:   message,
			})
		}
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.WiFiNetworksResponse{
		Networks:  networks,
		Count:     len(networks),
		Available: true,
		ReadOnly:  false,
	})
}

// handleConnectWiFi 连接指定无线网络。
func (s *Server) handleConnectWiFi(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.WiFiConnectRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	if err := s.network.ConnectWiFi(c.Context(), req); err != nil {
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.WiFiConnectResponse{
		Message: s.translator.T(locale, "common", "wifi.connected"),
	})
}

// handleDisconnectWiFi 断开当前无线网络。
func (s *Server) handleDisconnectWiFi(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.WiFiDisconnectRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return s.respondError(
				c,
				fiber.StatusBadRequest,
				"invalid_request",
				s.translator.T(locale, "errors", "invalid_request"),
				err.Error(),
			)
		}
	}

	if err := s.network.DisconnectWiFi(c.Context(), req); err != nil {
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.WiFiDisconnectResponse{
		Message: s.translator.T(locale, "common", "wifi.disconnected"),
	})
}

// handleConnectCellular 创建或更新移动网络连接并尝试拨号。
func (s *Server) handleConnectCellular(c *fiber.Ctx) error {
	locale := s.resolveLocale(c)

	var req model.CellularConnectRequest
	if err := c.BodyParser(&req); err != nil {
		return s.respondError(
			c,
			fiber.StatusBadRequest,
			"invalid_request",
			s.translator.T(locale, "errors", "invalid_request"),
			err.Error(),
		)
	}

	items, err := s.network.ConnectCellular(c.Context(), req)
	if err != nil {
		return s.respondNetworkError(c, locale, err)
	}

	return c.JSON(model.CellularConnectResponse{
		Interfaces: items,
		Message:    s.translator.T(locale, "common", "cellular.connected"),
	})
}

func (s *Server) requireDeveloper(c *fiber.Ctx, locale string) bool {
	err := s.developer.Validate(s.developerToken(c))
	if err == nil {
		return true
	}
	_ = s.respondError(
		c,
		fiber.StatusUnauthorized,
		"developer_invalid_session",
		s.translator.T(locale, "errors", "developer_invalid_session"),
		nil,
	)
	return false
}

func (s *Server) requireDeveloperOrLicenseRecovery(c *fiber.Ctx, locale string) bool {
	err := s.developer.Validate(s.developerToken(c))
	if err == nil {
		return true
	}
	if s.licenseRecoveryAllowed() {
		return true
	}
	_ = s.respondError(
		c,
		fiber.StatusUnauthorized,
		"developer_invalid_session",
		s.translator.T(locale, "errors", "developer_invalid_session"),
		nil,
	)
	return false
}

func (s *Server) developerToken(c *fiber.Ctx) string {
	token := strings.TrimSpace(c.Get("X-Developer-Token"))
	if token != "" {
		return token
	}
	return strings.TrimSpace(c.Query("developerToken"))
}

func (s *Server) respondNetworkError(c *fiber.Ctx, locale string, err error) error {
	status := fiber.StatusInternalServerError
	code := "network_update_failed"
	message := err.Error()

	switch {
	case errors.Is(err, network.ErrBackendUnavailable):
		status = fiber.StatusServiceUnavailable
		code = "network_backend_unavailable"
		message = s.translator.T(locale, "errors", "network_backend_unavailable")
	case errors.Is(err, network.ErrInvalidConfig):
		status = fiber.StatusBadRequest
		code = "network_invalid_config"
		message = s.translator.T(locale, "errors", "network_invalid_config")
	case errors.Is(err, network.ErrInterfaceNotFound):
		status = fiber.StatusNotFound
		code = "network_interface_not_found"
		message = s.translator.T(locale, "errors", "network_interface_not_found")
	case errors.Is(err, network.ErrInterfaceUnmanaged):
		status = fiber.StatusConflict
		code = "network_interface_unmanaged"
		message = s.translator.T(locale, "errors", "network_interface_unmanaged")
	case errors.Is(err, network.ErrWiFiUnavailable):
		status = fiber.StatusServiceUnavailable
		code = "wifi_unavailable"
		message = s.translator.T(locale, "errors", "wifi_unavailable")
	case errors.Is(err, network.ErrInvalidWiFiConfig):
		status = fiber.StatusBadRequest
		code = "wifi_invalid_config"
		message = s.translator.T(locale, "errors", "wifi_invalid_config")
	case errors.Is(err, network.ErrCellularUnavailable):
		status = fiber.StatusServiceUnavailable
		code = "cellular_unavailable"
		message = s.translator.T(locale, "errors", "cellular_unavailable")
	case errors.Is(err, network.ErrInvalidCellularConfig):
		status = fiber.StatusBadRequest
		code = "cellular_invalid_config"
		message = s.translator.T(locale, "errors", "cellular_invalid_config")
	}

	return s.respondError(c, status, code, message, err.Error())
}
