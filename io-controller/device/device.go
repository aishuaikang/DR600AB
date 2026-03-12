package device

import (
	"fmt"
	"io-controller/client"
	"io-controller/protocol"
)

// Device 设备操作封装，提供高层业务命令。
// 通过 client.Client 接口与设备通信，不依赖具体传输方式。
type Device struct {
	Client client.Client
	ID     uint16
}

// New 创建设备实例
func New(c client.Client, id uint16) *Device {
	return &Device{Client: c, ID: id}
}

// SetSwitchPower 设置N路开关功率 (命令 0x04)
// 返回结果码：0x00=成功
func (d *Device) SetSwitchPower(settings []protocol.ModelSetting) (byte, error) {
	data := protocol.BuildSetSwitchPower(d.ID, settings)
	resp, err := d.Client.SendAndReceive(data)
	if err != nil {
		return 0, fmt.Errorf("设置开关功率失败: %v", err)
	}
	return protocol.ParseSetSwitchPowerResponse(resp)
}

// QueryModuleInfo 查询N路模块信息 (命令 0x05)
func (d *Device) QueryModuleInfo() ([]protocol.ModelInfo, error) {
	data := protocol.BuildQueryModuleInfo(d.ID)
	resp, err := d.Client.SendAndReceive(data)
	if err != nil {
		return nil, fmt.Errorf("查询模块信息失败: %v", err)
	}
	return protocol.ParseModuleInfoResponse(resp)
}

// SendRaw 发送原始字节数据并接收响应帧
func (d *Device) SendRaw(data []byte) (*protocol.Frame, error) {
	return d.Client.SendAndReceive(data)
}
