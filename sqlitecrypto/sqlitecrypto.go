// Package sqlitecrypto centralizes SQLite opening for plaintext and encrypted
// database files without requiring CGO.
package sqlitecrypto

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
	"gorm.io/gorm"
)

const (
	sqliteHeader       = "SQLite format 3\x00"
	encryptedVFS       = "adiantum"
	defaultDatabaseKey = "dr600ab-default-sqlite-database-key-v1"
)

// ErrEncryptedWithoutKey is returned when an encrypted database is opened
// without a configured key.
var ErrEncryptedWithoutKey = errors.New("数据库已加密但未配置数据库密钥")

// Config controls how a SQLite database is opened.
type Config struct {
	// Key is the encrypted VFS passphrase. Empty uses the built-in default key.
	Key string
	// Plaintext explicitly disables encryption. Use this only for tests or
	// tools that must create a plain SQLite database.
	Plaintext bool
}

// Encrypted reports whether encrypted SQLite should be used.
func (c Config) Encrypted() bool {
	return !c.Plaintext
}

// ConfigFromEnv reads the database encryption key from APP_DATABASE_KEY or
// APP_DATABASE_KEY_FILE. If neither is set, the built-in default key is used.
func ConfigFromEnv() (Config, error) {
	key, err := EnvKey("APP_DATABASE_KEY", "APP_DATABASE_KEY_FILE")
	if err != nil {
		return Config{}, err
	}
	return Config{Key: key}, nil
}

// EnvKey reads a secret from valueKey or fileKey. The direct value wins.
func EnvKey(valueKey, fileKey string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(valueKey)); value != "" {
		return value, nil
	}
	filePath := strings.TrimSpace(os.Getenv(fileKey))
	if filePath == "" {
		return "", nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取 %s 指定的密钥文件失败: %w", fileKey, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("%s 指定的密钥文件为空: %s", fileKey, filePath)
	}
	return secret, nil
}

// Open opens a SQLite database. Disk databases use the encrypted VFS by
// default. Set Config.Plaintext only when a plain SQLite database is required.
func Open(path string, cfg Config) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("数据库路径不能为空")
	}
	if isMemoryDSN(path) {
		return openPlaintext(path)
	}
	if !cfg.Encrypted() {
		if encrypted, err := IsEncrypted(path); err == nil && encrypted {
			return nil, ErrEncryptedWithoutKey
		}
		return openPlaintext(path)
	}
	key := cfg.effectiveKey()
	if encrypted, err := IsEncrypted(path); err == nil && !encrypted {
		if err := migratePlaintextToEncrypted(path, key); err != nil {
			return nil, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("检查数据库加密状态失败: %w", err)
	}
	return openEncrypted(path, key)
}

// OpenGorm opens a SQLite database and wraps it with a GORM DB. If GORM
// initialization fails, the underlying sql.DB is closed.
func OpenGorm(path string, cfg Config, gormConfig *gorm.Config) (*gorm.DB, error) {
	sqlDB, err := Open(path, cfg)
	if err != nil {
		return nil, err
	}
	gormDB, err := gorm.Open(NewDialector(sqlDB), gormConfig)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return gormDB, nil
}

// IsEncrypted checks the SQLite file header. Non-existent files are treated as
// not encrypted because the encryption mode is determined during creation.
func IsEncrypted(path string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("数据库路径不能为空")
	}
	if isMemoryDSN(path) || strings.HasPrefix(path, "file:") {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("数据库路径是目录: %s", path)
	}
	if info.Size() == 0 {
		return false, nil
	}
	header := make([]byte, len(sqliteHeader))
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	n, err := file.Read(header)
	if err != nil {
		return false, err
	}
	if n < len(sqliteHeader) {
		return true, nil
	}
	return string(header) != sqliteHeader, nil
}

func (c Config) effectiveKey() string {
	if c.Plaintext {
		return ""
	}
	if key := strings.TrimSpace(c.Key); key != "" {
		return key
	}
	return defaultDatabaseKey
}

func openPlaintext(path string) (*sql.DB, error) {
	db, err := driver.Open(plainDSN(path))
	if err != nil {
		return nil, err
	}
	if err := configure(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openEncrypted(path, key string) (*sql.DB, error) {
	key = strings.TrimSpace(key)
	db, err := driver.Open(encryptedDSN(path), func(conn *sqlite3.Conn) error {
		return applyTextKey(conn, key)
	})
	if err != nil {
		return nil, err
	}
	if err := verifyEncrypted(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库密钥错误或数据库损坏: %w", err)
	}
	if err := configure(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migratePlaintextToEncrypted(path, key string) error {
	key = strings.TrimSpace(key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("检查明文数据库失败: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("数据库路径是目录: %s", path)
	}
	if info.Size() == 0 {
		return os.Remove(path)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".encrypted-*")
	if err != nil {
		return fmt.Errorf("创建加密数据库临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭加密数据库临时文件失败: %w", err)
	}
	_ = os.Remove(tmpPath)

	source, err := sqlite3.Open(plainDSN(path))
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("打开明文数据库失败: %w", err)
	}
	if err := source.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		_ = source.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("检查明文数据库 WAL 失败: %w", err)
	}
	if err := source.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭明文数据库失败: %w", err)
	}

	target, err := sqlite3.Open(encryptedDSN(tmpPath))
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("创建加密数据库失败: %w", err)
	}
	if err := applyTextKey(target, key); err != nil {
		_ = target.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("设置加密数据库密钥失败: %w", err)
	}
	if err := target.Restore("main", plainDSN(path)); err != nil {
		_ = target.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("导出加密数据库失败: %w", err)
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭加密数据库失败: %w", err)
	}

	if db, err := openEncrypted(tmpPath, key); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("验证迁移后的加密数据库失败: %w", err)
	} else if err := db.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭迁移后的加密数据库失败: %w", err)
	}

	backupPath := path + ".plaintext.bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("备份明文数据库失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换加密数据库失败: %w", err)
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	_ = os.Remove(backupPath)
	return nil
}

func plainDSN(path string) string {
	if isMemoryDSN(path) || strings.HasPrefix(path, "file:") {
		return path
	}
	values := url.Values{}
	values.Set("_pragma", "busy_timeout(5000)")
	return "file:" + filepath.ToSlash(path) + "?" + values.Encode()
}

func encryptedDSN(path string) string {
	values := url.Values{}
	values.Set("vfs", encryptedVFS)
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "temp_store(memory)")
	return "file:" + filepath.ToSlash(path) + "?" + values.Encode()
}

func isMemoryDSN(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file::memory:")
}

func applyTextKey(conn *sqlite3.Conn, key string) error {
	quoted, err := quoteSQLiteString(key)
	if err != nil {
		return err
	}
	return conn.Exec(`PRAGMA textkey=` + quoted)
}

func quoteSQLiteString(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", errors.New("数据库密钥不能包含 NUL 字符")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

func verifyEncrypted(db *sql.DB) error {
	var count int
	return db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&count)
}

func configure(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("设置数据库等待超时失败: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("启用数据库外键失败: %w", err)
	}
	return nil
}
