// Package deceptionreport persists GNSS spoofing evidence reports.
package deceptionreport

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
	"sync"
	"time"

	"dr600ab-api/internal/model"
	"gnss-spoofer/protocol"
	"sqlitecrypto"
)

const defaultQueryLimit = 200

// QueryOptions controls deception report listing.
type QueryOptions struct {
	Limit  int
	Offset int
	Status model.DeceptionReportStatus
}

// Store persists deception reports in a local SQLite database.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Options configures Store database access.
type Options struct {
	DBKey string
}

// NewStore opens and initializes a deception report SQLite database.
func NewStore(path string, options ...Options) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return &Store{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建诱骗报告目录失败: %w", err)
	}

	db, err := sqlitecrypto.Open(path, sqlitecrypto.Config{Key: storeOptions(options).DBKey})
	if err != nil {
		return nil, fmt.Errorf("打开诱骗报告库失败: %w", err)
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
CREATE TABLE IF NOT EXISTS deception_reports (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	target_id TEXT NOT NULL DEFAULT '',
	mode TEXT NOT NULL DEFAULT '',
	point_json TEXT,
	altitude_m REAL NOT NULL DEFAULT 0,
	signal_mask INTEGER NOT NULL DEFAULT 0,
	signal_names_json TEXT,
	strength_preset TEXT NOT NULL DEFAULT '',
	attenuation_db INTEGER NOT NULL DEFAULT 0,
	delay_mode TEXT NOT NULL DEFAULT '',
	delay_ns REAL NOT NULL DEFAULT 0,
	port_name TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	abnormal_reason TEXT NOT NULL DEFAULT '',
	request_json TEXT,
	session_json TEXT,
	start_state_json TEXT,
	end_state_json TEXT,
	start_device_status_json TEXT,
	before_stop_status_json TEXT,
	after_stop_status_json TEXT,
	raw_descriptions_json TEXT,
	query_errors_json TEXT,
	records_json TEXT,
	record_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deception_reports_status_started_at ON deception_reports(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_deception_reports_started_at ON deception_reports(started_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化诱骗报告库失败: %w", err)
	}
	return nil
}

// Create inserts a new deception report.
func (s *Store) Create(report model.DeceptionReport) (model.DeceptionReport, error) {
	if s == nil || s.db == nil {
		return report, nil
	}
	now := time.Now().UTC()
	if strings.TrimSpace(report.ID) == "" {
		report.ID = newReportID()
	}
	if report.Status == "" {
		report.Status = model.DeceptionReportStatusRunning
	}
	if report.StartedAt.IsZero() {
		report.StartedAt = now
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = now
	}
	report.UpdatedAt = now
	report.DurationSeconds = reportDuration(report.StartedAt, report.EndedAt)
	report.RecordCount = len(report.Records)
	report = normalizeReportSummary(report)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.insert(report); err != nil {
		return model.DeceptionReport{}, err
	}
	return report, nil
}

// CreateRunning inserts a newly started deception report.
func (s *Store) CreateRunning(report model.DeceptionReport) (model.DeceptionReport, error) {
	report.Status = model.DeceptionReportStatusRunning
	return s.Create(report)
}

// Update replaces an existing report with the supplied state.
func (s *Store) Update(report model.DeceptionReport) error {
	if s == nil || s.db == nil || strings.TrimSpace(report.ID) == "" {
		return nil
	}
	report.UpdatedAt = time.Now().UTC()
	report.DurationSeconds = reportDuration(report.StartedAt, report.EndedAt)
	report.RecordCount = len(report.Records)
	report = normalizeReportSummary(report)

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(
		`UPDATE deception_reports SET
			status = ?, started_at = ?, ended_at = ?, duration_seconds = ?, target_id = ?, mode = ?,
			point_json = ?, altitude_m = ?, signal_mask = ?, signal_names_json = ?, strength_preset = ?,
			attenuation_db = ?, delay_mode = ?, delay_ns = ?, port_name = ?, summary = ?, last_error = ?,
			abnormal_reason = ?, request_json = ?, session_json = ?, start_state_json = ?, end_state_json = ?,
			start_device_status_json = ?, before_stop_status_json = ?, after_stop_status_json = ?,
			raw_descriptions_json = ?, query_errors_json = ?, records_json = ?, record_count = ?, updated_at = ?
		WHERE id = ?`,
		string(report.Status),
		formatTime(report.StartedAt),
		nullableTime(report.EndedAt),
		report.DurationSeconds,
		report.TargetID,
		report.Mode,
		jsonString(report.Point),
		report.AltitudeM,
		report.SignalMask,
		jsonString(report.SignalNames),
		report.StrengthPreset,
		report.AttenuationDB,
		report.DelayMode,
		report.DelayNS,
		report.PortName,
		report.Summary,
		report.LastError,
		report.AbnormalReason,
		jsonString(report.Request),
		jsonString(report.Session),
		jsonString(report.StartState),
		jsonString(report.EndState),
		jsonString(report.StartDeviceStatus),
		jsonString(report.BeforeStopStatus),
		jsonString(report.AfterStopStatus),
		jsonString(report.RawDescriptions),
		jsonString(report.QueryErrors),
		jsonString(report.Records),
		report.RecordCount,
		formatTime(report.UpdatedAt),
		report.ID,
	)
	if err != nil {
		return fmt.Errorf("更新诱骗报告失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取诱骗报告更新数量失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) insert(report model.DeceptionReport) error {
	_, err := s.db.Exec(
		`INSERT INTO deception_reports (
			id, status, started_at, ended_at, duration_seconds, target_id, mode, point_json,
			altitude_m, signal_mask, signal_names_json, strength_preset, attenuation_db, delay_mode,
			delay_ns, port_name, summary, last_error, abnormal_reason, request_json, session_json,
			start_state_json, end_state_json, start_device_status_json, before_stop_status_json,
			after_stop_status_json, raw_descriptions_json, query_errors_json, records_json, record_count,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID,
		string(report.Status),
		formatTime(report.StartedAt),
		nullableTime(report.EndedAt),
		report.DurationSeconds,
		report.TargetID,
		report.Mode,
		jsonString(report.Point),
		report.AltitudeM,
		report.SignalMask,
		jsonString(report.SignalNames),
		report.StrengthPreset,
		report.AttenuationDB,
		report.DelayMode,
		report.DelayNS,
		report.PortName,
		report.Summary,
		report.LastError,
		report.AbnormalReason,
		jsonString(report.Request),
		jsonString(report.Session),
		jsonString(report.StartState),
		jsonString(report.EndState),
		jsonString(report.StartDeviceStatus),
		jsonString(report.BeforeStopStatus),
		jsonString(report.AfterStopStatus),
		jsonString(report.RawDescriptions),
		jsonString(report.QueryErrors),
		jsonString(report.Records),
		report.RecordCount,
		formatTime(report.CreatedAt),
		formatTime(report.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("写入诱骗报告失败: %w", err)
	}
	return nil
}

// List returns report summaries ordered by newest start time first.
func (s *Store) List(options QueryOptions) ([]model.DeceptionReportSummary, error) {
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
		id, status, started_at, ended_at, duration_seconds, target_id, mode, point_json,
		altitude_m, signal_mask, signal_names_json, strength_preset, attenuation_db, delay_mode,
		delay_ns, port_name, summary, last_error, abnormal_reason, created_at, updated_at
		FROM deception_reports`
	if options.Status != "" {
		query += ` WHERE status = ?`
		args = append(args, string(options.Status))
	}
	query += ` ORDER BY started_at DESC, created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询诱骗报告失败: %w", err)
	}
	defer rows.Close()

	items := make([]model.DeceptionReportSummary, 0, limit)
	for rows.Next() {
		item, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取诱骗报告失败: %w", err)
	}
	return items, nil
}

// Get returns a full deception report by ID.
func (s *Store) Get(id string) (model.DeceptionReport, error) {
	if s == nil || s.db == nil {
		return model.DeceptionReport{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return model.DeceptionReport{}, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT
		id, status, started_at, ended_at, duration_seconds, target_id, mode, point_json,
		altitude_m, signal_mask, signal_names_json, strength_preset, attenuation_db, delay_mode,
		delay_ns, port_name, summary, last_error, abnormal_reason, request_json, session_json,
		start_state_json, end_state_json, start_device_status_json, before_stop_status_json,
		after_stop_status_json, raw_descriptions_json, query_errors_json, records_json, record_count,
		created_at, updated_at
		FROM deception_reports WHERE id = ?`, id)
	report, err := scanReport(row)
	if err != nil {
		return model.DeceptionReport{}, err
	}
	return report, nil
}

// DeleteFailed deletes a failed deception report.
func (s *Store) DeleteFailed(id string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	if err := s.db.QueryRow(`SELECT status FROM deception_reports WHERE id = ?`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if model.DeceptionReportStatus(status) != model.DeceptionReportStatusFailed {
		return 0, ErrNotFailed
	}

	result, err := s.db.Exec(`DELETE FROM deception_reports WHERE id = ? AND status = ?`, id, string(model.DeceptionReportStatusFailed))
	if err != nil {
		return 0, fmt.Errorf("删除诱骗报告失败: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取诱骗报告删除数量失败: %w", err)
	}
	return deleted, nil
}

// CloseRunning marks all still-running reports as abnormal.
func (s *Store) CloseRunning(reason string, now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "abnormal"
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(
		`UPDATE deception_reports SET
			status = ?, ended_at = COALESCE(ended_at, ?),
			duration_seconds = CAST((julianday(COALESCE(ended_at, ?)) - julianday(started_at)) * 86400 AS INTEGER),
			abnormal_reason = CASE WHEN abnormal_reason = '' THEN ? ELSE abnormal_reason END,
			last_error = CASE WHEN last_error = '' THEN ? ELSE last_error END,
			updated_at = ?
		WHERE status = ?`,
		string(model.DeceptionReportStatusAbnormal),
		formatTime(now),
		formatTime(now),
		reason,
		reason,
		formatTime(now),
		string(model.DeceptionReportStatusRunning),
	)
	if err != nil {
		return 0, fmt.Errorf("闭合运行中诱骗报告失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取闭合诱骗报告数量失败: %w", err)
	}
	return affected, nil
}

// ErrNotFound is returned when a report does not exist.
var ErrNotFound = errors.New("deception report not found")

// ErrNotFailed is returned when deleting a report that is not failed.
var ErrNotFailed = errors.New("deception report is not failed")

func scanSummary(rows *sql.Rows) (model.DeceptionReportSummary, error) {
	var summary model.DeceptionReportSummary
	var status, startedAt, createdAt, updatedAt string
	var endedAt sql.NullString
	var pointJSON, signalNamesJSON sql.NullString
	err := rows.Scan(
		&summary.ID,
		&status,
		&startedAt,
		&endedAt,
		&summary.DurationSeconds,
		&summary.TargetID,
		&summary.Mode,
		&pointJSON,
		&summary.AltitudeM,
		&summary.SignalMask,
		&signalNamesJSON,
		&summary.StrengthPreset,
		&summary.AttenuationDB,
		&summary.DelayMode,
		&summary.DelayNS,
		&summary.PortName,
		&summary.Summary,
		&summary.LastError,
		&summary.AbnormalReason,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.DeceptionReportSummary{}, fmt.Errorf("解析诱骗报告摘要失败: %w", err)
	}
	fillSummary(&summary, status, startedAt, endedAt, createdAt, updatedAt, pointJSON, signalNamesJSON)
	return summary, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReport(scanner rowScanner) (model.DeceptionReport, error) {
	var report model.DeceptionReport
	var status, startedAt, createdAt, updatedAt string
	var endedAt sql.NullString
	var pointJSON, signalNamesJSON sql.NullString
	var requestJSON, sessionJSON sql.NullString
	var startStateJSON, endStateJSON sql.NullString
	var startDeviceStatusJSON, beforeStopStatusJSON, afterStopStatusJSON sql.NullString
	var rawDescriptionsJSON, queryErrorsJSON, recordsJSON sql.NullString
	err := scanner.Scan(
		&report.ID,
		&status,
		&startedAt,
		&endedAt,
		&report.DurationSeconds,
		&report.TargetID,
		&report.Mode,
		&pointJSON,
		&report.AltitudeM,
		&report.SignalMask,
		&signalNamesJSON,
		&report.StrengthPreset,
		&report.AttenuationDB,
		&report.DelayMode,
		&report.DelayNS,
		&report.PortName,
		&report.Summary,
		&report.LastError,
		&report.AbnormalReason,
		&requestJSON,
		&sessionJSON,
		&startStateJSON,
		&endStateJSON,
		&startDeviceStatusJSON,
		&beforeStopStatusJSON,
		&afterStopStatusJSON,
		&rawDescriptionsJSON,
		&queryErrorsJSON,
		&recordsJSON,
		&report.RecordCount,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.DeceptionReport{}, ErrNotFound
		}
		return model.DeceptionReport{}, fmt.Errorf("解析诱骗报告失败: %w", err)
	}
	fillSummary(&report.DeceptionReportSummary, status, startedAt, endedAt, createdAt, updatedAt, pointJSON, signalNamesJSON)
	report.Request = decodeJSONValue[model.ScreenDeceptionRequest](requestJSON)
	report.Session = decodeJSONValue[model.DeceptionSessionResponse](sessionJSON)
	report.StartState = decodeJSONPtr[model.ScreenDeceptionState](startStateJSON)
	report.EndState = decodeJSONPtr[model.ScreenDeceptionState](endStateJSON)
	report.StartDeviceStatus = decodeJSONPtr[model.ScreenDeceptionDeviceStatus](startDeviceStatusJSON)
	report.BeforeStopStatus = decodeJSONPtr[model.ScreenDeceptionDeviceStatus](beforeStopStatusJSON)
	report.AfterStopStatus = decodeJSONPtr[model.ScreenDeceptionDeviceStatus](afterStopStatusJSON)
	report.RawDescriptions = decodeJSONMap(rawDescriptionsJSON)
	report.QueryErrors = decodeJSONMap(queryErrorsJSON)
	report.Records = decodeJSONSlice[model.DeceptionRecord](recordsJSON)
	if report.RecordCount == 0 {
		report.RecordCount = len(report.Records)
	}
	return report, nil
}

func fillSummary(
	summary *model.DeceptionReportSummary,
	status string,
	startedAt string,
	endedAt sql.NullString,
	createdAt string,
	updatedAt string,
	pointJSON sql.NullString,
	signalNamesJSON sql.NullString,
) {
	summary.Status = model.DeceptionReportStatus(status)
	summary.StartedAt = parseStoredTime(startedAt)
	summary.EndedAt = parseStoredTimePtr(endedAt)
	summary.CreatedAt = parseStoredTime(createdAt)
	summary.UpdatedAt = parseStoredTime(updatedAt)
	summary.Point = decodeJSONPtr[model.GeoPoint](pointJSON)
	summary.SignalNames = decodeJSONSlice[string](signalNamesJSON)
}

func normalizeReportSummary(report model.DeceptionReport) model.DeceptionReport {
	if report.StartState != nil {
		state := report.StartState
		report.TargetID = strings.TrimSpace(state.TargetID)
		report.Mode = strings.TrimSpace(state.Mode)
		report.Point = cloneGeoPoint(state.Point)
		report.AltitudeM = state.AltitudeM
		report.SignalMask = state.SignalMask
		report.StrengthPreset = strings.TrimSpace(state.StrengthPreset)
		report.AttenuationDB = state.AttenuationDB
		report.DelayMode = strings.TrimSpace(state.DelayMode)
		report.DelayNS = state.DelayNS
	}
	if report.TargetID == "" {
		report.TargetID = strings.TrimSpace(report.Request.TargetID)
	}
	if report.Mode == "" {
		report.Mode = strings.TrimSpace(report.Request.Mode)
		if report.Mode == "" && report.Request.Enabled {
			report.Mode = "fixed_point"
		}
	}
	if report.Point == nil && report.Request.Latitude != nil && report.Request.Longitude != nil {
		report.Point = &model.GeoPoint{
			Latitude:  *report.Request.Latitude,
			Longitude: *report.Request.Longitude,
		}
	}
	if report.AltitudeM == 0 && report.Request.AltitudeM != nil {
		report.AltitudeM = *report.Request.AltitudeM
	}
	if report.SignalMask == 0 && report.Request.SignalMask != nil {
		report.SignalMask = *report.Request.SignalMask
	}
	if report.StrengthPreset == "" {
		report.StrengthPreset = strings.TrimSpace(report.Request.StrengthPreset)
	}
	if report.AttenuationDB == 0 && report.Request.AttenuationDB != nil {
		report.AttenuationDB = *report.Request.AttenuationDB
	}
	if report.DelayMode == "" {
		report.DelayMode = strings.TrimSpace(report.Request.DelayMode)
	}
	if report.DelayNS == 0 && report.Request.DelayNS != nil {
		report.DelayNS = *report.Request.DelayNS
	}
	if report.PortName == "" {
		report.PortName = report.Session.PortName
	}
	if len(report.SignalNames) == 0 {
		report.SignalNames = signalNames(report.SignalMask)
	}
	return report
}

func cloneGeoPoint(point *model.GeoPoint) *model.GeoPoint {
	if point == nil {
		return nil
	}
	cloned := *point
	return &cloned
}

func signalNames(mask uint16) []string {
	return protocol.SignalNames(mask)
}

func newReportID() string {
	var token [8]byte
	if _, err := rand.Read(token[:]); err == nil {
		return "deception-" + strings.ToUpper(hex.EncodeToString(token[:]))
	}
	return fmt.Sprintf("deception-%d", time.Now().UnixNano())
}

func reportDuration(startedAt time.Time, endedAt *time.Time) int64 {
	if startedAt.IsZero() || endedAt == nil || endedAt.Before(startedAt) {
		return 0
	}
	return int64(endedAt.Sub(startedAt).Seconds())
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil || value.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*value), Valid: true}
}

func parseStoredTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func parseStoredTimePtr(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed := parseStoredTime(value.String)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func jsonString(value any) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func decodeJSONValue[T any](raw sql.NullString) T {
	var value T
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return value
	}
	_ = json.Unmarshal([]byte(raw.String), &value)
	return value
}

func decodeJSONPtr[T any](raw sql.NullString) *T {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	var value T
	if err := json.Unmarshal([]byte(raw.String), &value); err != nil {
		return nil
	}
	return &value
}

func decodeJSONSlice[T any](raw sql.NullString) []T {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	var value []T
	if err := json.Unmarshal([]byte(raw.String), &value); err != nil {
		return nil
	}
	return value
}

func decodeJSONMap(raw sql.NullString) map[string]string {
	value := decodeJSONValue[map[string]string](raw)
	if len(value) == 0 {
		return nil
	}
	return value
}

// ParseStatus validates a public deception report status query value.
func ParseStatus(value string) (model.DeceptionReportStatus, error) {
	switch status := model.DeceptionReportStatus(strings.TrimSpace(value)); status {
	case "", model.DeceptionReportStatusRunning, model.DeceptionReportStatusCompleted, model.DeceptionReportStatusFailed, model.DeceptionReportStatusAbnormal:
		return status, nil
	default:
		return "", errors.New("invalid deception report status")
	}
}
