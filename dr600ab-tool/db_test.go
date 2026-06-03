package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"sqlitecrypto"
)

func TestQueryIntrusionsAndCSV(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "intrusions.db")
	db, err := sqlitecrypto.Open(dbPath, sqlitecrypto.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE intrusion_records (
		id TEXT, target_id TEXT, target_type TEXT, model TEXT, serial TEXT, device TEXT,
		frequency REAL, rssi REAL, first_seen TEXT, last_seen TEXT, duration_seconds INTEGER,
		hit_count INTEGER, source TEXT, sources_json TEXT, cracked INTEGER, device_location_json TEXT,
		drone_json TEXT, pilot_json TEXT, home_json TEXT, drone_trajectory_json TEXT, pilot_trajectory_json TEXT,
		pilot_distance_m REAL, drone_distance_m REAL, drone_direction_deg REAL, device_direction_deg REAL,
		height REAL, altitude REAL, speed REAL, last_record_json TEXT, archived_at TEXT
	);
	INSERT INTO intrusion_records VALUES (
		'id-1', 'target-1', 'position', 'Mavic', 'SN1', 'dev', 5745, -61,
		'2026-05-24T01:00:00Z', '2026-05-24T01:03:00Z', 180, 3, 'rid', '["rid","wifi"]',
		1, '{"source":"manual","point":{"latitude":39.0,"longitude":116.0},"valid":true}',
		'{"latitude":39.1,"longitude":116.1}', '', '{"latitude":39.2,"longitude":116.2}',
		'[{"latitude":39.1,"longitude":116.1,"time":"2026-05-24T01:01:00Z"}]', '',
		1200.5, 800.25, 90.5, 180.75, 120.3, 180.4, 15.2, '{"type":"position"}',
		'2026-05-24T01:04:00Z'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := queryIntrusions(dbPath, normalizeIntrusionQuery(IntrusionQuery{TargetType: "position", Search: "Mavic"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Drone == nil || records[0].Drone.Latitude != 39.1 {
		t.Fatalf("drone point not decoded: %+v", records[0].Drone)
	}
	if len(records[0].Sources) != 2 || records[0].DroneDistanceM == nil || *records[0].DroneDistanceM != 800.25 {
		t.Fatalf("extended fields not decoded: %+v", records[0])
	}

	csvPath := filepath.Join(t.TempDir(), "intrusions.csv")
	if err := writeIntrusionsCSV(csvPath, records, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Fatalf("csv missing utf-8 bom")
	}
	rows := readCSVRows(t, data)
	wantHeader := []string{"类型", "型号", "序列号", "频点", "信号", "首次发现", "最后发现", "持续时间", "坐标", "飞手距离", "无人机距离", "速度", "高度"}
	assertCSVRow(t, rows[0], wantHeader)
	wantRecord := []string{
		"定位",
		"Mavic",
		"SN1",
		"5745 MHz",
		"-61 dBm",
		"2026-05-24T01:00:00Z",
		"2026-05-24T01:03:00Z",
		"3 分 0 秒",
		"设备: 39.000000, 116.000000 / 无人机: 39.100000, 116.100000 / 返航点: 39.200000, 116.200000",
		"1.2 km",
		"800 m",
		"15.2 m/s",
		"120 m",
	}
	assertCSVRow(t, rows[1], wantRecord)
}

func TestQueryIntrusionsReadsEncryptedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "intrusions.db")
	key := "test-db-key"
	db, err := sqlitecrypto.Open(dbPath, sqlitecrypto.Config{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE intrusion_records (
		id TEXT, target_id TEXT, target_type TEXT, model TEXT, serial TEXT, device TEXT,
		frequency REAL, rssi REAL, first_seen TEXT, last_seen TEXT, duration_seconds INTEGER,
		hit_count INTEGER, source TEXT, sources_json TEXT, cracked INTEGER, device_location_json TEXT,
		drone_json TEXT, pilot_json TEXT, home_json TEXT, drone_trajectory_json TEXT, pilot_trajectory_json TEXT,
		pilot_distance_m REAL, drone_distance_m REAL, drone_direction_deg REAL, device_direction_deg REAL,
		height REAL, altitude REAL, speed REAL, last_record_json TEXT, archived_at TEXT
	);
	INSERT INTO intrusion_records VALUES (
		'id-1', 'target-1', 'position', 'Mavic', 'SN1', 'dev', 5745, -61,
		'2026-05-24T01:00:00Z', '2026-05-24T01:03:00Z', 180, 3, 'rid', '["rid"]',
		1, '', '{"latitude":39.1,"longitude":116.1}', '', '', '', '',
		NULL, NULL, NULL, NULL, NULL, NULL, NULL, '', '2026-05-24T01:04:00Z'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	encrypted, err := sqlitecrypto.IsEncrypted(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !encrypted {
		t.Fatal("database is not encrypted")
	}

	records, err := queryIntrusions(dbPath, normalizeIntrusionQuery(IntrusionQuery{TargetType: "position"}), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "id-1" {
		t.Fatalf("unexpected records: %+v", records)
	}
	if _, err := queryIntrusions(dbPath, normalizeIntrusionQuery(IntrusionQuery{}), "wrong-key"); err == nil {
		t.Fatalf("query with wrong key error = nil, want error")
	}
}

func TestQueryDeceptionReports(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "deception-reports.db")
	db, err := sqlitecrypto.Open(dbPath, sqlitecrypto.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE deception_reports (
		id TEXT, status TEXT, started_at TEXT, ended_at TEXT, duration_seconds INTEGER,
		target_id TEXT, mode TEXT, point_json TEXT, altitude_m REAL, signal_mask INTEGER,
		signal_names_json TEXT, strength_preset TEXT, attenuation_db INTEGER, delay_mode TEXT,
		delay_ns REAL, port_name TEXT, summary TEXT, last_error TEXT, abnormal_reason TEXT, record_count INTEGER,
		created_at TEXT, updated_at TEXT, request_json TEXT, session_json TEXT, records_json TEXT
	);
	INSERT INTO deception_reports VALUES (
		'report-1', 'completed', '2026-05-24T01:00:00Z', '2026-05-24T01:05:00Z', 300,
		'target-1', 'fixed_point', '{"latitude":39.1,"longitude":116.1}', 30, 3,
		'["GPS","BDS"]', 'strong', 12, 'auto', 25, '/dev/ttyUSB0', 'ok', '', '', 2,
		'2026-05-24T01:00:00Z', '2026-05-24T01:05:00Z', '{"enabled":true}', '{}', '[{"command":"start"}]'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reports, err := queryDeceptionReports(dbPath, normalizeDeceptionReportQuery(DeceptionReportQuery{Status: "completed", Mode: "fixed_point"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].SignalNames[1] != "BDS" {
		t.Fatalf("unexpected reports: %+v", reports)
	}
	if reports[0].SignalMask != 3 || reports[0].StrengthPreset != "strong" || reports[0].AttenuationDB != 12 || reports[0].DelayMode != "auto" || reports[0].DelayNS != 25 {
		t.Fatalf("extended fields not decoded: %+v", reports[0])
	}
	detail, err := getDeceptionReport(dbPath, "report-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.RecordsJSON == "" {
		t.Fatalf("expected detail records")
	}

	csvPath := filepath.Join(t.TempDir(), "deception.csv")
	if err := writeDeceptionReportsCSV(csvPath, reports, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Fatalf("csv missing utf-8 bom")
	}
	rows := readCSVRows(t, data)
	assertCSVRow(t, rows[1], []string{"已完成", "2026-05-24T01:00:00Z", "2026-05-24T01:05:00Z", "5 分 0 秒", "定点诱骗", ""})
}

func TestQueryInterferenceReportsAndCSV(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "interference-reports.db")
	db, err := sqlitecrypto.Open(dbPath, sqlitecrypto.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE interference_reports (
		id TEXT, status TEXT, started_at TEXT, ended_at TEXT, duration_seconds INTEGER,
		requested_duration_seconds INTEGER, channel_ids_json TEXT, channel_labels_json TEXT,
		channel_pins_json TEXT, summary TEXT, last_error TEXT, abnormal_reason TEXT,
		created_at TEXT, updated_at TEXT
	);
	INSERT INTO interference_reports VALUES (
		'interference-1', 'completed', '2026-05-24T01:00:00Z', '2026-05-24T01:02:00Z',
		120, 180, '["io1","io3"]', '["IOC4","IOC3"]', '[20,19]',
		'ok', '更新 GPIO 状态失败: 写入 GPIO20/value 失败: denied', '',
		'2026-05-24T01:00:00Z', '2026-05-24T01:02:00Z'
	);
	INSERT INTO interference_reports VALUES (
		'interference-2', 'failed', '2026-05-23T01:00:00Z', '2026-05-23T01:01:00Z',
		60, 60, '["io2"]', '["IOC2"]', '[18]', '', 'bad', '',
		'2026-05-23T01:00:00Z', '2026-05-23T01:01:00Z'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reports, err := queryInterferenceReports(dbPath, normalizeInterferenceReportQuery(InterferenceReportQuery{
		Status:   "completed",
		DateFrom: "2026-05-24",
		DateTo:   "2026-05-24",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	if reports[0].ChannelLabels[0] != "433M/800M/900M/1.4G" || reports[0].ChannelPins[1] != 19 {
		t.Fatalf("channel fields not decoded: %+v", reports[0])
	}
	if reports[0].LastError != "denied" {
		t.Fatalf("last error = %q, want denied", reports[0].LastError)
	}

	csvPath := filepath.Join(t.TempDir(), "interference.csv")
	if err := writeInterferenceReportsCSV(csvPath, reports, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Fatalf("csv missing utf-8 bom")
	}
	rows := readCSVRows(t, data)
	assertCSVRow(t, rows[1], []string{
		"已完成",
		"2026-05-24T01:00:00Z",
		"2026-05-24T01:02:00Z",
		"2 分 0 秒",
		"433M/800M/900M/1.4G，2.4G/5.2G/5.8G",
		"3 分 0 秒",
		"denied",
	})
}

func TestQueryReportsUseLocalDateBoundaries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "interference-reports.db")
	db, err := sqlitecrypto.Open(dbPath, sqlitecrypto.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE interference_reports (
		id TEXT, status TEXT, started_at TEXT, ended_at TEXT, duration_seconds INTEGER,
		requested_duration_seconds INTEGER, channel_ids_json TEXT, channel_labels_json TEXT,
		channel_pins_json TEXT, summary TEXT, last_error TEXT, abnormal_reason TEXT,
		created_at TEXT, updated_at TEXT
	);
	INSERT INTO interference_reports VALUES (
		'local-day', 'completed', '2026-05-23T16:30:00Z', '2026-05-23T16:31:00Z',
		60, 60, '[]', '[]', '[]', '', '', '',
		'2026-05-23T16:30:00Z', '2026-05-23T16:31:00Z'
	);
	INSERT INTO interference_reports VALUES (
		'previous-local-day', 'completed', '2026-05-23T15:30:00Z', '2026-05-23T15:31:00Z',
		60, 60, '[]', '[]', '[]', '', '', '',
		'2026-05-23T15:30:00Z', '2026-05-23T15:31:00Z'
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reports, err := queryInterferenceReports(dbPath, normalizeInterferenceReportQuery(InterferenceReportQuery{
		DateFrom: "2026-05-24",
		DateTo:   "2026-05-24",
		Locale:   "zh-CN",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1: %+v", len(reports), reports)
	}
	if reports[0].ID != "local-day" {
		t.Fatalf("got report %q, want local-day", reports[0].ID)
	}
}

func TestNormalizeInterferenceReportExternalIOFields(t *testing.T) {
	if got := defaultInterferenceBandLabel("io1", "IO2"); got != "433M/800M/900M/1.4G" {
		t.Fatalf("defaultInterferenceBandLabel(IO2) = %q", got)
	}
	if got := defaultInterferenceBandLabel("io1", "GPIO20"); got != "433M/800M/900M/1.4G" {
		t.Fatalf("defaultInterferenceBandLabel(GPIO20) = %q", got)
	}
	if got := defaultInterferenceBandLabel("io2", "GPIO18"); got != "1.2G/1.5G" {
		t.Fatalf("defaultInterferenceBandLabel(GPIO18) = %q", got)
	}
	if got := defaultInterferenceBandLabel("io3", "GPIO19"); got != "2.4G/5.2G/5.8G" {
		t.Fatalf("defaultInterferenceBandLabel(GPIO19) = %q", got)
	}

	raw := "更新 GPIO 状态失败: 外部 IO0 电平文件不可用: no such file or directory"
	if got := normalizeInterferenceReportError(raw); got != "no such file or directory" {
		t.Fatalf("normalizeInterferenceReportError() = %q", got)
	}
}

func readCSVRows(t *testing.T, data []byte) [][]string {
	t.Helper()
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertCSVRow(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("csv row length = %d, want %d\n got: %#v\nwant: %#v", len(got), len(want), got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("csv row[%d] = %q, want %q\n got: %#v\nwant: %#v", index, got[index], want[index], got, want)
		}
	}
}
