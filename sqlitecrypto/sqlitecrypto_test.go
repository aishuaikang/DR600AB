package sqlitecrypto

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

type testItem struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

func TestConfigFromEnvUsesDefaultWhenUnset(t *testing.T) {
	t.Setenv("APP_DATABASE_KEY", "")
	t.Setenv("APP_DATABASE_KEY_FILE", "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.Key != "" {
		t.Fatalf("Key = %q, want empty default-key marker", cfg.Key)
	}
}

func TestConfigFromEnvReadsKeyFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "database.key")
	if err := os.WriteFile(keyPath, []byte(" file-key \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("APP_DATABASE_KEY", "")
	t.Setenv("APP_DATABASE_KEY_FILE", keyPath)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.Key != "file-key" {
		t.Fatalf("Key = %q, want file-key", cfg.Key)
	}
}

func TestConfigFromEnvDirectKeyWinsOverFile(t *testing.T) {
	t.Setenv("APP_DATABASE_KEY", "direct-key")
	t.Setenv("APP_DATABASE_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.Key != "direct-key" {
		t.Fatalf("Key = %q, want direct-key", cfg.Key)
	}
}

func TestConfigFromEnvErrorsWhenKeyFileMissing(t *testing.T) {
	t.Setenv("APP_DATABASE_KEY", "")
	t.Setenv("APP_DATABASE_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))

	if _, err := ConfigFromEnv(); err == nil {
		t.Fatalf("ConfigFromEnv() error = nil, want error")
	}
}

func TestConfigFromEnvErrorsWhenKeyFileEmpty(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "database.key")
	if err := os.WriteFile(keyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("APP_DATABASE_KEY", "")
	t.Setenv("APP_DATABASE_KEY_FILE", keyPath)

	if _, err := ConfigFromEnv(); err == nil {
		t.Fatalf("ConfigFromEnv() error = nil, want error")
	}
}

func TestOpenUsesDefaultKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default-key.db")
	db, err := Open(path, Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db error = %v", err)
	}

	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatalf("IsEncrypted() error = %v", err)
	}
	if !encrypted {
		t.Fatalf("default database is not encrypted")
	}
	if _, err := Open(path, Config{Key: "wrong-key"}); err == nil {
		t.Fatalf("Open() with wrong key error = nil, want error")
	}
}

func TestOpenPlaintextDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.db")
	db, err := Open(path, Config{Plaintext: true})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table error = %v", err)
	}
	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatalf("IsEncrypted() error = %v", err)
	}
	if encrypted {
		t.Fatalf("plaintext database reported as encrypted")
	}
}

func TestOpenEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.db")
	cfg := Config{Key: "test-db-key"}
	db, err := Open(path, cfg)
	if err != nil {
		t.Fatalf("Open(encrypted) error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create encrypted table error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close encrypted db error = %v", err)
	}

	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatalf("IsEncrypted() error = %v", err)
	}
	if !encrypted {
		t.Fatalf("encrypted database reported as plaintext")
	}

	if db, err = Open(path, cfg); err != nil {
		t.Fatalf("reopen encrypted db error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close encrypted db error = %v", err)
	}
	if _, err := Open(path, Config{Key: "wrong-key"}); err == nil {
		t.Fatalf("Open() with wrong key error = nil, want error")
	}
	if _, err := Open(path, Config{Plaintext: true}); !errors.Is(err, ErrEncryptedWithoutKey) {
		t.Fatalf("Open() without key error = %v, want ErrEncryptedWithoutKey", err)
	}
}

func TestOpenMigratesPlaintextDatabaseWhenKeyConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.db")
	db, err := Open(path, Config{Plaintext: true})
	if err != nil {
		t.Fatalf("Open(plain) error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY); INSERT INTO items VALUES ('one')`); err != nil {
		t.Fatalf("seed plaintext db error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close plaintext db error = %v", err)
	}

	key := "test-db-key"
	db, err = Open(path, Config{Key: key})
	if err != nil {
		t.Fatalf("Open(encrypted) migration error = %v", err)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM items`).Scan(&id); err != nil {
		t.Fatalf("query migrated db error = %v", err)
	}
	if id != "one" {
		t.Fatalf("id = %q, want one", id)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close migrated db error = %v", err)
	}

	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatalf("IsEncrypted() error = %v", err)
	}
	if !encrypted {
		t.Fatalf("database was not migrated to encrypted")
	}
	if _, err := Open(path, Config{Plaintext: true}); !errors.Is(err, ErrEncryptedWithoutKey) {
		t.Fatalf("Open() without key error = %v, want ErrEncryptedWithoutKey", err)
	}
}

func TestOpenGormEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gorm.db")
	cfg := Config{Key: "test-db-key"}

	db, err := OpenGorm(path, cfg, &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("OpenGorm() error = %v", err)
	}
	if err := db.AutoMigrate(&testItem{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.Create(&testItem{ID: "one", Name: "first"}).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := db.Create(&testItem{ID: "one", Name: "duplicate"}).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate error = %v, want ErrDuplicatedKey", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err = OpenGorm(path, cfg, &gorm.Config{})
	if err != nil {
		t.Fatalf("OpenGorm(reopen) error = %v", err)
	}
	var item testItem
	if err := db.First(&item, "id = ?", "one").Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if item.Name != "first" {
		t.Fatalf("Name = %q, want first", item.Name)
	}
	sqlDB, err = db.DB()
	if err != nil {
		t.Fatalf("DB() reopen error = %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close() reopen error = %v", err)
	}
}
