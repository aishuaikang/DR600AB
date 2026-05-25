package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type Ack struct {
	Command     byte
	ReturnValue byte
	ErrorCode   uint16
}

func (a Ack) Success() bool {
	return a.ReturnValue == 0 && a.ErrorCode == 0
}

func ParseAck(frame Frame) (Ack, error) {
	if frame.Control != ControlAck {
		return Ack{}, fmt.Errorf("不是应答帧: 0x%02X", byte(frame.Control))
	}
	if len(frame.Body) < 4 {
		return Ack{}, fmt.Errorf("应答报文体长度不足: %d", len(frame.Body))
	}
	return Ack{
		Command:     frame.Body[0],
		ReturnValue: frame.Body[1],
		ErrorCode:   binary.LittleEndian.Uint16(frame.Body[2:4]),
	}, nil
}

func DescribeFrame(frame Frame) string {
	return DescribeFrameLocale(frame, "zh-CN")
}

func DescribeFrameLocale(frame Frame, locale string) string {
	cmd := frame.Command()
	prefix := fmt.Sprintf("%s cmd=0x%02X(%s) src=0x%02X dst=0x%02X",
		ControlNameLocale(frame.Control, locale),
		cmd,
		CommandNameLocale(frame.Control, cmd, locale),
		frame.Source,
		frame.Target,
	)

	switch frame.Control {
	case ControlAck:
		ack, err := ParseAck(frame)
		if err != nil {
			return prefix + " " + LocalizeErrorText(err.Error(), locale)
		}
		if ack.Success() {
			return fmt.Sprintf("%s %s", prefix, protocolText(locale, "ack_success"))
		}
		return fmt.Sprintf("%s %s return=%d error=%d(%s)", prefix, protocolText(locale, "ack_failed"), ack.ReturnValue, ack.ErrorCode, AckErrorTextLocale(ack.ErrorCode, locale))
	case ControlReport:
		if detail := describeReportLocale(cmd, frame.Body, locale); detail != "" {
			return prefix + " " + detail
		}
	}
	return fmt.Sprintf("%s body=%s", prefix, Hex(frame.Body))
}

func ControlName(control ControlWord) string {
	return ControlNameLocale(control, "zh-CN")
}

func ControlNameLocale(control ControlWord, locale string) string {
	switch control {
	case ControlSet:
		return protocolText(locale, "control_set")
	case ControlAck:
		return protocolText(locale, "control_ack")
	case ControlQuery:
		return protocolText(locale, "control_query")
	case ControlReport:
		return protocolText(locale, "control_report")
	case ControlDataSend:
		return protocolText(locale, "control_data_send")
	case ControlDataConfirm:
		return protocolText(locale, "control_data_confirm")
	default:
		return fmt.Sprintf(protocolText(locale, "unknown_control"), byte(control))
	}
}

func CommandName(control ControlWord, cmd byte) string {
	return CommandNameLocale(control, cmd, "zh-CN")
}

func CommandNameLocale(control ControlWord, cmd byte, locale string) string {
	if control == ControlQuery || control == ControlReport {
		if name, ok := commandNames(locale, true)[cmd]; ok {
			return name
		}
	}
	if name, ok := commandNames(locale, false)[cmd]; ok {
		return name
	}
	return protocolText(locale, "unknown_command")
}

func AckErrorText(code uint16) string {
	return AckErrorTextLocale(code, "zh-CN")
}

func AckErrorTextLocale(code uint16, locale string) string {
	switch code {
	case 0:
		return protocolText(locale, "ack_error_0")
	case 1:
		return protocolText(locale, "ack_error_1")
	case 2:
		return protocolText(locale, "ack_error_2")
	case 3:
		return protocolText(locale, "ack_error_3")
	case 4:
		return protocolText(locale, "ack_error_4")
	case 5:
		return protocolText(locale, "ack_error_5")
	case 6:
		return protocolText(locale, "ack_error_6")
	case 7:
		return protocolText(locale, "ack_error_7")
	case 8:
		return protocolText(locale, "ack_error_8")
	default:
		return protocolText(locale, "ack_error_unknown")
	}
}

func Hex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, " ")
}

func FormatSignals(mask uint16) string {
	return FormatSignalsLocale(mask, "zh-CN")
}

func FormatSignalsLocale(mask uint16, locale string) string {
	names := SignalNames(mask)
	if len(names) == 0 {
		return protocolText(locale, "none")
	}
	return strings.Join(names, "/")
}

var setCommandNames = map[byte]string{
	CmdSimulatedPosition: "设置模拟位置",
	CmdTransmitSwitch:    "设置发射开关",
	CmdPowerAttenuation:  "设置功率衰减",
	CmdSystemTime:        "设置系统时间",
	CmdDeviceReboot:      "设备重启",
	CmdInitialVelocity:   "设置模拟初速度",
	CmdAcceleration:      "设置模拟加速度",
	CmdSimulatedCircle:   "设置模拟圆周运动",
	CmdTrackImport:       "轨迹导入",
	CmdMaxSpeed:          "设置最大速度",
	CmdDevicePosition:    "设置设备位置",
	CmdTargetPosition:    "设置目标位置",
	CmdCoordinateControl: "设置诱骗坐标控制",
	CmdNoFlyZone:         "设置禁飞区",
	CmdSpoofCircle:       "设置诱骗圆周运动",
	CmdSuppression:       "设置压制信号发射",
	CmdRandomPosition:    "设置生成随机坐标",
	CmdSignalDelay:       "设置信号时延",
	CmdTimedSearch:       "设置定时搜星使能",
}

var queryCommandNames = map[byte]string{
	QuerySimulatedPosition: "模拟位置",
	QueryTransmitSwitch:    "发射开关",
	QueryDeviceStatus:      "设备状态",
	QueryFirmwareVersion:   "固件版本",
	QuerySystemTime:        "系统时间",
	QueryPowerAttenuation:  "功率衰减",
	QueryTargetPosition:    "目标位置",
	QueryNoFlyZone:         "禁飞区",
	QuerySpoofCircle:       "诱骗圆周运动",
	QuerySuppression:       "压制信号发射状态",
	QueryDeviceSignal:      "设备信号状态",
	QueryDevicePosition:    "设备位置",
	QueryRandomPosition:    "随机坐标设置",
	QuerySignalDelay:       "信号时延",
	QueryTimedSearch:       "定时搜星开关状态",
}

func describeReport(cmd byte, body []byte) string {
	return describeReportLocale(cmd, body, "zh-CN")
}

func describeReportLocale(cmd byte, body []byte, locale string) string {
	switch cmd {
	case QuerySimulatedPosition:
		return describeLatLonAlt(body, 2, true, locale)
	case QueryTransmitSwitch:
		if len(body) >= 4 {
			mask := binary.LittleEndian.Uint16(body[2:4])
			return fmt.Sprintf(protocolText(locale, "transmit_signals"), FormatSignalsLocale(mask, locale), mask)
		}
	case QueryDeviceStatus:
		return describeDeviceStatus(body, locale)
	case QueryFirmwareVersion:
		return describeVersion(body, locale)
	case QuerySystemTime:
		return describeTime(body, 1)
	case QueryPowerAttenuation:
		return describePowerAttenuation(body)
	case QueryTargetPosition:
		return describeTargetPosition(body, locale)
	case QuerySpoofCircle:
		return describeSpoofCircle(body, locale)
	case QuerySuppression:
		return describeSuppression(body, locale)
	case QueryDeviceSignal:
		return describeSignalStatus(body, locale)
	case QueryDevicePosition:
		return describeLatLonAlt(body, 2, true, locale)
	case QueryRandomPosition:
		return describeRandomPosition(body, locale)
	case QuerySignalDelay:
		return describeSignalDelay(body)
	case QueryTimedSearch:
		return describeTimedSearch(body, locale)
	}
	return ""
}

func describeDeviceStatus(body []byte, locale string) string {
	parts := []string{}
	if len(body) >= 10 {
		parts = append(parts, describeTime(body, 2))
		parts = append(parts, fmt.Sprintf(protocolText(locale, "time_sync"), body[8]))
		parts = append(parts, fmt.Sprintf(protocolText(locale, "oscillator"), body[9]))
	}
	if len(body) >= 30 {
		lon := readFloat64(body, 10)
		lat := readFloat64(body, 18)
		height := readFloat32(body, 26)
		parts = append(parts, fmt.Sprintf(protocolText(locale, "current_position"), lon, lat, height))
	}
	if len(body) >= 54 {
		lon := readFloat64(body, 34)
		lat := readFloat64(body, 42)
		height := readFloat32(body, 50)
		parts = append(parts, fmt.Sprintf(protocolText(locale, "simulated_position"), lon, lat, height))
	}
	if len(body) >= 96 {
		mask := binary.LittleEndian.Uint16(body[94:96])
		parts = append(parts, fmt.Sprintf(protocolText(locale, "active_signals"), FormatSignalsLocale(mask, locale)))
	}
	return strings.Join(parts, protocolText(locale, "separator"))
}

func describeVersion(body []byte, locale string) string {
	if len(body) < 14 {
		return ""
	}
	software := binary.LittleEndian.Uint32(body[2:6])
	fpga := binary.LittleEndian.Uint32(body[6:10])
	protocol := binary.LittleEndian.Uint32(body[10:14])
	return fmt.Sprintf(protocolText(locale, "version"), formatVersion(software), formatVersion(fpga), formatVersion(protocol))
}

func describePowerAttenuation(body []byte) string {
	if len(body) < 17 {
		return ""
	}
	return fmt.Sprintf("GPS=%ddB BDS=%ddB GLO=%ddB GAL=%ddB", body[1], body[2], body[3], body[7])
}

func describeTargetPosition(body []byte, locale string) string {
	if len(body) < 18 {
		return ""
	}
	return fmt.Sprintf(protocolText(locale, "target_position"),
		int32(binary.LittleEndian.Uint32(body[2:6])),
		int32(binary.LittleEndian.Uint32(body[6:10])),
		readFloat32(body, 10),
		readFloat32(body, 14),
	)
}

func describeSpoofCircle(body []byte, locale string) string {
	if len(body) < 30 {
		return ""
	}
	return fmt.Sprintf(protocolText(locale, "spoof_circle"),
		int32(binary.LittleEndian.Uint32(body[2:6])),
		int32(binary.LittleEndian.Uint32(body[6:10])),
		readFloat32(body, 10),
		readFloat32(body, 14),
		readFloat32(body, 18),
		readFloat32(body, 22),
		int32(binary.LittleEndian.Uint32(body[26:30])),
	)
}

func describeSuppression(body []byte, locale string) string {
	if len(body) < 10 {
		return ""
	}
	return fmt.Sprintf(protocolText(locale, "suppression"),
		binary.LittleEndian.Uint32(body[2:6]),
		binary.LittleEndian.Uint32(body[6:10]),
	)
}

func describeSignalStatus(body []byte, locale string) string {
	if len(body) < 90 {
		return ""
	}
	mask := binary.LittleEndian.Uint16(body[8:10])
	return fmt.Sprintf(protocolText(locale, "signal_status"),
		describeTime(body, 2),
		readFloat32(body, 10),
		body[14],
		body[15],
		body[16],
		body[17],
		compactPRNs(body[18:42]),
		body[66],
		compactPRNs(body[67:91]),
		FormatSignalsLocale(mask, locale),
	)
}

func describeRandomPosition(body []byte, locale string) string {
	if len(body) < 14 {
		return ""
	}
	return fmt.Sprintf(protocolText(locale, "random_position"),
		binary.LittleEndian.Uint32(body[2:6]),
		binary.LittleEndian.Uint32(body[6:10]),
		binary.LittleEndian.Uint32(body[10:14]),
	)
}

func describeSignalDelay(body []byte) string {
	if len(body) < 30 {
		return ""
	}
	return fmt.Sprintf("GPS=%.2fns BDS=%.2fns GLO=%.2fns GAL=%.2fns",
		readFloat32(body, 2),
		readFloat32(body, 6),
		readFloat32(body, 10),
		readFloat32(body, 26),
	)
}

func describeTimedSearch(body []byte, locale string) string {
	if len(body) < 2 {
		return ""
	}
	return fmt.Sprintf(protocolText(locale, "timed_search"), body[1])
}

func describeLatLonAlt(body []byte, offset int, latFirst bool, locale string) string {
	if len(body) < offset+20 {
		return ""
	}
	first := readFloat64(body, offset)
	second := readFloat64(body, offset+8)
	alt := int32(binary.LittleEndian.Uint32(body[offset+16 : offset+20]))
	if latFirst {
		return fmt.Sprintf(protocolText(locale, "lat_lon_alt"), first, second, alt)
	}
	return fmt.Sprintf(protocolText(locale, "lon_lat_alt"), first, second, alt)
}

func describeTime(body []byte, offset int) string {
	if len(body) < offset+6 {
		return ""
	}
	return fmt.Sprintf("UTC=%04d-%02d-%02d %02d:%02d:%02d",
		2000+int(body[offset]),
		body[offset+1],
		body[offset+2],
		body[offset+3],
		body[offset+4],
		body[offset+5],
	)
}

func formatVersion(value uint32) string {
	major := (value >> 20) & 0x0FFF
	minor := (value >> 10) & 0x03FF
	build := value & 0x03FF
	return fmt.Sprintf("%d.%d.%d", major, minor, build)
}

func readFloat32(data []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func readFloat64(data []byte, offset int) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(data[offset : offset+8]))
}
