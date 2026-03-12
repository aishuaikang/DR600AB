package client

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

// SerialClient 串口文本行协议客户端，包装已打开的串口连接，
// 提供基于文本行的收发能力。
type SerialClient struct {
	port     serial.Port
	portName string
	scanner  *bufio.Scanner
	verbose  bool
}

// NewSerialClient 创建串口客户端，包装已打开的串口连接。
// portName 用于日志标识，verbose 控制是否打印收发数据。
func NewSerialClient(port serial.Port, portName string, verbose bool) *SerialClient {
	scanner := bufio.NewScanner(port)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	return &SerialClient{
		port:     port,
		portName: portName,
		scanner:  scanner,
		verbose:  verbose,
	}
}

// Send 发送一行文本命令（自动追加换行符）
func (c *SerialClient) Send(cmd string) error {
	if !strings.HasSuffix(cmd, "\n") {
		cmd += "\n"
	}

	if c.verbose {
		fmt.Printf("  -> 发送: %q\n", strings.TrimRight(cmd, "\n"))
	}

	n, err := c.port.Write([]byte(cmd))
	if err != nil {
		return fmt.Errorf("发送失败: %v", err)
	}
	if n != len(cmd) {
		return fmt.Errorf("发送不完整: 期望%d字节, 实际%d字节", len(cmd), n)
	}
	return nil
}

// ReadLine 读取一行文本（带超时）
func (c *SerialClient) ReadLine() (string, error) {
	type result struct {
		line string
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		if c.scanner.Scan() {
			ch <- result{line: strings.TrimSpace(c.scanner.Text())}
		} else {
			err := c.scanner.Err()
			if err == nil {
				err = fmt.Errorf("连接已关闭")
			}
			ch <- result{err: err}
		}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		if c.verbose {
			fmt.Printf("  <- 接收: %q\n", r.line)
		}
		return r.line, nil
	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("接收超时(5秒)")
	}
}

// SendAndReceive 发送命令并等待一行响应
func (c *SerialClient) SendAndReceive(cmd string) (string, error) {
	if err := c.Send(cmd); err != nil {
		return "", err
	}
	return c.ReadLine()
}

// ReadLoop 持续读取行数据，通过回调处理每一行（阻塞）。
// 适用于持续接收设备上报的侦测数据。
func (c *SerialClient) ReadLoop(handler LineHandler) {
	for c.scanner.Scan() {
		line := strings.TrimSpace(c.scanner.Text())
		if line == "" {
			continue
		}
		if c.verbose {
			fmt.Printf("  <- 接收: %q\n", line)
		}
		handler(line)
	}
	if err := c.scanner.Err(); err != nil {
		fmt.Printf("读取错误: %v\n", err)
	}
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
