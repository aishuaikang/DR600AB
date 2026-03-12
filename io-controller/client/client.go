package client

import "io-controller/protocol"

// Client 通信客户端接口，定义与设备通信的基本操作。
// 实现此接口即可接入不同传输层（串口、TCP 等）。
type Client interface {
	// Send 发送原始字节数据
	Send(data []byte) error
	// Receive 接收一帧数据
	Receive() (*protocol.Frame, error)
	// SendAndReceive 发送数据并等待响应帧
	SendAndReceive(data []byte) (*protocol.Frame, error)
	// Close 关闭连接
	Close()
}
