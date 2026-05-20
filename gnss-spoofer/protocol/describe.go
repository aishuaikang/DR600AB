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
	cmd := frame.Command()
	prefix := fmt.Sprintf("%s cmd=0x%02X(%s) src=0x%02X dst=0x%02X",
		ControlName(frame.Control),
		cmd,
		CommandName(frame.Control, cmd),
		frame.Source,
		frame.Target,
	)

	switch frame.Control {
	case ControlAck:
		ack, err := ParseAck(frame)
		if err != nil {
			return prefix + " " + err.Error()
		}
		if ack.Success() {
			return fmt.Sprintf("%s 应答成功", prefix)
		}
		return fmt.Sprintf("%s 应答失败 return=%d error=%d(%s)", prefix, ack.ReturnValue, ack.ErrorCode, AckErrorText(ack.ErrorCode))
	case ControlReport:
		if detail := describeReport(cmd, frame.Body); detail != "" {
			return prefix + " " + detail
		}
	}
	return fmt.Sprintf("%s body=%s", prefix, Hex(frame.Body))
}

func ControlName(control ControlWord) string {
	switch control {
	case ControlSet:
		return "设置"
	case ControlAck:
		return "应答"
	case ControlQuery:
		return "查询"
	case ControlReport:
		return "上报"
	case ControlDataSend:
		return "数据发送"
	case ControlDataConfirm:
		return "数据确认"
	default:
		return fmt.Sprintf("未知控制字0x%02X", byte(control))
	}
}

func CommandName(control ControlWord, cmd byte) string {
	if control == ControlQuery || control == ControlReport {
		if name, ok := queryCommandNames[cmd]; ok {
			return name
		}
	}
	if name, ok := setCommandNames[cmd]; ok {
		return name
	}
	return "未知命令"
}

func AckErrorText(code uint16) string {
	switch code {
	case 0:
		return "正确"
	case 1:
		return "格式错误"
	case 2:
		return "空间不足"
	case 3:
		return "数据帧超长"
	case 4:
		return "固件版本相同"
	case 5:
		return "命令 key 错误"
	case 6:
		return "参数无效"
	case 7:
		return "无效命令"
	case 8:
		return "设备地址不匹配"
	default:
		return "未知错误"
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
	names := SignalNames(mask)
	if len(names) == 0 {
		return "无"
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
	switch cmd {
	case QuerySimulatedPosition:
		return describeLatLonAlt(body, 2, true)
	case QueryTransmitSwitch:
		if len(body) >= 4 {
			mask := binary.LittleEndian.Uint16(body[2:4])
			return fmt.Sprintf("发射信号: %s (0x%04X)", FormatSignals(mask), mask)
		}
	case QueryDeviceStatus:
		return describeDeviceStatus(body)
	case QueryFirmwareVersion:
		return describeVersion(body)
	case QuerySystemTime:
		return describeTime(body, 1)
	case QueryPowerAttenuation:
		return describePowerAttenuation(body)
	case QueryTargetPosition:
		return describeTargetPosition(body)
	case QuerySpoofCircle:
		return describeSpoofCircle(body)
	case QuerySuppression:
		return describeSuppression(body)
	case QueryDeviceSignal:
		return describeSignalStatus(body)
	case QueryDevicePosition:
		return describeLatLonAlt(body, 2, true)
	case QueryRandomPosition:
		return describeRandomPosition(body)
	case QuerySignalDelay:
		return describeSignalDelay(body)
	case QueryTimedSearch:
		return describeTimedSearch(body)
	}
	return ""
}

func describeDeviceStatus(body []byte) string {
	parts := []string{}
	if len(body) >= 10 {
		parts = append(parts, describeTime(body, 2))
		parts = append(parts, fmt.Sprintf("授时同步=0x%02X", body[8]))
		parts = append(parts, fmt.Sprintf("晶振=%d", body[9]))
	}
	if len(body) >= 30 {
		lon := readFloat64(body, 10)
		lat := readFloat64(body, 18)
		height := readFloat32(body, 26)
		parts = append(parts, fmt.Sprintf("当前定位=(%.6f, %.6f, %.1fm)", lon, lat, height))
	}
	if len(body) >= 54 {
		lon := readFloat64(body, 34)
		lat := readFloat64(body, 42)
		height := readFloat32(body, 50)
		parts = append(parts, fmt.Sprintf("模拟定位=(%.6f, %.6f, %.1fm)", lon, lat, height))
	}
	if len(body) >= 96 {
		mask := binary.LittleEndian.Uint16(body[94:96])
		parts = append(parts, fmt.Sprintf("信号导通=%s", FormatSignals(mask)))
	}
	return strings.Join(parts, "，")
}

func describeVersion(body []byte) string {
	if len(body) < 14 {
		return ""
	}
	software := binary.LittleEndian.Uint32(body[2:6])
	fpga := binary.LittleEndian.Uint32(body[6:10])
	protocol := binary.LittleEndian.Uint32(body[10:14])
	return fmt.Sprintf("软件=%s FPGA=%s 协议=%s", formatVersion(software), formatVersion(fpga), formatVersion(protocol))
}

func describePowerAttenuation(body []byte) string {
	if len(body) < 17 {
		return ""
	}
	return fmt.Sprintf("GPS=%ddB BDS=%ddB GLO=%ddB GAL=%ddB", body[1], body[2], body[3], body[7])
}

func describeTargetPosition(body []byte) string {
	if len(body) < 18 {
		return ""
	}
	return fmt.Sprintf("距离=%dm 高度=%dm 方向角=%.1f° 航向角=%.1f°",
		int32(binary.LittleEndian.Uint32(body[2:6])),
		int32(binary.LittleEndian.Uint32(body[6:10])),
		readFloat32(body, 10),
		readFloat32(body, 14),
	)
}

func describeSpoofCircle(body []byte) string {
	if len(body) < 30 {
		return ""
	}
	return fmt.Sprintf("距离=%dm 高度=%dm 方向角=%.1f° 航向角=%.1f° 半径=%.1fm 周期=%.1fs 方向=%d",
		int32(binary.LittleEndian.Uint32(body[2:6])),
		int32(binary.LittleEndian.Uint32(body[6:10])),
		readFloat32(body, 10),
		readFloat32(body, 14),
		readFloat32(body, 18),
		readFloat32(body, 22),
		int32(binary.LittleEndian.Uint32(body[26:30])),
	)
}

func describeSuppression(body []byte) string {
	if len(body) < 10 {
		return ""
	}
	return fmt.Sprintf("波形使能=0x%08X 发射=%d",
		binary.LittleEndian.Uint32(body[2:6]),
		binary.LittleEndian.Uint32(body[6:10]),
	)
}

func describeSignalStatus(body []byte) string {
	if len(body) < 90 {
		return ""
	}
	mask := binary.LittleEndian.Uint16(body[8:10])
	return fmt.Sprintf("%s，时延=%.2fns，工作状态=0x%02X，发射=%d，衰减=%ddB，接收星数=%d，接收PRN=%v，发射星数=%d，发射PRN=%v",
		describeTime(body, 2),
		readFloat32(body, 10),
		body[14],
		body[15],
		body[16],
		body[17],
		compactPRNs(body[18:42]),
		body[66],
		compactPRNs(body[67:91]),
	) + fmt.Sprintf("，信号=%s", FormatSignals(mask))
}

func describeRandomPosition(body []byte) string {
	if len(body) < 14 {
		return ""
	}
	return fmt.Sprintf("使能=%d 半径=%dm 刷新周期=%ds",
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

func describeTimedSearch(body []byte) string {
	if len(body) < 2 {
		return ""
	}
	return fmt.Sprintf("定时搜星=%d", body[1])
}

func describeLatLonAlt(body []byte, offset int, latFirst bool) string {
	if len(body) < offset+20 {
		return ""
	}
	first := readFloat64(body, offset)
	second := readFloat64(body, offset+8)
	alt := int32(binary.LittleEndian.Uint32(body[offset+16 : offset+20]))
	if latFirst {
		return fmt.Sprintf("纬度=%.6f 经度=%.6f 高度=%dm", first, second, alt)
	}
	return fmt.Sprintf("经度=%.6f 纬度=%.6f 高度=%dm", first, second, alt)
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
