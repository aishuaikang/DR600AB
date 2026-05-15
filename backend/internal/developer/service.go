// Package developer 管理 TOTP 开发者登录和短时会话。
package developer

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	codeLength = 6
	timeStep   = 30 * time.Second
	skewSteps  = 1
)

var (
	// ErrNotConfigured 表示后端没有配置开发者 TOTP 密钥。
	ErrNotConfigured = errors.New("developer totp secret is not configured")
	// ErrInvalidCode 表示开发者 TOTP 验证码错误或已过期。
	ErrInvalidCode = errors.New("invalid developer totp code")
	// ErrInvalidSession 表示开发者会话不存在或已过期。
	ErrInvalidSession = errors.New("invalid developer session")
)

type session struct {
	expiresAt time.Time
}

// Service 校验 TOTP 验证码，并维护短时开发者会话。
type Service struct {
	mu         sync.Mutex
	secret     []byte
	sessions   map[string]session
	sessionTTL time.Duration
}

// NewService 创建开发者登录服务。
func NewService(secret string, sessionTTL time.Duration) (*Service, error) {
	decodedSecret, err := decodeSecret(secret)
	if err != nil {
		return nil, err
	}
	if sessionTTL <= 0 {
		sessionTTL = 10 * time.Minute
	}

	return &Service{
		secret:     decodedSecret,
		sessions:   map[string]session{},
		sessionTTL: sessionTTL,
	}, nil
}

// Login 校验 TOTP 动态码，并返回短时会话 token。
func (s *Service) Login(code string) (string, time.Time, error) {
	code = strings.TrimSpace(code)
	if len(s.secret) == 0 {
		return "", time.Time{}, ErrNotConfigured
	}
	if len(code) != codeLength {
		return "", time.Time{}, ErrInvalidCode
	}

	now := time.Now()
	if !s.verifyCode(code, now) {
		return "", time.Time{}, ErrInvalidCode
	}

	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.Add(s.sessionTTL)

	s.mu.Lock()
	s.cleanupLocked(now)
	s.sessions[token] = session{expiresAt: expiresAt}
	s.mu.Unlock()

	return token, expiresAt, nil
}

// Logout 删除开发者会话。
func (s *Service) Logout(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// Validate 校验开发者会话是否仍有效。
func (s *Service) Validate(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidSession
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)
	item, ok := s.sessions[token]
	if !ok || item.expiresAt.Before(now) {
		return ErrInvalidSession
	}
	return nil
}

// CodeLength 返回验证码位数。
func (s *Service) CodeLength() int {
	return codeLength
}

// SessionTTL 返回开发者会话有效期。
func (s *Service) SessionTTL() time.Duration {
	return s.sessionTTL
}

func (s *Service) verifyCode(code string, now time.Time) bool {
	step := now.Unix() / int64(timeStep.Seconds())
	for offset := -skewSteps; offset <= skewSteps; offset++ {
		expected := totpCode(s.secret, uint64(step+int64(offset)))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func (s *Service) cleanupLocked(now time.Time) {
	for token, item := range s.sessions {
		if !item.expiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
}

func decodeSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil
	}

	normalized := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(secret))
	if decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized); err == nil {
		return decoded, nil
	}
	if decoded, err := base32.StdEncoding.DecodeString(normalized); err == nil {
		return decoded, nil
	}

	hexDecoded, hexErr := hex.DecodeString(secret)
	if hexErr == nil {
		return hexDecoded, nil
	}

	return nil, fmt.Errorf("解析开发者 TOTP 密钥失败")
}

func totpCode(secret []byte, counter uint64) string {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], counter)

	hash := hmac.New(sha1.New, secret)
	hash.Write(buffer[:])
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulo := uint32(math.Pow10(codeLength))
	code := value % modulo

	return fmt.Sprintf("%0*s", codeLength, strconv.FormatUint(uint64(code), 10))
}

func randomToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("生成会话令牌失败: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
