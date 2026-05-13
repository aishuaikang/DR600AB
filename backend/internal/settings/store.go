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

// Store 将侦测会话设置持久化到本地 JSON 文件。
type Store struct {
	mu   sync.Mutex
	path string
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

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.DetectionSessionRequest{}, false, nil
		}
		return model.DetectionSessionRequest{}, false, err
	}

	var req model.DetectionSessionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return model.DetectionSessionRequest{}, false, err
	}
	return req, true, nil
}

// Save 以原子方式将侦测设置写入磁盘。
func (s *Store) Save(req model.DetectionSessionRequest) error {
	if s == nil || s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(req, "", "  ")
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
