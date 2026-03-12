package protocol

import (
	"encoding/binary"
	"fmt"
)

const (
	FrameHeader byte = 0xAA
	FrameTail   byte = 0x55

	// 命令码
	CmdSetSwitchPower  byte = 0x04 // 设置N路开关功率
	CmdQueryModuleInfo byte = 0x05 // 查询N路模块信息

	MaxModules = 10 // 最大模块数
)

// Frame 协议帧结构
type Frame struct {
	ID      uint16 // 设备ID (0-5000)
	Length  byte   // 整帧数据长度
	Command byte   // 命令码
	Data    []byte // 数据域
}

// ModelSetting 模块设置 (用于0x04命令)
type ModelSetting struct {
	SwitchSetting byte // 开关设置: 1=开, 0=关
	PowerSetting  byte // 功率设置
}

// ModelInfo 模块信息 (用于0x05命令返回, 新协议)
type ModelInfo struct {
	FreqStart     uint16 // 起始频率
	FreqEnd       uint16 // 结束频率
	PowerDisp     byte   // 功率显示
	TempDisp      byte   // 温度显示
	SwitchSetting byte   // 当前开关设置: 1=开, 0=关
	PowerSetting  byte   // 当前功率设置
	Alarm         byte   // 告警: 0x01=驻波, 0x02=温度, 0x04=功率, 0x08=门禁
	Reserved      uint16 // 保留两个字节
}

// Encode 将帧编码为字节流
func (f *Frame) Encode() []byte {
	// 帧结构: header(1) + id(2) + length(1) + cmd(1) + data(N) + tail(1)
	totalLen := 1 + 2 + 1 + 1 + len(f.Data) + 1
	f.Length = byte(totalLen)

	buf := make([]byte, totalLen)
	buf[0] = FrameHeader
	// ID: 小端序
	binary.LittleEndian.PutUint16(buf[1:3], f.ID)
	buf[3] = f.Length
	buf[4] = f.Command
	copy(buf[5:5+len(f.Data)], f.Data)
	buf[totalLen-1] = FrameTail

	return buf
}

// DecodeFrame 从字节流中解码一帧数据
func DecodeFrame(data []byte) (*Frame, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("数据长度不足，最少需要6字节，当前%d字节", len(data))
	}
	if data[0] != FrameHeader {
		return nil, fmt.Errorf("帧头错误: 期望0x%02X, 实际0x%02X", FrameHeader, data[0])
	}

	frame := &Frame{}
	frame.ID = binary.LittleEndian.Uint16(data[1:3])
	frame.Length = data[3]

	if int(frame.Length) > len(data) {
		return nil, fmt.Errorf("帧长度标识(%d)超过实际数据长度(%d)", frame.Length, len(data))
	}

	if data[frame.Length-1] != FrameTail {
		return nil, fmt.Errorf("帧尾错误: 期望0x%02X, 实际0x%02X", FrameTail, data[frame.Length-1])
	}

	frame.Command = data[4]
	dataLen := int(frame.Length) - 6 // 减去 header + id(2) + length + cmd + tail
	if dataLen > 0 {
		frame.Data = make([]byte, dataLen)
		copy(frame.Data, data[5:5+dataLen])
	}

	return frame, nil
}

// BuildSetSwitchPower 构建设置N路开关功率命令 (0x04)
// 服务器发送: 0xaa + ID + 长度 + 0x04 + data(Nx模块设置) + 0x55
func BuildSetSwitchPower(id uint16, settings []ModelSetting) []byte {
	data := make([]byte, len(settings)*2)
	for i, s := range settings {
		data[i*2] = s.SwitchSetting
		data[i*2+1] = s.PowerSetting
	}

	frame := &Frame{
		ID:      id,
		Command: CmdSetSwitchPower,
		Data:    data,
	}
	return frame.Encode()
}

// BuildQueryModuleInfo 构建查询N路模块信息命令 (0x05)
// 服务器发送: 0xaa + ID + 0x06 + 0x05 + 0x55
func BuildQueryModuleInfo(id uint16) []byte {
	frame := &Frame{
		ID:      id,
		Command: CmdQueryModuleInfo,
		Data:    nil,
	}
	return frame.Encode()
}

// ParseSetSwitchPowerResponse 解析设置开关功率的响应
// 终端返回: 0xaa + ID + 0x06 + 0x04 + 结果(1byte) + 0x55
func ParseSetSwitchPowerResponse(frame *Frame) (result byte, err error) {
	if frame.Command != CmdSetSwitchPower {
		return 0, fmt.Errorf("命令码错误: 期望0x%02X, 实际0x%02X", CmdSetSwitchPower, frame.Command)
	}
	if len(frame.Data) < 1 {
		return 0, fmt.Errorf("响应数据为空")
	}
	return frame.Data[0], nil
}

// ParseModuleInfoResponse 解析查询模块信息的响应
// 终端返回: 0xaa + ID + 长度 + 0x05 + data(Nx模块信息) + 0x55
func ParseModuleInfoResponse(frame *Frame) ([]ModelInfo, error) {
	if frame.Command != CmdQueryModuleInfo {
		return nil, fmt.Errorf("命令码错误: 期望0x%02X, 实际0x%02X", CmdQueryModuleInfo, frame.Command)
	}

	const infoSize = 11 // 每个模块信息11字节
	if len(frame.Data)%infoSize != 0 {
		return nil, fmt.Errorf("数据长度(%d)不是模块信息大小(%d)的整数倍", len(frame.Data), infoSize)
	}

	count := len(frame.Data) / infoSize
	infos := make([]ModelInfo, count)
	for i := 0; i < count; i++ {
		offset := i * infoSize
		d := frame.Data[offset:]
		infos[i] = ModelInfo{
			FreqStart:     binary.LittleEndian.Uint16(d[0:2]),
			FreqEnd:       binary.LittleEndian.Uint16(d[2:4]),
			PowerDisp:     d[4],
			TempDisp:      d[5],
			SwitchSetting: d[6],
			PowerSetting:  d[7],
			Alarm:         d[8],
			Reserved:      binary.LittleEndian.Uint16(d[9:11]),
		}
	}
	return infos, nil
}

// AlarmString 将告警字节转为可读字符串
func AlarmString(alarm byte) string {
	if alarm == 0 {
		return "无告警"
	}
	s := ""
	if alarm&0x01 != 0 {
		s += "驻波告警 "
	}
	if alarm&0x02 != 0 {
		s += "温度告警 "
	}
	if alarm&0x04 != 0 {
		s += "功率告警 "
	}
	if alarm&0x08 != 0 {
		s += "门禁告警 "
	}
	return s
}

// FormatHex 格式化字节数组为十六进制字符串
func FormatHex(data []byte) string {
	s := ""
	for i, b := range data {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%02X", b)
	}
	return s
}

// FindFrame 从缓冲区中查找完整帧，返回帧数据和剩余数据
func FindFrame(buf []byte) (frameData []byte, remaining []byte) {
	for i := 0; i < len(buf); i++ {
		if buf[i] != FrameHeader {
			continue
		}
		// 至少需要 header + id(2) + length(1) = 4 字节才能读取 length 字段
		if i+3 >= len(buf) {
			return nil, buf[i:]
		}
		frameLen := int(buf[i+3])
		if frameLen < 6 {
			continue // 无效长度，跳过
		}
		if i+frameLen > len(buf) {
			return nil, buf[i:] // 数据不完整，等待更多数据
		}
		if buf[i+frameLen-1] != FrameTail {
			continue // 帧尾不匹配，跳过
		}
		return buf[i : i+frameLen], buf[i+frameLen:]
	}
	return nil, nil
}
