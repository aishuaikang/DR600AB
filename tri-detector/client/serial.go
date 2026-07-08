package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// SerialClient 串口文本行协议客户端，包装已打开的串口连接，
// 提供基于文本行的收发能力。
type SerialClient struct {
	readPort        serial.Port
	writePort       serial.Port
	readPortName    string
	writePortName   string
	sharedPort      bool
	closeOnce       sync.Once
	readerOnce      sync.Once
	scanner         *bufio.Scanner
	readCh          chan serialReadResult
	done            chan struct{}
	readLineTimeout time.Duration
	verbose         bool
	output          io.Writer
}

type serialReadResult struct {
	line string
	err  error
}

// NewSerialClient 创建串口客户端，包装已打开的串口连接。
// portName 用于日志标识，verbose 控制是否打印收发数据。
func NewSerialClient(port serial.Port, portName string, verbose bool) *SerialClient {
	return newSerialClient(port, portName, port, portName, true, verbose)
}

// NewDuplexSerialClient 创建收发分离的串口客户端。
// readPort 仅用于接收设备上报数据，writePort 仅用于发送命令。
func NewDuplexSerialClient(readPort serial.Port, readPortName string, writePort serial.Port, writePortName string, verbose bool) *SerialClient {
	return newSerialClient(readPort, readPortName, writePort, writePortName, readPortName == writePortName, verbose)
}

func newSerialClient(readPort serial.Port, readPortName string, writePort serial.Port, writePortName string, sharedPort bool, verbose bool) *SerialClient {
	scanner := bufio.NewScanner(readPort)
	scanner.Split(scanSerialRecords)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	return &SerialClient{
		readPort:        readPort,
		writePort:       writePort,
		readPortName:    readPortName,
		writePortName:   writePortName,
		sharedPort:      sharedPort,
		scanner:         scanner,
		readCh:          make(chan serialReadResult, 64),
		done:            make(chan struct{}),
		readLineTimeout: 5 * time.Second,
		verbose:         verbose,
		output:          os.Stdout,
	}
}

// SetOutput 设置客户端日志输出位置。
func (c *SerialClient) SetOutput(w io.Writer) {
	if w == nil {
		c.output = os.Stdout
		return
	}
	c.output = w
}

// Send 发送一行文本命令（自动追加 CRLF 换行符）。
func (c *SerialClient) Send(cmd string) error {
	cmd = strings.TrimRight(cmd, "\r\n") + "\r\n"

	if c.verbose {
		fmt.Fprintf(c.output, "  -> 发送: %q\n", strings.TrimRight(cmd, "\r\n"))
	}

	n, err := c.writePort.Write([]byte(cmd))
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
	c.ensureReader()
	timeout := c.readLineTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r, ok := <-c.readCh:
		if !ok {
			return "", fmt.Errorf("连接已关闭")
		}
		if r.err != nil {
			return "", r.err
		}
		if c.verbose {
			fmt.Fprintf(c.output, "  <- 接收: %q\n", r.line)
		}
		return r.line, nil
	case <-timer.C:
		return "", fmt.Errorf("接收超时(%s)", timeout)
	case <-c.done:
		return "", fmt.Errorf("连接已关闭")
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
	c.ensureReader()
	for {
		select {
		case r, ok := <-c.readCh:
			if !ok {
				return
			}
			if r.err != nil {
				fmt.Fprintf(c.output, "读取错误: %v\n", r.err)
				return
			}
			line := strings.TrimSpace(r.line)
			if line == "" {
				continue
			}
			if c.verbose {
				fmt.Fprintf(c.output, "  <- 接收: %q\n", line)
			}
			handler(line)
		case <-c.done:
			return
		}
	}
}

func (c *SerialClient) ensureReader() {
	c.readerOnce.Do(func() {
		go c.readLoop()
	})
}

func (c *SerialClient) readLoop() {
	defer close(c.readCh)
	for c.scanner.Scan() {
		line := strings.TrimSpace(c.scanner.Text())
		if line == "" {
			continue
		}
		select {
		case c.readCh <- serialReadResult{line: line}:
		case <-c.done:
			return
		}
	}
	if err := c.scanner.Err(); err != nil {
		select {
		case c.readCh <- serialReadResult{err: err}:
		case <-c.done:
		}
	}
}

// Close 关闭串口
func (c *SerialClient) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.readPort != nil {
			c.readPort.Close()
		}
		if !c.sharedPort && c.writePort != nil {
			c.writePort.Close()
		}
	})
}

// PortName 返回串口名称
func (c *SerialClient) PortName() string {
	return c.readPortName
}

// ReadPortName 返回接收数据串口名称。
func (c *SerialClient) ReadPortName() string {
	return c.readPortName
}

// WritePortName 返回发送命令串口名称。
func (c *SerialClient) WritePortName() string {
	return c.writePortName
}

// 编译期接口实现检查
var _ Client = (*SerialClient)(nil)
