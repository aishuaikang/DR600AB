package protocol

import (
	"fmt"
	"strings"
)

var protocolTexts = map[string]map[string]string{
	"zh-CN": {
		"ack_success":          "应答成功",
		"ack_failed":           "应答失败",
		"control_set":          "设置",
		"control_ack":          "应答",
		"control_query":        "查询",
		"control_report":       "上报",
		"control_data_send":    "数据发送",
		"control_data_confirm": "数据确认",
		"unknown_control":      "未知控制字0x%02X",
		"unknown_command":      "未知命令",
		"ack_error_0":          "正确",
		"ack_error_1":          "格式错误",
		"ack_error_2":          "空间不足",
		"ack_error_3":          "数据帧超长",
		"ack_error_4":          "固件版本相同",
		"ack_error_5":          "命令 key 错误",
		"ack_error_6":          "参数无效",
		"ack_error_7":          "无效命令",
		"ack_error_8":          "设备地址不匹配",
		"ack_error_unknown":    "未知错误",
		"none":                 "无",
		"separator":            " ",
		"tx_build_failed":      "TX 构建失败: %s",
		"parse_failed":         "解析失败: %s",
		"waiting_response":     "等待响应",
		"empty_command_frame":  "命令帧为空",
		"send_failed":          "发送失败: %s",
		"partial_send":         "发送不完整: 期望 %d 字节，实际 %d 字节",
		"read_failed":          "读取串口失败: %s",
		"query_status_failed":  "查询设备状态失败: %s",
		"parse_status_failed":  "解析设备状态失败: %s",
		"status_missing_pos":   "设备状态缺少当前定位",
		"status_invalid_pos":   "设备状态当前定位经纬度无效",
		"status_pos_required":  "设备状态当前定位未查询",
		"command_failed":       "%s失败: %s",
		"command_ack_failed":   "%s应答失败: %s",
		"stop_transmit":        "关闭发射",
		"ack_result":           "%s ACK return=%d error=%d",
		"transmit_signals":     "发射信号: %s (0x%04X)",
		"time_sync":            "授时同步=0x%02X",
		"oscillator":           "晶振=%d",
		"current_position":     "当前定位=(%.6f, %.6f, %.1fm)",
		"simulated_position":   "模拟位置=(%.6f, %.6f, %.1fm)",
		"active_signals":       "工作信号=%s",
		"version":              "软件=%s FPGA=%s 协议=%s",
		"target_position":      "目标距离=%dm 高度=%dm 方位=%.1f° 航向=%.1f°",
		"spoof_circle":         "距离=%dm 高度=%dm 方位=%.1f° 航向=%.1f° 半径=%.1fm 周期=%.1fs 方向=%d",
		"suppression":          "波形=0x%08X 发射=%d",
		"signal_status":        "%s 时延=%.1fns 发射=%d 工作=0x%02X 接收卫星=%d 接收载噪比=%s 发射卫星=%d 发射PRN=%s 信号=%s",
		"random_position":      "使能=%d 半径=%dm 刷新=%ds",
		"timed_search":         "定时搜星=%d",
		"lat_lon_alt":          "纬度=%.6f 经度=%.6f 高度=%dm",
		"lon_lat_alt":          "经度=%.6f 纬度=%.6f 高度=%dm",
	},
	"en-US": {
		"ack_success":          "ack success",
		"ack_failed":           "ack failed",
		"control_set":          "set",
		"control_ack":          "ack",
		"control_query":        "query",
		"control_report":       "report",
		"control_data_send":    "data send",
		"control_data_confirm": "data confirm",
		"unknown_control":      "unknown control 0x%02X",
		"unknown_command":      "unknown command",
		"ack_error_0":          "ok",
		"ack_error_1":          "format error",
		"ack_error_2":          "insufficient space",
		"ack_error_3":          "frame too long",
		"ack_error_4":          "same firmware version",
		"ack_error_5":          "invalid command key",
		"ack_error_6":          "invalid parameter",
		"ack_error_7":          "invalid command",
		"ack_error_8":          "device address mismatch",
		"ack_error_unknown":    "unknown error",
		"none":                 "none",
		"separator":            " ",
		"tx_build_failed":      "TX build failed: %s",
		"parse_failed":         "parse failed: %s",
		"waiting_response":     "waiting for response",
		"empty_command_frame":  "empty command frame",
		"send_failed":          "send failed: %s",
		"partial_send":         "partial send: expected %d bytes, wrote %d bytes",
		"read_failed":          "read serial port failed: %s",
		"query_status_failed":  "query device status failed: %s",
		"parse_status_failed":  "parse device status failed: %s",
		"status_missing_pos":   "device status has no current position",
		"status_invalid_pos":   "device status current position is invalid",
		"status_pos_required":  "device status current position was not queried",
		"command_failed":       "%s failed: %s",
		"command_ack_failed":   "%s ack failed: %s",
		"stop_transmit":        "stop transmit",
		"ack_result":           "%s ACK return=%d error=%d",
		"transmit_signals":     "transmit signals: %s (0x%04X)",
		"time_sync":            "time sync=0x%02X",
		"oscillator":           "oscillator=%d",
		"current_position":     "current position=(%.6f, %.6f, %.1fm)",
		"simulated_position":   "simulated position=(%.6f, %.6f, %.1fm)",
		"active_signals":       "active signals=%s",
		"version":              "software=%s FPGA=%s protocol=%s",
		"target_position":      "target distance=%dm height=%dm direction=%.1f° heading=%.1f°",
		"spoof_circle":         "distance=%dm height=%dm direction=%.1f° heading=%.1f° radius=%.1fm period=%.1fs rotate=%d",
		"suppression":          "waveform=0x%08X transmit=%d",
		"signal_status":        "%s delay=%.1fns transmit=%d work=0x%02X received=%d cn0=%s transmitted=%d transmitted PRN=%s signal=%s",
		"random_position":      "enabled=%d radius=%dm refresh=%ds",
		"timed_search":         "timed search=%d",
		"lat_lon_alt":          "lat=%.6f lon=%.6f alt=%dm",
		"lon_lat_alt":          "lon=%.6f lat=%.6f alt=%dm",
	},
}

func protocolText(locale string, key string) string {
	if values, ok := protocolTexts[normalizeProtocolLocale(locale)]; ok {
		if value, ok := values[key]; ok {
			return value
		}
	}
	if value, ok := protocolTexts["zh-CN"][key]; ok {
		return value
	}
	return key
}

func TextLocale(locale string, key string, args ...any) string {
	text := protocolText(locale, key)
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

func commandNames(locale string, query bool) map[byte]string {
	if normalizeProtocolLocale(locale) != "en-US" {
		if query {
			return queryCommandNames
		}
		return setCommandNames
	}
	if query {
		return queryCommandNamesEN
	}
	return setCommandNamesEN
}

func LocalizeErrorText(text string, locale string) string {
	if normalizeProtocolLocale(locale) != "en-US" {
		return text
	}
	replacements := [][2]string{
		{"不是应答帧", "not an ack frame"},
		{"应答报文体长度不足", "ack body too short"},
		{"不是上报帧", "not a report frame"},
		{"上报报文体为空", "report body is empty"},
		{"上报命令不匹配", "report command mismatch"},
		{"发射开关上报长度不足", "transmit switch report too short"},
		{"固件版本上报长度不足", "firmware version report too short"},
		{"系统时间上报长度不足", "system time report too short"},
		{"系统时间字段无效", "invalid system time field"},
		{"位置上报长度不足", "position report too short"},
		{"目标位置上报长度不足", "target position report too short"},
		{"诱骗圆周上报长度不足", "spoof circle report too short"},
		{"功率衰减上报长度不足", "power attenuation report too short"},
		{"信号时延上报长度不足", "signal delay report too short"},
		{"压制信号发射上报长度不足", "suppression report too short"},
		{"随机坐标上报长度不足", "random position report too short"},
		{"设备信号状态上报长度不足", "device signal report too short"},
		{"定时搜星上报长度不足", "timed search report too short"},
		{"功率衰减值必须在 0~80 dB 之间", "power attenuation must be between 0 and 80 dB"},
		{"年份超出协议范围", "year is out of protocol range"},
		{"经度必须在 -180~180 之间", "longitude must be between -180 and 180"},
		{"纬度必须在 -90~90 之间", "latitude must be between -90 and 90"},
		{"获取串口列表失败", "failed to get serial port list"},
		{"串口选择已取消", "serial port selection canceled"},
		{"不支持的停止位", "unsupported stop bits"},
		{"不支持的校验位", "unsupported parity"},
		{"打开串口", "open serial port"},
	}
	for _, replacement := range replacements {
		zh, en := replacement[0], replacement[1]
		text = strings.ReplaceAll(text, zh, en)
	}
	return text
}

func normalizeProtocolLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch locale {
	case "en", "en-us":
		return "en-US"
	default:
		return "zh-CN"
	}
}

var setCommandNamesEN = map[byte]string{
	CmdSimulatedPosition: "set simulated position",
	CmdTransmitSwitch:    "set transmit switch",
	CmdPowerAttenuation:  "set power attenuation",
	CmdSystemTime:        "set system time",
	CmdDeviceReboot:      "device reboot",
	CmdInitialVelocity:   "set initial velocity",
	CmdAcceleration:      "set acceleration",
	CmdSimulatedCircle:   "set simulated circle",
	CmdTrackImport:       "track import",
	CmdMaxSpeed:          "set max speed",
	CmdDevicePosition:    "set device position",
	CmdTargetPosition:    "set target position",
	CmdCoordinateControl: "set coordinate control",
	CmdNoFlyZone:         "set no-fly zone",
	CmdSpoofCircle:       "set spoof circle",
	CmdSuppression:       "set suppression transmit",
	CmdRandomPosition:    "set random position",
	CmdSignalDelay:       "set signal delay",
	CmdTimedSearch:       "set timed search",
}

var queryCommandNamesEN = map[byte]string{
	QuerySimulatedPosition: "simulated position",
	QueryTransmitSwitch:    "transmit switch",
	QueryDeviceStatus:      "device status",
	QueryFirmwareVersion:   "firmware version",
	QuerySystemTime:        "system time",
	QueryPowerAttenuation:  "power attenuation",
	QueryTargetPosition:    "target position",
	QueryNoFlyZone:         "no-fly zone",
	QuerySpoofCircle:       "spoof circle",
	QuerySuppression:       "suppression transmit status",
	QueryDeviceSignal:      "device signal status",
	QueryDevicePosition:    "device position",
	QueryRandomPosition:    "random position settings",
	QuerySignalDelay:       "signal delay",
	QueryTimedSearch:       "timed search switch",
}
