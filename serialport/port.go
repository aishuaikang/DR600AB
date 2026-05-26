package serialport

import (
	"fmt"
	"strings"

	"go.bug.st/serial"
)

// ListPorts 获取系统可用串口列表
func ListPorts() ([]string, error) {
	return serial.GetPortsList()
}

// BuildMode 根据配置构建 go.bug.st/serial 的 Mode 参数
func BuildMode(cfg Config) (*serial.Mode, error) {
	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
	}

	switch cfg.StopBits {
	case 1:
		mode.StopBits = serial.OneStopBit
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		return nil, fmt.Errorf("不支持的停止位: %d，仅支持 1 或 2", cfg.StopBits)
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Parity)) {
	case "none", "":
		mode.Parity = serial.NoParity
	case "even":
		mode.Parity = serial.EvenParity
	case "odd":
		mode.Parity = serial.OddParity
	default:
		return nil, fmt.Errorf("不支持的校验位: %s，仅支持 none/even/odd", cfg.Parity)
	}

	return mode, nil
}

// Open 打开串口连接。
// 若 cfg.PortName 为空，会调用 SelectPort 交互式选择，并回填到 cfg 中。
func Open(cfg *Config) (serial.Port, error) {
	if cfg.PortName == "" {
		selectedPort, err := SelectPort()
		if err != nil {
			return nil, err
		}
		cfg.PortName = selectedPort
	}

	mode, err := BuildMode(*cfg)
	if err != nil {
		return nil, err
	}

	port, err := serial.Open(cfg.PortName, mode)
	if err != nil {
		return nil, err
	}

	if cfg.ReadTimeout > 0 {
		port.SetReadTimeout(cfg.ReadTimeout)
	}

	return port, nil
}
