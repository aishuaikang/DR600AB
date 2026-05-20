package protocol

import (
	"encoding/binary"
	"fmt"
	"time"
)

type PositionReport struct {
	Latitude  float64
	Longitude float64
	AltitudeM float64
}

type VersionReport struct {
	Software string
	FPGA     string
	Protocol string
}

type TargetPositionReport struct {
	DistanceM    int32
	HeightM      int32
	DirectionDeg float64
	HeadingDeg   float64
}

type SpoofCircleReport struct {
	DistanceM     int32
	HeightM       int32
	DirectionDeg  float64
	HeadingDeg    float64
	RadiusM       float64
	PeriodSeconds float64
	Direction     string
}

type SyncStatusReport struct {
	ReceiverWorking    bool
	ReceiverPositioned bool
	LeapSecondValid    bool
	TimeSynced         bool
	AntennaOK          bool
}

type MotionStatusReport struct {
	MaxSpeedMPS              *float64
	InitialSpeedMPS          *float64
	InitialDirectionDeg      *float64
	AccelerationMPS2         *float64
	AccelerationDirectionDeg *float64
	CircleRadiusM            *float64
	CirclePeriodSeconds      *float64
	CircleDirection          string
}

type DeviceStatusReport struct {
	SystemTime        *time.Time
	SyncStatus        *SyncStatusReport
	OscillatorState   string
	CurrentPosition   *PositionReport
	SimulatedPosition *PositionReport
	TemperatureC      *float64
	TimePrecisionNS   *float64
	UptimeSeconds     *uint32
	Motion            *MotionStatusReport
	AmplifierOn       *bool
	AutoTransmit      *bool
	FirstTimeSynced   *bool
	SignalMask        *uint16
}

type TransmitSwitchReport struct {
	Mask uint16
}

type PowerAttenuationReport struct {
	GPS int
	BDS int
	GLO int
	GAL int
}

type SignalDelayReport struct {
	GPS *float64
	BDS *float64
	GLO *float64
	GAL *float64
}

type SuppressionReport struct {
	WaveformMask int32
	TransmitOn   bool
}

type RandomPositionReport struct {
	Enabled        bool
	RadiusM        uint32
	RefreshSeconds uint32
}

type TimedSearchReport struct {
	Enabled bool
}

type SignalWorkStatusReport struct {
	ClockOK         bool
	EphemerisValid  bool
	RFModuleOK      bool
	SignalTransmit  bool
	TransmitChannel bool
	FPGAOK          bool
	Raw             byte
}

type DeviceSignalReport struct {
	SystemTime             *time.Time
	SignalMask             uint16
	SignalNames            []string
	DelayNS                float64
	WorkStatus             SignalWorkStatusReport
	TransmitSwitch         bool
	AttenuationDB          int
	ReceivedSatelliteCount int
	ReceivedPRNs           []int
	ReceivedCN0            []int
	TransmittedCount       int
	TransmittedPRNs        []int
}

func SignalNames(mask uint16) []string {
	names := []string{}
	if mask&SignalGPSL1CA != 0 {
		names = append(names, "GPS_L1CA")
	}
	if mask&SignalBDSB1I != 0 {
		names = append(names, "BDS_B1I")
	}
	if mask&SignalGLOL1 != 0 {
		names = append(names, "GLO_L1")
	}
	if mask&SignalBDSB2I != 0 {
		names = append(names, "BDS_B2I")
	}
	if mask&SignalBDSB3I != 0 {
		names = append(names, "BDS_B3I")
	}
	if mask&SignalBDSB1C != 0 {
		names = append(names, "BDS_B1C")
	}
	if mask&SignalGALE1 != 0 {
		names = append(names, "GAL_E1")
	}
	if mask&SignalGLOL2 != 0 {
		names = append(names, "GLO_L2")
	}
	if mask&SignalGPSL2C != 0 {
		names = append(names, "GPS_L2C")
	}
	if mask&SignalGPSL5 != 0 {
		names = append(names, "GPS_L5")
	}
	if mask&SignalGALE5A != 0 {
		names = append(names, "GAL_E5A")
	}
	if mask&SignalGALE5B != 0 {
		names = append(names, "GAL_E5B")
	}
	if mask&SignalGALE6 != 0 {
		names = append(names, "GAL_E6")
	}
	return names
}

func ParseDeviceStatusReport(frame Frame) (DeviceStatusReport, error) {
	if err := validateReportFrame(frame, QueryDeviceStatus); err != nil {
		return DeviceStatusReport{}, err
	}

	body := frame.Body
	report := DeviceStatusReport{}
	if len(body) >= 8 {
		if parsed, ok := parseProtocolTime(body, 2); ok {
			report.SystemTime = &parsed
		}
	}
	if len(body) >= 9 {
		sync := parseSyncStatus(body[8])
		report.SyncStatus = &sync
	}
	if len(body) >= 10 {
		report.OscillatorState = OscillatorStateName(body[9])
	}
	if len(body) >= 30 {
		report.CurrentPosition = &PositionReport{
			Longitude: readFloat64(body, 10),
			Latitude:  readFloat64(body, 18),
			AltitudeM: float64(readFloat32(body, 26)),
		}
	}
	if len(body) >= 34 {
		report.TemperatureC = float64Ptr(float64(readFloat32(body, 30)))
	}
	if len(body) >= 54 {
		report.SimulatedPosition = &PositionReport{
			Longitude: readFloat64(body, 34),
			Latitude:  readFloat64(body, 42),
			AltitudeM: float64(readFloat32(body, 50)),
		}
	}
	if len(body) >= 58 {
		report.TimePrecisionNS = float64Ptr(float64(readFloat32(body, 54)))
	}
	if len(body) >= 62 {
		uptime := binary.LittleEndian.Uint32(body[58:62])
		report.UptimeSeconds = &uptime
	}
	if len(body) >= 91 {
		report.Motion = &MotionStatusReport{
			MaxSpeedMPS:              float64Ptr(float64(readFloat32(body, 62))),
			InitialSpeedMPS:          float64Ptr(float64(readFloat32(body, 66))),
			InitialDirectionDeg:      float64Ptr(float64(readFloat32(body, 70))),
			AccelerationMPS2:         float64Ptr(float64(readFloat32(body, 74))),
			AccelerationDirectionDeg: float64Ptr(float64(readFloat32(body, 78))),
			CircleRadiusM:            float64Ptr(float64(readFloat32(body, 82))),
			CirclePeriodSeconds:      float64Ptr(float64(readFloat32(body, 86))),
			CircleDirection:          circleDirectionName(body[90]),
		}
	}
	if len(body) >= 92 {
		report.AmplifierOn = boolPtr(body[91] != 0)
	}
	if len(body) >= 93 {
		report.AutoTransmit = boolPtr(body[92] != 0)
	}
	if len(body) >= 94 {
		report.FirstTimeSynced = boolPtr(body[93] != 0)
	}
	if len(body) >= 96 {
		mask := binary.LittleEndian.Uint16(body[94:96])
		report.SignalMask = &mask
	}
	return report, nil
}

func ParseTransmitSwitchReport(frame Frame) (TransmitSwitchReport, error) {
	if err := validateReportFrame(frame, QueryTransmitSwitch); err != nil {
		return TransmitSwitchReport{}, err
	}
	if len(frame.Body) < 4 {
		return TransmitSwitchReport{}, fmt.Errorf("发射开关上报长度不足: %d", len(frame.Body))
	}
	return TransmitSwitchReport{Mask: binary.LittleEndian.Uint16(frame.Body[2:4])}, nil
}

func ParseVersionReport(frame Frame) (VersionReport, error) {
	if err := validateReportFrame(frame, QueryFirmwareVersion); err != nil {
		return VersionReport{}, err
	}
	if len(frame.Body) < 14 {
		return VersionReport{}, fmt.Errorf("固件版本上报长度不足: %d", len(frame.Body))
	}
	return VersionReport{
		Software: formatVersion(binary.LittleEndian.Uint32(frame.Body[2:6])),
		FPGA:     formatVersion(binary.LittleEndian.Uint32(frame.Body[6:10])),
		Protocol: formatVersion(binary.LittleEndian.Uint32(frame.Body[10:14])),
	}, nil
}

func ParseSystemTimeReport(frame Frame) (*time.Time, error) {
	if err := validateReportFrame(frame, QuerySystemTime); err != nil {
		return nil, err
	}
	if len(frame.Body) < 7 {
		return nil, fmt.Errorf("系统时间上报长度不足: %d", len(frame.Body))
	}
	parsed, ok := parseProtocolTime(frame.Body, 1)
	if !ok {
		return nil, fmt.Errorf("系统时间字段无效")
	}
	return &parsed, nil
}

func ParsePositionReport(frame Frame, command byte) (PositionReport, error) {
	if err := validateReportFrame(frame, command); err != nil {
		return PositionReport{}, err
	}
	if len(frame.Body) < 22 {
		return PositionReport{}, fmt.Errorf("位置上报长度不足: %d", len(frame.Body))
	}
	return PositionReport{
		Latitude:  readFloat64(frame.Body, 2),
		Longitude: readFloat64(frame.Body, 10),
		AltitudeM: float64(int32(binary.LittleEndian.Uint32(frame.Body[18:22]))),
	}, nil
}

func ParseTargetPositionReport(frame Frame) (TargetPositionReport, error) {
	if err := validateReportFrame(frame, QueryTargetPosition); err != nil {
		return TargetPositionReport{}, err
	}
	if len(frame.Body) < 18 {
		return TargetPositionReport{}, fmt.Errorf("目标位置上报长度不足: %d", len(frame.Body))
	}
	return TargetPositionReport{
		DistanceM:    int32(binary.LittleEndian.Uint32(frame.Body[2:6])),
		HeightM:      int32(binary.LittleEndian.Uint32(frame.Body[6:10])),
		DirectionDeg: float64(readFloat32(frame.Body, 10)),
		HeadingDeg:   float64(readFloat32(frame.Body, 14)),
	}, nil
}

func ParseSpoofCircleReport(frame Frame) (SpoofCircleReport, error) {
	if err := validateReportFrame(frame, QuerySpoofCircle); err != nil {
		return SpoofCircleReport{}, err
	}
	if len(frame.Body) < 30 {
		return SpoofCircleReport{}, fmt.Errorf("诱骗圆周上报长度不足: %d", len(frame.Body))
	}
	return SpoofCircleReport{
		DistanceM:     int32(binary.LittleEndian.Uint32(frame.Body[2:6])),
		HeightM:       int32(binary.LittleEndian.Uint32(frame.Body[6:10])),
		DirectionDeg:  float64(readFloat32(frame.Body, 10)),
		HeadingDeg:    float64(readFloat32(frame.Body, 14)),
		RadiusM:       float64(readFloat32(frame.Body, 18)),
		PeriodSeconds: float64(readFloat32(frame.Body, 22)),
		Direction:     circleDirectionName(byte(binary.LittleEndian.Uint32(frame.Body[26:30]))),
	}, nil
}

func ParsePowerAttenuationReport(frame Frame) (PowerAttenuationReport, error) {
	if err := validateReportFrame(frame, QueryPowerAttenuation); err != nil {
		return PowerAttenuationReport{}, err
	}
	if len(frame.Body) < 17 {
		return PowerAttenuationReport{}, fmt.Errorf("功率衰减上报长度不足: %d", len(frame.Body))
	}
	return PowerAttenuationReport{
		GPS: int(frame.Body[1]),
		BDS: int(frame.Body[2]),
		GLO: int(frame.Body[3]),
		GAL: int(frame.Body[7]),
	}, nil
}

func ParseSignalDelayReport(frame Frame) (SignalDelayReport, error) {
	if err := validateReportFrame(frame, QuerySignalDelay); err != nil {
		return SignalDelayReport{}, err
	}
	if len(frame.Body) < 2 {
		return SignalDelayReport{}, fmt.Errorf("信号时延上报长度不足: %d", len(frame.Body))
	}

	report := SignalDelayReport{}
	if len(frame.Body) >= 6 {
		report.GPS = float64Ptr(float64(readFloat32(frame.Body, 2)))
	}
	if len(frame.Body) >= 10 {
		report.BDS = float64Ptr(float64(readFloat32(frame.Body, 6)))
	}
	if len(frame.Body) >= 14 {
		report.GLO = float64Ptr(float64(readFloat32(frame.Body, 10)))
	}
	if len(frame.Body) >= 30 {
		report.GAL = float64Ptr(float64(readFloat32(frame.Body, 26)))
	}
	return report, nil
}

func ParseSuppressionReport(frame Frame) (SuppressionReport, error) {
	if err := validateReportFrame(frame, QuerySuppression); err != nil {
		return SuppressionReport{}, err
	}
	if len(frame.Body) < 10 {
		return SuppressionReport{}, fmt.Errorf("压制信号发射上报长度不足: %d", len(frame.Body))
	}
	return SuppressionReport{
		WaveformMask: int32(binary.LittleEndian.Uint32(frame.Body[2:6])),
		TransmitOn:   int32(binary.LittleEndian.Uint32(frame.Body[6:10])) != 0,
	}, nil
}

func ParseRandomPositionReport(frame Frame) (RandomPositionReport, error) {
	if err := validateReportFrame(frame, QueryRandomPosition); err != nil {
		return RandomPositionReport{}, err
	}
	if len(frame.Body) < 14 {
		return RandomPositionReport{}, fmt.Errorf("随机坐标上报长度不足: %d", len(frame.Body))
	}
	return RandomPositionReport{
		Enabled:        binary.LittleEndian.Uint32(frame.Body[2:6]) != 0,
		RadiusM:        binary.LittleEndian.Uint32(frame.Body[6:10]),
		RefreshSeconds: binary.LittleEndian.Uint32(frame.Body[10:14]),
	}, nil
}

func ParseDeviceSignalReport(frame Frame) (DeviceSignalReport, error) {
	if err := validateReportFrame(frame, QueryDeviceSignal); err != nil {
		return DeviceSignalReport{}, err
	}
	if len(frame.Body) < 90 {
		return DeviceSignalReport{}, fmt.Errorf("设备信号状态上报长度不足: %d", len(frame.Body))
	}

	body := frame.Body
	report := DeviceSignalReport{}
	if parsed, ok := parseProtocolTime(body, 2); ok {
		report.SystemTime = &parsed
	}
	report.SignalMask = binary.LittleEndian.Uint16(body[8:10])
	report.SignalNames = SignalNames(report.SignalMask)
	report.DelayNS = float64(readFloat32(body, 10))
	report.WorkStatus = parseSignalWorkStatus(body[14])
	report.TransmitSwitch = body[15] != 0
	report.AttenuationDB = int(body[16])
	report.ReceivedSatelliteCount = int(body[17])
	report.ReceivedPRNs = compactPRNs(body[18:42])
	report.ReceivedCN0 = compactUint8(body[42:66])
	report.TransmittedCount = int(body[66])
	report.TransmittedPRNs = compactPRNs(body[67:91])
	return report, nil
}

func ParseTimedSearchReport(frame Frame) (TimedSearchReport, error) {
	if err := validateReportFrame(frame, QueryTimedSearch); err != nil {
		return TimedSearchReport{}, err
	}
	if len(frame.Body) < 2 {
		return TimedSearchReport{}, fmt.Errorf("定时搜星上报长度不足: %d", len(frame.Body))
	}
	return TimedSearchReport{Enabled: frame.Body[1] != 0}, nil
}

func OscillatorStateName(value byte) string {
	switch value {
	case 0:
		return "warming"
	case 1:
		return "unlocked"
	case 2:
		return "tracking"
	case 3:
		return "locked"
	case 4:
		return "hold"
	default:
		return "unknown"
	}
}

func validateReportFrame(frame Frame, command byte) error {
	if frame.Control != ControlReport {
		return fmt.Errorf("不是上报帧: 0x%02X", byte(frame.Control))
	}
	if len(frame.Body) == 0 {
		return fmt.Errorf("上报报文体为空")
	}
	if frame.Command() != command {
		return fmt.Errorf("上报命令不匹配: got 0x%02X want 0x%02X", frame.Command(), command)
	}
	return nil
}

func parseSyncStatus(value byte) SyncStatusReport {
	return SyncStatusReport{
		ReceiverWorking:    value&0x01 != 0,
		ReceiverPositioned: value&0x02 != 0,
		LeapSecondValid:    value&0x04 != 0,
		TimeSynced:         value&0x08 != 0,
		AntennaOK:          value&0x80 != 0,
	}
}

func parseSignalWorkStatus(value byte) SignalWorkStatusReport {
	return SignalWorkStatusReport{
		ClockOK:         value&0x01 != 0,
		EphemerisValid:  value&0x04 != 0,
		RFModuleOK:      value&0x08 != 0,
		SignalTransmit:  value&0x10 != 0,
		TransmitChannel: value&0x20 != 0,
		FPGAOK:          value&0x40 != 0,
		Raw:             value,
	}
}

func compactPRNs(values []byte) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		out = append(out, int(value))
	}
	return out
}

func compactUint8(values []byte) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		out = append(out, int(value))
	}
	return out
}

func parseProtocolTime(body []byte, offset int) (time.Time, bool) {
	if len(body) < offset+6 {
		return time.Time{}, false
	}
	year := 2000 + int(body[offset])
	month := time.Month(body[offset+1])
	day := int(body[offset+2])
	hour := int(body[offset+3])
	minute := int(body[offset+4])
	second := int(body[offset+5])
	if month < time.January || month > time.December ||
		day < 1 || day > 31 ||
		hour > 23 || minute > 59 || second > 59 {
		return time.Time{}, false
	}
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC), true
}

func circleDirectionName(value byte) string {
	switch value {
	case 0:
		return "cw"
	case 1:
		return "ccw"
	default:
		return "unknown"
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
