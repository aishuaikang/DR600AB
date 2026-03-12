package serialport

import "time"

// Config 串口配置参数。
// 统一两个业务模块（io-controller / tri-detector）的串口配置需求。
type Config struct {
	PortName    string        // 串口名称（如 /dev/tty.usbserial-110），为空时 Open 会交互式选择
	BaudRate    int           // 波特率
	DataBits    int           // 数据位（5-8）
	StopBits    int           // 停止位（1 或 2）
	Parity      string        // 校验位: none / even / odd
	ReadTimeout time.Duration // 读取超时，0 表示不设超时
}

// DefaultConfig 返回 8N1 默认配置，需指定波特率。
func DefaultConfig(baudRate int) Config {
	return Config{
		BaudRate: baudRate,
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}
}
