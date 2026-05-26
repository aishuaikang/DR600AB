package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var gpioErrorPrefixPattern = regexp.MustCompile(`^(?:导出 GPIO\d+\s*(?:后等待就绪)?失败|取消导出 GPIO\d+ 失败|设置 GPIO\d+ 为输出模式失败|(?:读取|写入|检查|解析) GPIO\d+/\S+ 失败):\s*`)

var (
	defaultInterferenceBandLabelsByID = map[string]string{
		"io1": "433M/800M/900M/1.4G",
		"io2": "1.2G/1.5G",
		"io3": "2.4G/5.2G/5.8G",
	}
	defaultInterferenceBandLabelsByGPIO = map[string]string{
		"IOC4": "433M/800M/900M/1.4G",
		"IOC2": "1.2G/1.5G",
		"IOC3": "2.4G/5.2G/5.8G",
	}
)

var (
	intrusionCSVHeaderZh = []string{
		"类型", "型号", "序列号", "频点", "信号", "首次发现", "最后发现", "持续时间",
		"飞手距离", "无人机距离", "速度", "高度", "坐标",
	}
	intrusionCSVHeaderEn = []string{
		"Type", "Model", "Serial", "Frequency", "Signal", "First seen", "Last seen", "Duration",
		"Pilot Distance", "Drone Distance", "Speed", "Height", "Coordinates",
	}
	interferenceReportCSVHeaderZh = []string{"状态", "开始时间", "结束时间", "持续时间", "干扰频段", "设置时长", "错误"}
	interferenceReportCSVHeaderEn = []string{"Status", "Started", "Ended", "Duration", "Interference Bands", "Set Duration", "Error"}
	deceptionReportCSVHeaderZh    = []string{"状态", "开始时间", "结束时间", "持续时间", "模式", "错误"}
	deceptionReportCSVHeaderEn    = []string{"Status", "Started", "Ended", "Duration", "Mode", "Error"}
)

type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type DeviceLocation struct {
	Source    string    `json:"source"`
	Point     *GeoPoint `json:"point,omitempty"`
	UpdatedAt string    `json:"updatedAt,omitempty"`
	Valid     bool      `json:"valid"`
}

type TrackPoint struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Speed     *float64 `json:"speed,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	Time      string   `json:"time"`
}

type IntrusionQuery struct {
	InstallDir string `json:"installDir"`
	TargetType string `json:"targetType,omitempty"`
	DateFrom   string `json:"dateFrom,omitempty"`
	DateTo     string `json:"dateTo,omitempty"`
	Search     string `json:"search,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Locale     string `json:"locale,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
}

type IntrusionRecord struct {
	ID                 string          `json:"id"`
	TargetID           string          `json:"targetId"`
	TargetType         string          `json:"targetType"`
	Model              string          `json:"model,omitempty"`
	DisplayModel       string          `json:"displayModel,omitempty"`
	Serial             string          `json:"serial,omitempty"`
	Device             string          `json:"device,omitempty"`
	Frequency          float64         `json:"frequency,omitempty"`
	RSSI               float64         `json:"rssi,omitempty"`
	FirstSeen          string          `json:"firstSeen"`
	LastSeen           string          `json:"lastSeen"`
	DurationSeconds    int64           `json:"durationSeconds"`
	HitCount           int             `json:"hitCount"`
	Source             string          `json:"source,omitempty"`
	Sources            []string        `json:"sources,omitempty"`
	Cracked            bool            `json:"cracked,omitempty"`
	DeviceLocation     *DeviceLocation `json:"deviceLocation,omitempty"`
	Drone              *GeoPoint       `json:"drone,omitempty"`
	Pilot              *GeoPoint       `json:"pilot,omitempty"`
	Home               *GeoPoint       `json:"home,omitempty"`
	DroneTrajectory    []TrackPoint    `json:"droneTrajectory,omitempty"`
	PilotTrajectory    []TrackPoint    `json:"pilotTrajectory,omitempty"`
	PilotDistanceM     *float64        `json:"pilotDistanceM,omitempty"`
	DroneDistanceM     *float64        `json:"droneDistanceM,omitempty"`
	DroneDirectionDeg  *float64        `json:"droneDirectionDeg,omitempty"`
	DeviceDirectionDeg *float64        `json:"deviceDirectionDeg,omitempty"`
	Height             *float64        `json:"height,omitempty"`
	Altitude           *float64        `json:"altitude,omitempty"`
	Speed              *float64        `json:"speed,omitempty"`
	LastRecordJSON     string          `json:"lastRecordJson,omitempty"`
	ArchivedAt         string          `json:"archivedAt"`
}

type DeceptionReportQuery struct {
	InstallDir string `json:"installDir"`
	Status     string `json:"status,omitempty"`
	Mode       string `json:"mode,omitempty"`
	DateFrom   string `json:"dateFrom,omitempty"`
	DateTo     string `json:"dateTo,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Locale     string `json:"locale,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
}

type DeceptionReportSummary struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	StartedAt       string    `json:"startedAt"`
	EndedAt         string    `json:"endedAt,omitempty"`
	DurationSeconds int64     `json:"durationSeconds"`
	TargetID        string    `json:"targetId,omitempty"`
	Mode            string    `json:"mode,omitempty"`
	Point           *GeoPoint `json:"point,omitempty"`
	AltitudeM       float64   `json:"altitudeM,omitempty"`
	SignalMask      int       `json:"signalMask,omitempty"`
	SignalNames     []string  `json:"signalNames,omitempty"`
	StrengthPreset  string    `json:"strengthPreset,omitempty"`
	AttenuationDB   int       `json:"attenuationDB,omitempty"`
	DelayMode       string    `json:"delayMode,omitempty"`
	DelayNS         float64   `json:"delayNS,omitempty"`
	PortName        string    `json:"portName,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	LastError       string    `json:"lastError,omitempty"`
	AbnormalReason  string    `json:"abnormalReason,omitempty"`
	RecordCount     int       `json:"recordCount,omitempty"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
}

type DeceptionReportDetail struct {
	DeceptionReportSummary
	RequestJSON string `json:"requestJson,omitempty"`
	SessionJSON string `json:"sessionJson,omitempty"`
	RecordsJSON string `json:"recordsJson,omitempty"`
}

type InterferenceReportQuery struct {
	InstallDir string `json:"installDir"`
	Status     string `json:"status,omitempty"`
	DateFrom   string `json:"dateFrom,omitempty"`
	DateTo     string `json:"dateTo,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Locale     string `json:"locale,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
}

type InterferenceReportSummary struct {
	ID                       string   `json:"id"`
	Status                   string   `json:"status"`
	StartedAt                string   `json:"startedAt"`
	EndedAt                  string   `json:"endedAt,omitempty"`
	DurationSeconds          int64    `json:"durationSeconds"`
	RequestedDurationSeconds int      `json:"requestedDurationSeconds,omitempty"`
	ChannelIDs               []string `json:"channelIds,omitempty"`
	ChannelLabels            []string `json:"channelLabels,omitempty"`
	ChannelPins              []int    `json:"channelPins,omitempty"`
	Summary                  string   `json:"summary,omitempty"`
	LastError                string   `json:"lastError,omitempty"`
	AbnormalReason           string   `json:"abnormalReason,omitempty"`
	CreatedAt                string   `json:"createdAt"`
	UpdatedAt                string   `json:"updatedAt"`
}

type ExportResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

func (a *App) ListIntrusions(query IntrusionQuery) ([]IntrusionRecord, error) {
	localPath, cleanup, err := a.snapshotDB(query.InstallDir, "intrusions.db")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return queryIntrusions(localPath, normalizeIntrusionQuery(query))
}

func (a *App) ExportIntrusionsCSV(query IntrusionQuery) (ExportResult, error) {
	records, err := a.ListIntrusions(query)
	if err != nil {
		return ExportResult{}, err
	}
	path := strings.TrimSpace(query.OutputPath)
	if path == "" {
		path, err = a.saveCSVPath("目标入侵列表_" + time.Now().Format("20060102_150405") + ".csv")
		if err != nil {
			return ExportResult{}, err
		}
	}
	if err := writeIntrusionsCSV(path, records, query.Locale); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Count: len(records)}, nil
}

func (a *App) ListDeceptionReports(query DeceptionReportQuery) ([]DeceptionReportSummary, error) {
	localPath, cleanup, err := a.snapshotDB(query.InstallDir, "deception-reports.db")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return queryDeceptionReports(localPath, normalizeDeceptionReportQuery(query))
}

func (a *App) GetDeceptionReport(id string, installDir string) (DeceptionReportDetail, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DeceptionReportDetail{}, errors.New("报告 ID 不能为空")
	}
	localPath, cleanup, err := a.snapshotDB(installDir, "deception-reports.db")
	if err != nil {
		return DeceptionReportDetail{}, err
	}
	defer cleanup()
	return getDeceptionReport(localPath, id)
}

func (a *App) ExportDeceptionReportsCSV(query DeceptionReportQuery) (ExportResult, error) {
	reports, err := a.ListDeceptionReports(query)
	if err != nil {
		return ExportResult{}, err
	}
	path := strings.TrimSpace(query.OutputPath)
	if path == "" {
		path, err = a.saveCSVPath("诱骗报告_" + time.Now().Format("20060102_150405") + ".csv")
		if err != nil {
			return ExportResult{}, err
		}
	}
	if err := writeDeceptionReportsCSV(path, reports, query.Locale); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Count: len(reports)}, nil
}

func (a *App) ListInterferenceReports(query InterferenceReportQuery) ([]InterferenceReportSummary, error) {
	localPath, cleanup, err := a.snapshotDB(query.InstallDir, "interference-reports.db")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return queryInterferenceReports(localPath, normalizeInterferenceReportQuery(query))
}

func (a *App) ExportInterferenceReportsCSV(query InterferenceReportQuery) (ExportResult, error) {
	reports, err := a.ListInterferenceReports(query)
	if err != nil {
		return ExportResult{}, err
	}
	path := strings.TrimSpace(query.OutputPath)
	if path == "" {
		path, err = a.saveCSVPath("干扰报告_" + time.Now().Format("20060102_150405") + ".csv")
		if err != nil {
			return ExportResult{}, err
		}
	}
	if err := writeInterferenceReportsCSV(path, reports, query.Locale); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Count: len(reports)}, nil
}

func (a *App) snapshotDB(installDir, fileName string) (string, func(), error) {
	installDir = a.getInstallDir(installDir)
	remotePath := a.firstExistingRemotePath([]string{
		remoteJoin(installDir, "data", fileName),
		remoteJoin(installDir, "backend", "data", fileName),
	})
	if remotePath == "" {
		return "", func() {}, fmt.Errorf("未找到远程数据库 %s", fileName)
	}
	tmpDir, err := os.MkdirTemp("", "dr600ab-db-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	localPath := filepath.Join(tmpDir, fileName)
	a.emitProgress("db-sync-progress", 0, "同步数据库", "正在复制 "+fileName, "running", 0, nil)
	if ok, err := a.downloadIfExists(remotePath, localPath); err != nil || !ok {
		cleanup()
		if err != nil {
			a.emitProgress("db-sync-progress", 0, "同步数据库", "复制数据库失败", "error", 0, err)
			return "", func() {}, err
		}
		err := fmt.Errorf("未找到远程数据库 %s", remotePath)
		a.emitProgress("db-sync-progress", 0, "同步数据库", "复制数据库失败", "error", 0, err)
		return "", func() {}, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_, _ = a.downloadIfExists(remotePath+suffix, localPath+suffix)
	}
	a.emitProgress("db-sync-progress", 0, "同步数据库", "数据库文件复制完成", "success", 100, nil)
	a.emitProgress("db-sync-progress", 1, "同步数据库", "数据库同步完成", "success", 100, nil)
	return localPath, cleanup, nil
}

func normalizeIntrusionQuery(query IntrusionQuery) IntrusionQuery {
	query.TargetType = strings.TrimSpace(query.TargetType)
	if query.TargetType == "all" {
		query.TargetType = ""
	}
	query.Search = strings.TrimSpace(query.Search)
	if query.Limit <= 0 || query.Limit > 5000 {
		query.Limit = 500
	}
	return query
}

func normalizeDeceptionReportQuery(query DeceptionReportQuery) DeceptionReportQuery {
	query.Status = strings.TrimSpace(query.Status)
	if query.Status == "all" {
		query.Status = ""
	}
	query.Mode = strings.TrimSpace(query.Mode)
	if query.Mode == "all" {
		query.Mode = ""
	}
	if query.Limit <= 0 || query.Limit > 5000 {
		query.Limit = 500
	}
	return query
}

func normalizeInterferenceReportQuery(query InterferenceReportQuery) InterferenceReportQuery {
	query.Status = strings.TrimSpace(query.Status)
	if query.Status == "all" {
		query.Status = ""
	}
	if query.Limit <= 0 || query.Limit > 5000 {
		query.Limit = 500
	}
	return query
}

func queryIntrusions(path string, query IntrusionQuery) ([]IntrusionRecord, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses := []string{"1=1"}
	args := []any{}
	if query.TargetType != "" {
		clauses = append(clauses, "target_type = ?")
		args = append(args, query.TargetType)
	}
	if from := dateStart(query.DateFrom); from != "" {
		clauses = append(clauses, "last_seen >= ?")
		args = append(args, from)
	}
	if to := dateEnd(query.DateTo); to != "" {
		clauses = append(clauses, "first_seen <= ?")
		args = append(args, to)
	}
	if query.Search != "" {
		like := "%" + query.Search + "%"
		clauses = append(clauses, "(target_id LIKE ? OR model LIKE ? OR serial LIKE ? OR device LIKE ? OR source LIKE ? OR sources_json LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	args = append(args, query.Limit)
	rows, err := db.Query(`SELECT id, target_id, target_type, model, serial, device, frequency, rssi,
		first_seen, last_seen, duration_seconds, hit_count, source, sources_json, cracked,
		device_location_json, drone_json, pilot_json, home_json, drone_trajectory_json, pilot_trajectory_json,
		pilot_distance_m, drone_distance_m, drone_direction_deg, device_direction_deg,
		height, altitude, speed, last_record_json, archived_at
		FROM intrusion_records WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY last_seen DESC, archived_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]IntrusionRecord, 0)
	for rows.Next() {
		var record IntrusionRecord
		var cracked int
		var sourcesJSON sql.NullString
		var deviceLocationJSON sql.NullString
		var droneJSON, pilotJSON, homeJSON sql.NullString
		var droneTrajectoryJSON, pilotTrajectoryJSON sql.NullString
		var pilotDistanceM, droneDistanceM, droneDirectionDeg, deviceDirectionDeg sql.NullFloat64
		var height, altitude, speed sql.NullFloat64
		var lastRecordJSON sql.NullString
		if err := rows.Scan(&record.ID, &record.TargetID, &record.TargetType, &record.Model, &record.Serial,
			&record.Device, &record.Frequency, &record.RSSI, &record.FirstSeen, &record.LastSeen,
			&record.DurationSeconds, &record.HitCount, &record.Source, &sourcesJSON, &cracked,
			&deviceLocationJSON, &droneJSON, &pilotJSON, &homeJSON, &droneTrajectoryJSON,
			&pilotTrajectoryJSON, &pilotDistanceM, &droneDistanceM, &droneDirectionDeg,
			&deviceDirectionDeg, &height, &altitude, &speed, &lastRecordJSON, &record.ArchivedAt); err != nil {
			return nil, err
		}
		record.Cracked = cracked != 0
		record.DisplayModel = displayModelName(record.Model)
		record.Sources = normalizeStringSlice(decodeJSONSlice[string](sourcesJSON), record.Source)
		record.DeviceLocation = decodeJSONPtr[DeviceLocation](deviceLocationJSON)
		record.Drone = decodeJSONPtr[GeoPoint](droneJSON)
		record.Pilot = decodeJSONPtr[GeoPoint](pilotJSON)
		record.Home = decodeJSONPtr[GeoPoint](homeJSON)
		record.DroneTrajectory = decodeJSONSlice[TrackPoint](droneTrajectoryJSON)
		record.PilotTrajectory = decodeJSONSlice[TrackPoint](pilotTrajectoryJSON)
		record.PilotDistanceM = floatPtr(pilotDistanceM)
		record.DroneDistanceM = floatPtr(droneDistanceM)
		record.DroneDirectionDeg = floatPtr(droneDirectionDeg)
		record.DeviceDirectionDeg = floatPtr(deviceDirectionDeg)
		record.Height = floatPtr(height)
		record.Altitude = floatPtr(altitude)
		record.Speed = floatPtr(speed)
		record.LastRecordJSON = nullString(lastRecordJSON)
		records = append(records, record)
	}
	return records, rows.Err()
}

func queryDeceptionReports(path string, query DeceptionReportQuery) ([]DeceptionReportSummary, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses := []string{"1=1"}
	args := []any{}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if query.Mode != "" {
		clauses = append(clauses, "mode = ?")
		args = append(args, query.Mode)
	}
	if from := dateStart(query.DateFrom); from != "" {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, from)
	}
	if to := dateEnd(query.DateTo); to != "" {
		clauses = append(clauses, "started_at <= ?")
		args = append(args, to)
	}
	args = append(args, query.Limit)
	rows, err := db.Query(`SELECT id, status, started_at, ended_at, duration_seconds, target_id, mode,
		point_json, altitude_m, signal_mask, signal_names_json, strength_preset, attenuation_db,
		delay_mode, delay_ns, port_name, summary, last_error, abnormal_reason,
		record_count, created_at, updated_at
		FROM deception_reports WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY started_at DESC, created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := make([]DeceptionReportSummary, 0)
	for rows.Next() {
		report, err := scanDeceptionReportSummary(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func getDeceptionReport(path, id string) (DeceptionReportDetail, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return DeceptionReportDetail{}, err
	}
	defer db.Close()
	row := db.QueryRow(`SELECT id, status, started_at, ended_at, duration_seconds, target_id, mode,
		point_json, altitude_m, signal_mask, signal_names_json, strength_preset, attenuation_db,
		delay_mode, delay_ns, port_name, summary, last_error, abnormal_reason,
		record_count, created_at, updated_at, request_json, session_json, records_json
		FROM deception_reports WHERE id = ?`, id)
	var detail DeceptionReportDetail
	var requestJSON, sessionJSON, recordsJSON sql.NullString
	report, err := scanDeceptionReportSummary(row, &requestJSON, &sessionJSON, &recordsJSON)
	if err != nil {
		return DeceptionReportDetail{}, err
	}
	detail.DeceptionReportSummary = report
	detail.RequestJSON = nullString(requestJSON)
	detail.SessionJSON = nullString(sessionJSON)
	detail.RecordsJSON = nullString(recordsJSON)
	return detail, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDeceptionReportSummary(scan scanner, extra ...*sql.NullString) (DeceptionReportSummary, error) {
	var report DeceptionReportSummary
	var endedAt sql.NullString
	var pointJSON, signalNamesJSON sql.NullString
	dest := []any{&report.ID, &report.Status, &report.StartedAt, &endedAt, &report.DurationSeconds,
		&report.TargetID, &report.Mode, &pointJSON, &report.AltitudeM, &report.SignalMask,
		&signalNamesJSON, &report.StrengthPreset, &report.AttenuationDB, &report.DelayMode,
		&report.DelayNS, &report.PortName,
		&report.Summary, &report.LastError, &report.AbnormalReason, &report.RecordCount, &report.CreatedAt,
		&report.UpdatedAt}
	for _, item := range extra {
		dest = append(dest, item)
	}
	if err := scan.Scan(dest...); err != nil {
		return DeceptionReportSummary{}, err
	}
	report.EndedAt = nullString(endedAt)
	report.Point = decodeJSONPtr[GeoPoint](pointJSON)
	report.SignalNames = decodeJSONSlice[string](signalNamesJSON)
	return report, nil
}

func queryInterferenceReports(path string, query InterferenceReportQuery) ([]InterferenceReportSummary, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses := []string{"1=1"}
	args := []any{}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if from := dateStart(query.DateFrom); from != "" {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, from)
	}
	if to := dateEnd(query.DateTo); to != "" {
		clauses = append(clauses, "started_at <= ?")
		args = append(args, to)
	}
	args = append(args, query.Limit)
	rows, err := db.Query(`SELECT id, status, started_at, ended_at, duration_seconds, requested_duration_seconds,
		channel_ids_json, channel_labels_json, channel_pins_json, summary, last_error, abnormal_reason,
		created_at, updated_at
		FROM interference_reports WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY started_at DESC, created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := make([]InterferenceReportSummary, 0)
	for rows.Next() {
		report, err := scanInterferenceReportSummary(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func scanInterferenceReportSummary(scan scanner) (InterferenceReportSummary, error) {
	var report InterferenceReportSummary
	var endedAt sql.NullString
	var channelIDsJSON, channelLabelsJSON, channelPinsJSON sql.NullString
	if err := scan.Scan(
		&report.ID,
		&report.Status,
		&report.StartedAt,
		&endedAt,
		&report.DurationSeconds,
		&report.RequestedDurationSeconds,
		&channelIDsJSON,
		&channelLabelsJSON,
		&channelPinsJSON,
		&report.Summary,
		&report.LastError,
		&report.AbnormalReason,
		&report.CreatedAt,
		&report.UpdatedAt,
	); err != nil {
		return InterferenceReportSummary{}, err
	}
	report.EndedAt = nullString(endedAt)
	report.ChannelIDs = decodeJSONSlice[string](channelIDsJSON)
	report.ChannelLabels = normalizeInterferenceBandLabels(report.ChannelIDs, decodeJSONSlice[string](channelLabelsJSON))
	report.ChannelPins = decodeJSONSlice[int](channelPinsJSON)
	report.LastError = normalizeInterferenceReportError(report.LastError)
	return report, nil
}

func writeIntrusionsCSV(path string, records []IntrusionRecord, locale string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write(csvHeaders(locale, intrusionCSVHeaderZh, intrusionCSVHeaderEn)); err != nil {
		return err
	}
	for _, record := range records {
		if err := writer.Write([]string{
			intrusionTargetTypeLabel(record.TargetType, locale),
			record.DisplayModel,
			record.Serial,
			formatFloat(record.Frequency),
			formatFloat(record.RSSI),
			record.FirstSeen,
			record.LastSeen,
			fmt.Sprintf("%d", record.DurationSeconds),
			formatFloatPtr(record.PilotDistanceM),
			formatFloatPtr(record.DroneDistanceM),
			formatFloatPtr(record.Speed),
			formatFloatPtr(record.Height),
			formatCoordinateSummary(record, locale),
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeDeceptionReportsCSV(path string, reports []DeceptionReportSummary, locale string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write(csvHeaders(locale, deceptionReportCSVHeaderZh, deceptionReportCSVHeaderEn)); err != nil {
		return err
	}
	for _, report := range reports {
		errText := report.LastError
		if errText == "" {
			errText = report.AbnormalReason
		}
		if err := writer.Write([]string{
			reportStatusLabel(report.Status, locale),
			report.StartedAt,
			report.EndedAt,
			fmt.Sprintf("%d", report.DurationSeconds),
			deceptionModeLabel(report.Mode, locale),
			errText,
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeInterferenceReportsCSV(path string, reports []InterferenceReportSummary, locale string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write(csvHeaders(locale, interferenceReportCSVHeaderZh, interferenceReportCSVHeaderEn)); err != nil {
		return err
	}
	for _, report := range reports {
		errText := report.LastError
		if errText == "" {
			errText = report.AbnormalReason
		}
		if err := writer.Write([]string{
			reportStatusLabel(report.Status, locale),
			report.StartedAt,
			report.EndedAt,
			fmt.Sprintf("%d", report.DurationSeconds),
			strings.Join(report.ChannelLabels, "/"),
			fmt.Sprintf("%d", report.RequestedDurationSeconds),
			errText,
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func dateStart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func dateEnd(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ""
	}
	return t.Add(24*time.Hour - time.Nanosecond).UTC().Format(time.RFC3339Nano)
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

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func formatPoint(point *GeoPoint) string {
	if point == nil {
		return ""
	}
	return fmt.Sprintf("%.6f, %.6f", point.Latitude, point.Longitude)
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func formatFloatPtr(value *float64) string {
	if value == nil {
		return ""
	}
	return formatFloat(*value)
}

func formatCoordinateSummary(record IntrusionRecord, locale string) string {
	labels := coordinateLabels(locale)
	parts := []string{}
	if record.DeviceLocation != nil && record.DeviceLocation.Point != nil {
		parts = append(parts, labels.device+": "+formatPoint(record.DeviceLocation.Point))
	}
	if record.TargetType == "position" {
		if record.Drone != nil {
			parts = append(parts, labels.drone+": "+formatPoint(record.Drone))
		}
		if record.Pilot != nil {
			parts = append(parts, labels.pilot+": "+formatPoint(record.Pilot))
		}
		if record.Home != nil {
			parts = append(parts, labels.home+": "+formatPoint(record.Home))
		}
	}
	return strings.Join(parts, " / ")
}

func coordinateLabels(locale string) struct {
	device string
	drone  string
	pilot  string
	home   string
} {
	if isEnglishLocale(locale) {
		return struct {
			device string
			drone  string
			pilot  string
			home   string
		}{device: "Device", drone: "Drone", pilot: "Pilot", home: "Home"}
	}
	return struct {
		device string
		drone  string
		pilot  string
		home   string
	}{device: "设备", drone: "无人机", pilot: "飞手", home: "返航点"}
}

func csvHeaders(locale string, zh []string, en []string) []string {
	if isEnglishLocale(locale) {
		return en
	}
	return zh
}

func isEnglishLocale(locale string) bool {
	locale = strings.ToLower(strings.TrimSpace(locale))
	return strings.HasPrefix(locale, "en")
}

func intrusionTargetTypeLabel(value, locale string) string {
	switch value {
	case "detection":
		if isEnglishLocale(locale) {
			return "Detection"
		}
		return "侦测"
	case "position":
		if isEnglishLocale(locale) {
			return "Positioning"
		}
		return "定位"
	default:
		return value
	}
}

func reportStatusLabel(value, locale string) string {
	english := isEnglishLocale(locale)
	switch value {
	case "running":
		if english {
			return "Running"
		}
		return "运行中"
	case "completed":
		if english {
			return "Completed"
		}
		return "已完成"
	case "failed":
		if english {
			return "Failed"
		}
		return "启动失败"
	case "abnormal":
		if english {
			return "Abnormal"
		}
		return "异常闭合"
	default:
		return value
	}
}

func deceptionModeLabel(value, locale string) string {
	english := isEnglishLocale(locale)
	switch value {
	case "fixed_point":
		if english {
			return "Fixed point"
		}
		return "定点诱骗"
	case "circle":
		if english {
			return "Circle"
		}
		return "圆周诱骗"
	case "linear":
		if english {
			return "Linear"
		}
		return "线性诱骗"
	default:
		return value
	}
}

func normalizeStringSlice(values []string, fallback string) []string {
	result := make([]string, 0, len(values)+1)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsString(result, value) {
			continue
		}
		result = append(result, value)
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" && !containsString(result, fallback) {
		result = append(result, fallback)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func displayModelName(modelName string) string {
	modelName = normalizeDisplayModelName(modelName)
	if modelName == "" {
		return ""
	}
	if displayName, ok := uavModelDisplayNames[modelName]; ok {
		return displayName
	}
	return modelName
}

func normalizeDisplayModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	prefix, suffix, ok := strings.Cut(modelName, "-")
	if !ok {
		return modelName
	}
	prefix = strings.TrimSpace(prefix)
	suffix = strings.TrimSpace(suffix)
	if prefix == "" || suffix == "" || !isDecimalString(prefix) {
		return modelName
	}
	return suffix
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var uavModelDisplayNames = map[string]string{
	"Lightbridge_type1": "DJI P4/Spark",
	"Lightbridge_type2": "Autel Evo2V2/Feimi",
	"eWifi_5M":          "DJI Mini1/Air1",
	"eWifi_10M":         "HUBSAN Zino/HUBSAN Eagle",
	"PAL Analog":        "Analog PAL",
	"NTSC Analog":       "Analog NTSC",
	"DJI_OC123_10M":     "DJI-O1/O2/O3 Series Mini 2,3, 4k, Air 2, 3, Mavic 2,3, Avata, P4-2.0",
	"DJI_OC123_20M":     "DJI-O1/O2/O3 Series Mini 2,3, 4k,Air 2, 3, Mavic 2,3, Avata, P4-2.0",
	"DJI_O4_type":       "DJI-O4 Series Mini4, Air3s, Avata2, Neo",
	"Autel_type1":       "Autel nano/nano+/ lite/lite+",
	"Autel_type2":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Autel_type3":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Autel_type4":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Datalink_type1":    "P900/P840/Mavlink",
	"Datalink_type2":    "P900/P840/Mavlink",
	"Datalink_type3":    "DJI/Autel remote",
	"LTE_type0":         "LTE-image feed",
	"LORA":              "LoRa drone",
	"Walksnail":         "Walksnail drone",
	"DJI_O3+":           "Mavic 3 series",
	"O3+_ofdm_datalink": "Drone/RC",
}

func normalizeInterferenceBandLabels(ids []string, labels []string) []string {
	if len(ids) == 0 && len(labels) == 0 {
		return nil
	}
	count := len(labels)
	if len(ids) > count {
		count = len(ids)
	}
	normalized := make([]string, 0, count)
	for index := 0; index < count; index++ {
		id := ""
		if index < len(ids) {
			id = ids[index]
		}
		label := ""
		if index < len(labels) {
			label = strings.TrimSpace(labels[index])
		}
		if mapped := defaultInterferenceBandLabel(id, label); mapped != "" {
			normalized = append(normalized, mapped)
			continue
		}
		if label != "" {
			normalized = append(normalized, label)
			continue
		}
		if id != "" {
			normalized = append(normalized, id)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func defaultInterferenceBandLabel(id string, label string) string {
	label = strings.TrimSpace(label)
	if label != "" {
		if mapped := defaultInterferenceBandLabelsByGPIO[label]; mapped != "" {
			return mapped
		}
		return label
	}
	return defaultInterferenceBandLabelsByID[strings.TrimSpace(id)]
}

func normalizeInterferenceReportError(message string) string {
	message = strings.TrimSpace(message)
	for {
		next := strings.TrimSpace(message)
		for _, prefix := range []string{
			"更新 GPIO 状态失败:",
			"Failed to update GPIO state:",
		} {
			next = strings.TrimSpace(strings.TrimPrefix(next, prefix))
		}
		next = strings.TrimSpace(gpioErrorPrefixPattern.ReplaceAllString(next, ""))
		if next == message {
			return next
		}
		message = next
	}
}
