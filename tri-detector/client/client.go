package client

// Client 文本行协议客户端接口，定义与侦测设备通信的基本操作。
// 实现此接口即可接入不同传输层（串口、TCP 等）。
type Client interface {
	// Send 发送一行文本命令（自动追加换行符）
	Send(cmd string) error
	// ReadLine 读取一行文本（带超时）
	ReadLine() (string, error)
	// SendAndReceive 发送命令并等待一行响应
	SendAndReceive(cmd string) (string, error)
	// Close 关闭连接
	Close()
}

// LineHandler 行数据处理回调函数
type LineHandler func(line string)
