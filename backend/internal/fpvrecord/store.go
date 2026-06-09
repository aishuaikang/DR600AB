// Package fpvrecord persists FPV video viewing records.
package fpvrecord

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dr600ab-api/internal/model"
	"sqlitecrypto"
)

const defaultQueryLimit = 200

// Store persists FPV video records in a local SQLite database.
type Store struct {
	db *sql.DB
}

// Options configures Store database access.
type Options struct {
	DBKey string
}

// QueryOptions controls FPV video record listing.
type QueryOptions struct {
	Limit  int
	Offset int
	Status model.FPVVideoRecordStatus
}

// NewStore opens and initializes an FPV video record SQLite database.
func NewStore(path string, options ...Options) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return &Store{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建 FPV 图传记录目录失败: %w", err)
	}

	db, err := sqlitecrypto.Open(path, sqlitecrypto.Config{Key: storeOptions(options).DBKey})
	if err != nil {
		return nil, fmt.Errorf("打开 FPV 图传记录库失败: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func storeOptions(options []Options) Options {
	if len(options) == 0 {
		return Options{}
	}
	return options[0]
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	if s == nil || s.db == nil {
		return nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS fpv_video_records (
	id TEXT PRIMARY KEY,
	target_id TEXT NOT NULL DEFAULT '',
	serial TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	display_model TEXT NOT NULL DEFAULT '',
	device TEXT NOT NULL DEFAULT '',
	frequency REAL NOT NULL DEFAULT 0,
	rssi REAL NOT NULL DEFAULT 0,
	started_at TEXT NOT NULL,
	ended_at TEXT NOT NULL,
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '',
	frame_count INTEGER NOT NULL DEFAULT 0,
	last_frame_rows INTEGER NOT NULL DEFAULT 0,
	last_frame_cols INTEGER NOT NULL DEFAULT 0,
	last_frame_at TEXT,
	error TEXT NOT NULL DEFAULT '',
	last_record_json TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_fpv_video_records_status_started_at ON fpv_video_records(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_fpv_video_records_started_at ON fpv_video_records(started_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化 FPV 图传记录库失败: %w", err)
	}
	if err := s.ensureColumn("fpv_video_records", "frames_json", "TEXT"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("检查 FPV 图传记录库字段失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("读取 FPV 图传记录库字段失败: %w", err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取 FPV 图传记录库字段失败: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("迁移 FPV 图传记录库字段失败: %w", err)
	}
	return nil
}

// Insert persists a single FPV video record.
func (s *Store) Insert(record model.FPVVideoRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	record = normalizeRecord(record)
	if strings.TrimSpace(record.ID) == "" || record.StartedAt.IsZero() || record.EndedAt.IsZero() {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO fpv_video_records (
			id, target_id, serial, model, display_model, device, frequency, rssi,
			started_at, ended_at, duration_seconds, status, frame_count,
			last_frame_rows, last_frame_cols, last_frame_at, error, last_record_json, frames_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.TargetID,
		record.Serial,
		record.Model,
		record.DisplayModel,
		record.Device,
		record.Frequency,
		record.RSSI,
		formatTime(record.StartedAt),
		formatTime(record.EndedAt),
		record.DurationSeconds,
		string(record.Status),
		record.FrameCount,
		record.LastFrameRows,
		record.LastFrameCols,
		nullableTime(record.LastFrameAt),
		record.Error,
		jsonString(record.LastRecord),
		jsonString(record.Frames),
		formatTime(record.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("写入 FPV 图传记录失败: %w", err)
	}
	return nil
}

// List returns FPV video records ordered by newest start time first.
func (s *Store) List(options QueryOptions) ([]model.FPVVideoRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	query := `SELECT
		id, target_id, serial, model, display_model, device, frequency, rssi,
		started_at, ended_at, duration_seconds, status, frame_count,
		last_frame_rows, last_frame_cols, last_frame_at, error, last_record_json, NULL AS frames_json, created_at
		FROM fpv_video_records`
	if options.Status != "" {
		query += ` WHERE status = ?`
		args = append(args, string(options.Status))
	}
	query += ` ORDER BY started_at DESC, created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询 FPV 图传记录失败: %w", err)
	}
	defer rows.Close()

	items := make([]model.FPVVideoRecord, 0, limit)
	for rows.Next() {
		item, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 FPV 图传记录失败: %w", err)
	}
	return items, nil
}

// Get returns one FPV video record by ID, including archived frame snapshots.
func (s *Store) Get(id string) (model.FPVVideoRecord, bool, error) {
	if s == nil || s.db == nil {
		return model.FPVVideoRecord{}, false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return model.FPVVideoRecord{}, false, nil
	}
	rows, err := s.db.Query(
		`SELECT
		id, target_id, serial, model, display_model, device, frequency, rssi,
		started_at, ended_at, duration_seconds, status, frame_count,
		last_frame_rows, last_frame_cols, last_frame_at, error, last_record_json, frames_json, created_at
		FROM fpv_video_records WHERE id = ?`,
		id,
	)
	if err != nil {
		return model.FPVVideoRecord{}, false, fmt.Errorf("查询 FPV 图传记录失败: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return model.FPVVideoRecord{}, false, nil
	}
	record, err := scanRecord(rows)
	if err != nil {
		return model.FPVVideoRecord{}, false, err
	}
	if err := rows.Err(); err != nil {
		return model.FPVVideoRecord{}, false, fmt.Errorf("读取 FPV 图传记录失败: %w", err)
	}
	return record, true, nil
}

// Delete removes selected FPV video records.
func (s *Store) Delete(ids []string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args[index] = id
	}
	result, err := s.db.Exec(`DELETE FROM fpv_video_records WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("删除 FPV 图传记录失败: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取 FPV 图传记录删除数量失败: %w", err)
	}
	return deleted, nil
}

// PruneBefore removes FPV video records started before cutoff.
func (s *Store) PruneBefore(cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil || cutoff.IsZero() {
		return 0, nil
	}
	result, err := s.db.Exec(`DELETE FROM fpv_video_records WHERE started_at < ?`, formatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("清理过期 FPV 图传记录失败: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取清理 FPV 图传记录数量失败: %w", err)
	}
	return deleted, nil
}

// PruneRetention removes records older than the configured retention days. 0 means keep forever.
func (s *Store) PruneRetention(days int, now time.Time) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	return s.PruneBefore(now.AddDate(0, 0, -days))
}

func normalizeRecord(record model.FPVVideoRecord) model.FPVVideoRecord {
	if strings.TrimSpace(record.ID) == "" {
		record.ID = NewRecordID()
	}
	record.TargetID = strings.TrimSpace(record.TargetID)
	record.Serial = strings.TrimSpace(record.Serial)
	record.Model = strings.TrimSpace(record.Model)
	record.DisplayModel = strings.TrimSpace(record.DisplayModel)
	record.Device = strings.TrimSpace(record.Device)
	record.Error = strings.TrimSpace(record.Error)
	if record.Status == "" {
		record.Status = model.FPVVideoRecordStatusCompleted
	}
	if record.EndedAt.Before(record.StartedAt) {
		record.EndedAt = record.StartedAt
	}
	record.DurationSeconds = int64(record.EndedAt.Sub(record.StartedAt).Seconds())
	if record.DurationSeconds < 0 {
		record.DurationSeconds = 0
	}
	if record.FrameCount < 0 {
		record.FrameCount = 0
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	return record
}

// NewRecordID returns a unique FPV video record ID.
func NewRecordID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "fpv-" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return "fpv-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(random[:])
}

func scanRecord(rows *sql.Rows) (model.FPVVideoRecord, error) {
	var record model.FPVVideoRecord
	var status string
	var startedAt, endedAt, createdAt string
	var lastFrameAt sql.NullString
	var lastRecordJSON sql.NullString
	var framesJSON sql.NullString
	if err := rows.Scan(
		&record.ID,
		&record.TargetID,
		&record.Serial,
		&record.Model,
		&record.DisplayModel,
		&record.Device,
		&record.Frequency,
		&record.RSSI,
		&startedAt,
		&endedAt,
		&record.DurationSeconds,
		&status,
		&record.FrameCount,
		&record.LastFrameRows,
		&record.LastFrameCols,
		&lastFrameAt,
		&record.Error,
		&lastRecordJSON,
		&framesJSON,
		&createdAt,
	); err != nil {
		return model.FPVVideoRecord{}, fmt.Errorf("读取 FPV 图传记录字段失败: %w", err)
	}
	record.Status = model.FPVVideoRecordStatus(status)
	record.StartedAt = parseTime(startedAt)
	record.EndedAt = parseTime(endedAt)
	record.CreatedAt = parseTime(createdAt)
	if lastFrameAt.Valid {
		parsed := parseTime(lastFrameAt.String)
		if !parsed.IsZero() {
			record.LastFrameAt = &parsed
		}
	}
	record.LastRecord = decodeJSONAny(lastRecordJSON)
	record.Frames = decodeFrames(framesJSON)
	return record, nil
}

func normalizeIDs(ids []string) []string {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

func jsonString(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) == "null" {
		return ""
	}
	return string(data)
}

func decodeJSONAny(value sql.NullString) any {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(value.String), &decoded); err != nil {
		return nil
	}
	return decoded
}

func decodeFrames(value sql.NullString) []model.FPVVideoRecordFrame {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var frames []model.FPVVideoRecordFrame
	if err := json.Unmarshal([]byte(value.String), &frames); err != nil {
		return nil
	}
	return frames
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return formatTime(*value)
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// ParseStatus validates a public FPV video record status query value.
func ParseStatus(value string) (model.FPVVideoRecordStatus, error) {
	switch status := model.FPVVideoRecordStatus(strings.TrimSpace(value)); status {
	case "", model.FPVVideoRecordStatusCompleted, model.FPVVideoRecordStatusFailed:
		return status, nil
	default:
		return "", errors.New("invalid fpv video record status")
	}
}
