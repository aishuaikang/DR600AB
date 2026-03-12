package client

import (
	"fmt"
	"io-controller/protocol"
	"time"

	"go.bug.st/serial"
)

// SerialClient 串口通信客户端，包装已打开的串口连接，
// 提供基于二进制帧协议的收发能力。
type SerialClient struct {
	port     serial.Port
	portName string
	recvBuf  []byte
	verbose  bool
}

// NewSerialClient 创建串口客户端，包装已打开的串口连接。
// portName 用于日志标识，verbose 控制是否打印收发数据。
func NewSerialClient(port serial.Port, portName string, verbose bool) *SerialClient {
	return &SerialClient{
		port:     port,
		portName: portName,
		recvBuf:  make([]byte, 0, 1024),
		verbose:  verbose,
	}
}

// Send 发送原始字节数据
func (c *SerialClient) Send(data []byte) error {
	if c.verbose {
		fmt.Printf("  -> 发送: %s\n", protocol.FormatHex(data))
	}
	n, err := c.port.Write(data)
	if err != nil {
		return fmt.Errorf("发送失败: %v", err)
	}
	if n != len(data) {
		return fmt.Errorf("发送不完整: 期望%d字节, 实际%d字节", len(data), n)
	}
	return nil
}

// Receive 接收一帧数据（超时5秒）
func (c *SerialClient) Receive() (*protocol.Frame, error) {
	buf := make([]byte, 256)
	c.recvBuf = c.recvBuf[:0]

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := c.port.Read(buf)
		if err != nil {
			continue
		}
		if n > 0 {
			c.recvBuf = append(c.recvBuf, buf[:n]...)

			frameData, remaining := protocol.FindFrame(c.recvBuf)
			if frameData != nil {
				if c.verbose {
					fmt.Printf("  <- 接收: %s\n", protocol.FormatHex(frameData))
				}
				c.recvBuf = remaining
				return protocol.DecodeFrame(frameData)
			}
		}
	}
	return nil, fmt.Errorf("接收超时(5秒)")
}

// SendAndReceive 发送命令并等待响应
func (c *SerialClient) SendAndReceive(data []byte) (*protocol.Frame, error) {
	if err := c.Send(data); err != nil {
		return nil, err
	}
	return c.Receive()
}

// Close 关闭串口
func (c *SerialClient) Close() {
	if c.port != nil {
		c.port.Close()
	}
}

// PortName 返回串口名称
func (c *SerialClient) PortName() string {
	return c.portName
}

// 编译期接口实现检查
var _ Client = (*SerialClient)(nil)
