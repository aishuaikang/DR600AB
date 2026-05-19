// Package settings 在后端重启之间持久化操作配置。
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"dr600ab-api/internal/model"
)

// Store 将操作设置持久化到本地 JSON 文件。
type Store struct {
	mu   sync.Mutex
	path string
}

type savedSettings struct {
	Detection model.DetectionSessionRequest `json:"detection"`
	GPS       model.GPSSessionRequest       `json:"gps"`
	Network   model.NetworkSettings         `json:"network"`
	User      model.UserSettings            `json:"user"`
}

// NewStore 创建使用指定路径的设置存储。
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load 在文件存在时读取已持久化的侦测设置。
func (s *Store) Load() (model.DetectionSessionRequest, bool, error) {
	if s == nil || s.path == "" {
		return model.DetectionSessionRequest{}, false, nil
	}

	settings, ok, err := s.load()
	if err != nil || !ok {
		return model.DetectionSessionRequest{}, false, err
	}
	if isEmptyDetectionSettings(settings.Detection) {
		return model.DetectionSessionRequest{}, false, nil
	}
	return settings.Detection, true, nil
}

// Save 以原子方式将侦测设置写入磁盘。
func (s *Store) Save(req model.DetectionSessionRequest) error {
	if s == nil || s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, _, err := s.load()
	if err != nil {
		return err
	}
	settings.Detection = req
	return s.save(settings)
}

// LoadGPS 在文件存在时读取已持久化的 GPS 设置。
func (s *Store) LoadGPS() (model.GPSSessionRequest, bool, error) {
	if s == nil || s.path == "" {
		return model.GPSSessionRequest{}, false, nil
	}

	settings, ok, err := s.load()
	if err != nil || !ok {
		return model.GPSSessionRequest{}, false, err
	}
	if isEmptyGPSSettings(settings.GPS) {
		return model.GPSSessionRequest{}, false, nil
	}
	return settings.GPS, true, nil
}

// SaveGPS 以原子方式将 GPS 设置写入磁盘。
func (s *Store) SaveGPS(req model.GPSSessionRequest) error {
	if s == nil || s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, _, err := s.load()
	if err != nil {
		return err
	}
	settings.GPS = req
	return s.save(settings)
}

// LoadNetwork 在文件存在时读取已持久化的网络设置。
func (s *Store) LoadNetwork() (model.NetworkSettings, bool, error) {
	if s == nil || s.path == "" {
		return model.NetworkSettings{}, false, nil
	}

	settings, ok, err := s.load()
	if err != nil || !ok {
		return model.NetworkSettings{}, false, err
	}
	if isEmptyNetworkSettings(settings.Network) {
		return model.NetworkSettings{}, false, nil
	}
	return settings.Network, true, nil
}

// SaveNetwork 以原子方式将网络设置写入磁盘。
func (s *Store) SaveNetwork(req model.NetworkSettings) error {
	if s == nil || s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, _, err := s.load()
	if err != nil {
		return err
	}
	settings.Network = req
	return s.save(settings)
}

// LoadUser 在文件存在时读取已持久化的公开用户设置。
func (s *Store) LoadUser() (model.UserSettings, bool, error) {
	if s == nil || s.path == "" {
		return model.UserSettings{}, false, nil
	}

	settings, ok, err := s.load()
	if err != nil || !ok {
		return model.UserSettings{}, false, err
	}
	if isEmptyUserSettings(settings.User) {
		return model.UserSettings{}, false, nil
	}
	return settings.User, true, nil
}

// SaveUser 以原子方式将公开用户设置写入磁盘。
func (s *Store) SaveUser(req model.UserSettings) error {
	if s == nil || s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, _, err := s.load()
	if err != nil {
		return err
	}
	settings.User = req
	return s.save(settings)
}

func (s *Store) load() (savedSettings, bool, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return savedSettings{}, false, nil
		}
		return savedSettings{}, false, err
	}

	var settings savedSettings
	if err := json.Unmarshal(data, &settings); err == nil {
		if !isEmptyDetectionSettings(settings.Detection) ||
			!isEmptyGPSSettings(settings.GPS) ||
			!isEmptyNetworkSettings(settings.Network) ||
			!isEmptyUserSettings(settings.User) {
			return settings, true, nil
		}
	}

	var legacy model.DetectionSessionRequest
	if err := json.Unmarshal(data, &legacy); err != nil {
		return savedSettings{}, false, err
	}
	return savedSettings{Detection: legacy}, true, nil
}

func (s *Store) save(settings savedSettings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	settings = normalizeSavedSettings(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func isEmptyDetectionSettings(req model.DetectionSessionRequest) bool {
	return req.PortName == "" &&
		req.RxPortName == "" &&
		req.TxPortName == "" &&
		req.BaudRate == 0 &&
		req.DataBits == 0 &&
		req.StopBits == 0 &&
		req.Parity == "" &&
		req.ReadTimeoutMs == 0 &&
		!req.AutoConnect
}

func isEmptyGPSSettings(req model.GPSSessionRequest) bool {
	return req.PortName == "" &&
		req.DataPortName == "" &&
		req.ControlPortName == "" &&
		req.BaudRate == 0 &&
		req.DataBits == 0 &&
		req.StopBits == 0 &&
		req.Parity == "" &&
		req.ReadTimeoutMs == 0 &&
		!req.AutoConnect
}

func isEmptyNetworkSettings(req model.NetworkSettings) bool {
	return len(req.Priorities) == 0
}

func isEmptyUserSettings(req model.UserSettings) bool {
	return req.ManualDeviceLocation == nil &&
		len(req.ScreenStrikeChannelLabels) == 0
}

func normalizeSavedSettings(settings savedSettings) savedSettings {
	if settings.Network.Priorities == nil {
		settings.Network.Priorities = []model.NetworkPrioritySetting{}
	}
	if settings.User.ScreenStrikeChannelLabels == nil {
		settings.User.ScreenStrikeChannelLabels = []string{}
	}
	return settings
}
