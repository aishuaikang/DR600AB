// Package intrusion persists disappeared screen targets as intrusion records.
package intrusion

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"dr600ab-api/internal/model"

	_ "modernc.org/sqlite"
)

const defaultQueryLimit = 200

// Store persists intrusion records in a local SQLite database.
type Store struct {
	db                     *sql.DB
	mu                     sync.RWMutex
	deviceLocationProvider DeviceLocationProvider
}

// DeviceLocationProvider returns the current device location for newly archived records.
type DeviceLocationProvider func() *model.ScreenDeviceLocationResponse

// QueryOptions controls intrusion record listing.
type QueryOptions struct {
	Limit      int
	TargetType model.IntrusionTargetType
}

// NewStore opens and initializes an intrusion SQLite database.
func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return &Store{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建入侵记录目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开入侵记录库失败: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// SetDeviceLocationProvider sets the source used to stamp new intrusion records with device location.
func (s *Store) SetDeviceLocationProvider(provider DeviceLocationProvider) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceLocationProvider = provider
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
CREATE TABLE IF NOT EXISTS intrusion_records (
	id TEXT PRIMARY KEY,
	target_id TEXT NOT NULL,
	target_type TEXT NOT NULL,
	model TEXT NOT NULL DEFAULT '',
	serial TEXT NOT NULL DEFAULT '',
	device TEXT NOT NULL DEFAULT '',
	frequency REAL NOT NULL DEFAULT 0,
	rssi REAL NOT NULL DEFAULT 0,
	first_seen TEXT NOT NULL,
	last_seen TEXT NOT NULL,
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	hit_count INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT '',
	sources_json TEXT,
	cracked INTEGER NOT NULL DEFAULT 0,
	device_location_json TEXT,
	drone_json TEXT,
	pilot_json TEXT,
	home_json TEXT,
	drone_trajectory_json TEXT,
	pilot_trajectory_json TEXT,
	pilot_distance_m REAL,
	drone_distance_m REAL,
	drone_direction_deg REAL,
	device_direction_deg REAL,
	height REAL,
	altitude REAL,
	speed REAL,
	last_record_json TEXT,
	archived_at TEXT NOT NULL,
	UNIQUE(target_type, target_id, first_seen)
);
CREATE INDEX IF NOT EXISTS idx_intrusion_records_type_last_seen ON intrusion_records(target_type, last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_intrusion_records_last_seen ON intrusion_records(last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_intrusion_records_archived_at ON intrusion_records(archived_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化入侵记录库失败: %w", err)
	}
	if err := s.ensureColumn("intrusion_records", "device_location_json", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("intrusion_records", "sources_json", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("intrusion_records", "pilot_distance_m", "REAL"); err != nil {
		return err
	}
	if err := s.ensureColumn("intrusion_records", "drone_distance_m", "REAL"); err != nil {
		return err
	}
	if err := s.ensureColumn("intrusion_records", "drone_direction_deg", "REAL"); err != nil {
		return err
	}
	if err := s.ensureColumn("intrusion_records", "device_direction_deg", "REAL"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("检查入侵记录库字段失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("读取入侵记录库字段失败: %w", err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取入侵记录库字段失败: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("迁移入侵记录库字段失败: %w", err)
	}
	return nil
}

// ArchiveDetection persists an expired detection target.
func (s *Store) ArchiveDetection(target model.ScreenDetectionTarget) error {
	if s == nil || s.db == nil || strings.TrimSpace(target.ID) == "" {
		return nil
	}
	serial := strings.TrimSpace(target.Serial)
	if serial == "" {
		serial = newDetectionSerial()
	}
	displayModel := model.DisplayModelName(target.Model)
	targetDisplayModel := model.DisplayModelName(target.LastRecord.Model)
	if targetDisplayModel == "" {
		targetDisplayModel = displayModel
	}
	target.LastRecord.DisplayModel = targetDisplayModel
	record := model.IntrusionRecord{
		ID:              intrusionRecordID(model.IntrusionTargetTypeDetection, target.ID, target.FirstSeen),
		TargetID:        target.ID,
		TargetType:      model.IntrusionTargetTypeDetection,
		Model:           strings.TrimSpace(target.Model),
		DisplayModel:    displayModel,
		Serial:          serial,
		Device:          strings.TrimSpace(target.Device),
		Frequency:       target.Frequency,
		RSSI:            target.RSSI,
		FirstSeen:       target.FirstSeen,
		LastSeen:        target.LastSeen,
		DurationSeconds: durationSeconds(target.FirstSeen, target.LastSeen),
		HitCount:        target.HitCount,
		DeviceLocation:  s.currentDeviceLocation(),
		LastRecord:      target.LastRecord,
		ArchivedAt:      time.Now(),
	}
	return s.insert(record)
}

// ArchivePosition persists an expired position target.
func (s *Store) ArchivePosition(target model.ScreenPositionTarget) error {
	if s == nil || s.db == nil || strings.TrimSpace(target.ID) == "" {
		return nil
	}
	deviceLocation := s.currentDeviceLocation()
	pilotDistanceM, droneDistanceM, droneDirectionDeg, deviceDirectionDeg := model.ScreenPositionRelations(
		deviceLocation,
		target.Drone,
		target.Pilot,
	)
	record := model.IntrusionRecord{
		ID:                 intrusionRecordID(model.IntrusionTargetTypePosition, target.ID, target.FirstSeen),
		TargetID:           target.ID,
		TargetType:         model.IntrusionTargetTypePosition,
		Model:              strings.TrimSpace(target.Model),
		Serial:             strings.TrimSpace(target.Serial),
		Device:             strings.TrimSpace(target.Device),
		Frequency:          target.Frequency,
		RSSI:               target.RSSI,
		FirstSeen:          target.FirstSeen,
		LastSeen:           target.LastSeen,
		DurationSeconds:    durationSeconds(target.FirstSeen, target.LastSeen),
		HitCount:           target.HitCount,
		Source:             strings.TrimSpace(target.Source),
		Sources:            cloneStrings(normalizeSources(target.Sources, target.Source)),
		Cracked:            target.Cracked,
		DeviceLocation:     deviceLocation,
		Drone:              clonePoint(target.Drone),
		Pilot:              clonePoint(target.Pilot),
		Home:               clonePoint(target.Home),
		DroneTrajectory:    cloneTrajectory(target.DroneTrajectory),
		PilotTrajectory:    cloneTrajectory(target.PilotTrajectory),
		PilotDistanceM:     cloneFloat(pilotDistanceM),
		DroneDistanceM:     cloneFloat(droneDistanceM),
		DroneDirectionDeg:  cloneFloat(droneDirectionDeg),
		DeviceDirectionDeg: cloneFloat(deviceDirectionDeg),
		Height:             cloneFloat(target.Height),
		Altitude:           cloneFloat(target.Altitude),
		Speed:              cloneFloat(target.Speed),
		LastRecord:         target.LastRecord,
		ArchivedAt:         time.Now(),
	}
	return s.insert(record)
}

func (s *Store) insert(record model.IntrusionRecord) error {
	if record.FirstSeen.IsZero() || record.LastSeen.IsZero() {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO intrusion_records (
			id, target_id, target_type, model, serial, device, frequency, rssi,
			first_seen, last_seen, duration_seconds, hit_count, source, sources_json, cracked,
			device_location_json, drone_json, pilot_json, home_json, drone_trajectory_json, pilot_trajectory_json,
			pilot_distance_m, drone_distance_m, drone_direction_deg, device_direction_deg,
			height, altitude, speed, last_record_json, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.TargetID,
		string(record.TargetType),
		record.Model,
		record.Serial,
		record.Device,
		record.Frequency,
		record.RSSI,
		formatTime(record.FirstSeen),
		formatTime(record.LastSeen),
		record.DurationSeconds,
		record.HitCount,
		record.Source,
		jsonString(record.Sources),
		boolInt(record.Cracked),
		jsonString(record.DeviceLocation),
		jsonString(record.Drone),
		jsonString(record.Pilot),
		jsonString(record.Home),
		jsonString(record.DroneTrajectory),
		jsonString(record.PilotTrajectory),
		nullableFloat(record.PilotDistanceM),
		nullableFloat(record.DroneDistanceM),
		nullableFloat(record.DroneDirectionDeg),
		nullableFloat(record.DeviceDirectionDeg),
		nullableFloat(record.Height),
		nullableFloat(record.Altitude),
		nullableFloat(record.Speed),
		jsonString(record.LastRecord),
		formatTime(record.ArchivedAt),
	)
	if err != nil {
		return fmt.Errorf("写入入侵记录失败: %w", err)
	}
	return nil
}

// List returns intrusion records ordered by latest disappearance first.
func (s *Store) List(options QueryOptions) ([]model.IntrusionRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	args := []any{}
	query := `SELECT id, target_id, target_type, model, serial, device, frequency, rssi,
		first_seen, last_seen, duration_seconds, hit_count, source, sources_json, cracked,
		device_location_json, drone_json, pilot_json, home_json, drone_trajectory_json, pilot_trajectory_json,
		pilot_distance_m, drone_distance_m, drone_direction_deg, device_direction_deg,
		height, altitude, speed, last_record_json, archived_at
		FROM intrusion_records`
	if options.TargetType != "" {
		query += ` WHERE target_type = ?`
		args = append(args, string(options.TargetType))
	}
	query += ` ORDER BY last_seen DESC, archived_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询入侵记录失败: %w", err)
	}
	defer rows.Close()

	records := make([]model.IntrusionRecord, 0, limit)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取入侵记录失败: %w", err)
	}
	return records, nil
}

// Delete removes intrusion records by ID and returns the number of deleted rows.
func (s *Store) Delete(ids []string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, errors.New("empty intrusion record ids")
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args[index] = id
	}
	result, err := s.db.Exec(
		`DELETE FROM intrusion_records WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("删除入侵记录失败: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取删除入侵记录数量失败: %w", err)
	}
	return deleted, nil
}

// PruneBefore removes intrusion records archived before cutoff.
func (s *Store) PruneBefore(cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil || cutoff.IsZero() {
		return 0, nil
	}
	result, err := s.db.Exec(`DELETE FROM intrusion_records WHERE archived_at < ?`, formatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("清理过期入侵记录失败: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取清理入侵记录数量失败: %w", err)
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

func scanRecord(rows *sql.Rows) (model.IntrusionRecord, error) {
	var record model.IntrusionRecord
	var targetType string
	var cracked int
	var firstSeen, lastSeen, archivedAt string
	var sourcesJSON sql.NullString
	var deviceLocationJSON sql.NullString
	var droneJSON, pilotJSON, homeJSON sql.NullString
	var droneTrajectoryJSON, pilotTrajectoryJSON sql.NullString
	var pilotDistanceM, droneDistanceM, droneDirectionDeg, deviceDirectionDeg sql.NullFloat64
	var height, altitude, speed sql.NullFloat64
	var lastRecordJSON sql.NullString

	err := rows.Scan(
		&record.ID,
		&record.TargetID,
		&targetType,
		&record.Model,
		&record.Serial,
		&record.Device,
		&record.Frequency,
		&record.RSSI,
		&firstSeen,
		&lastSeen,
		&record.DurationSeconds,
		&record.HitCount,
		&record.Source,
		&sourcesJSON,
		&cracked,
		&deviceLocationJSON,
		&droneJSON,
		&pilotJSON,
		&homeJSON,
		&droneTrajectoryJSON,
		&pilotTrajectoryJSON,
		&pilotDistanceM,
		&droneDistanceM,
		&droneDirectionDeg,
		&deviceDirectionDeg,
		&height,
		&altitude,
		&speed,
		&lastRecordJSON,
		&archivedAt,
	)
	if err != nil {
		return model.IntrusionRecord{}, fmt.Errorf("解析入侵记录失败: %w", err)
	}

	record.TargetType = model.IntrusionTargetType(targetType)
	record.Cracked = cracked != 0
	record.FirstSeen = parseStoredTime(firstSeen)
	record.LastSeen = parseStoredTime(lastSeen)
	record.ArchivedAt = parseStoredTime(archivedAt)
	record.Sources = normalizeSources(decodeJSONSlice[string](sourcesJSON), record.Source)
	record.DeviceLocation = decodeJSONPtr[model.ScreenDeviceLocationResponse](deviceLocationJSON)
	record.Drone = decodeJSONPtr[model.ScreenPositionPoint](droneJSON)
	record.Pilot = decodeJSONPtr[model.ScreenPositionPoint](pilotJSON)
	record.Home = decodeJSONPtr[model.ScreenPositionPoint](homeJSON)
	record.DroneTrajectory = decodeJSONSlice[model.ScreenPositionTrackPoint](droneTrajectoryJSON)
	record.PilotTrajectory = decodeJSONSlice[model.ScreenPositionTrackPoint](pilotTrajectoryJSON)
	record.PilotDistanceM = floatPtr(pilotDistanceM)
	record.DroneDistanceM = floatPtr(droneDistanceM)
	record.DroneDirectionDeg = floatPtr(droneDirectionDeg)
	record.DeviceDirectionDeg = floatPtr(deviceDirectionDeg)
	record.Height = floatPtr(height)
	record.Altitude = floatPtr(altitude)
	record.Speed = floatPtr(speed)
	record.LastRecord = decodeLastRecord(record.TargetType, lastRecordJSON)
	if record.TargetType == model.IntrusionTargetTypeDetection {
		record.DisplayModel = model.DisplayModelName(record.Model)
	}
	return record, nil
}

func normalizeIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(result, id) {
			continue
		}
		result = append(result, id)
	}
	return result
}

func intrusionRecordID(targetType model.IntrusionTargetType, targetID string, firstSeen time.Time) string {
	return fmt.Sprintf("intrusion-%s-%s-%d", targetType, strings.TrimSpace(targetID), firstSeen.UnixNano())
}

func newDetectionSerial() string {
	var token [6]byte
	if _, err := rand.Read(token[:]); err == nil {
		return "DET-" + strings.ToUpper(hex.EncodeToString(token[:]))
	}
	return fmt.Sprintf("DET-%d", time.Now().UnixNano())
}

func durationSeconds(firstSeen, lastSeen time.Time) int64 {
	if firstSeen.IsZero() || lastSeen.IsZero() || lastSeen.Before(firstSeen) {
		return 0
	}
	return int64(lastSeen.Sub(firstSeen).Seconds())
}

func (s *Store) currentDeviceLocation() *model.ScreenDeviceLocationResponse {
	s.mu.RLock()
	provider := s.deviceLocationProvider
	s.mu.RUnlock()
	if provider == nil {
		return nil
	}
	return cloneDeviceLocation(provider())
}

func clonePoint(point *model.ScreenPositionPoint) *model.ScreenPositionPoint {
	if point == nil {
		return nil
	}
	cloned := *point
	return &cloned
}

func cloneDeviceLocation(location *model.ScreenDeviceLocationResponse) *model.ScreenDeviceLocationResponse {
	if location == nil || !location.Valid || !validGeoPoint(location.Point) {
		return nil
	}
	cloned := *location
	point := *location.Point
	cloned.Point = &point
	if location.UpdatedAt != nil {
		updatedAt := *location.UpdatedAt
		cloned.UpdatedAt = &updatedAt
	}
	return &cloned
}

func normalizeSources(values []string, fallback string) []string {
	result := make([]string, 0, len(values)+1)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slicesContains(result, value) {
			continue
		}
		result = append(result, value)
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" && !slicesContains(result, fallback) {
		result = append(result, fallback)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func slicesContains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func validGeoPoint(point *model.GeoPoint) bool {
	if point == nil {
		return false
	}
	return !math.IsNaN(point.Latitude) &&
		!math.IsInf(point.Latitude, 0) &&
		!math.IsNaN(point.Longitude) &&
		!math.IsInf(point.Longitude, 0) &&
		point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180 &&
		!(point.Latitude == 0 && point.Longitude == 0)
}

func cloneTrajectory(points []model.ScreenPositionTrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) == 0 {
		return nil
	}
	cloned := make([]model.ScreenPositionTrackPoint, len(points))
	copy(cloned, points)
	return cloned
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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

func nullableFloat(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseStoredTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
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

func decodeLastRecord(targetType model.IntrusionTargetType, raw sql.NullString) any {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	switch targetType {
	case model.IntrusionTargetTypeDetection:
		var value model.ScreenDetectionLastRecord
		if err := json.Unmarshal([]byte(raw.String), &value); err == nil {
			return value
		}
	case model.IntrusionTargetTypePosition:
		var value model.ScreenPositionLastRecord
		if err := json.Unmarshal([]byte(raw.String), &value); err == nil {
			return value
		}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw.String), &value); err == nil {
		return value
	}
	return nil
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func validTargetType(value string) (model.IntrusionTargetType, bool) {
	switch model.IntrusionTargetType(strings.TrimSpace(value)) {
	case "", model.IntrusionTargetTypeDetection, model.IntrusionTargetTypePosition:
		return model.IntrusionTargetType(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

// ParseTargetType validates a public target type query value.
func ParseTargetType(value string) (model.IntrusionTargetType, error) {
	targetType, ok := validTargetType(value)
	if !ok {
		return "", errors.New("invalid intrusion target type")
	}
	return targetType, nil
}
