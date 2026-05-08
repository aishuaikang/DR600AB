package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/chzyer/readline"

	"tri-detector/client"
	"tri-detector/parser"
)

// OutputMode 控制串口接收数据的打印方式。
type OutputMode string

const (
	OutputParsed OutputMode = "parsed"
	OutputRaw    OutputMode = "raw"
	OutputBoth   OutputMode = "both"
)

// OutputModeState 保存运行时可切换的接收输出模式。
type OutputModeState struct {
	mu   sync.RWMutex
	mode OutputMode
}

// NewOutputModeState 创建输出模式状态。
func NewOutputModeState(mode OutputMode) *OutputModeState {
	return &OutputModeState{mode: mode}
}

// Get 返回当前输出模式。
func (s *OutputModeState) Get() OutputMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// Set 更新当前输出模式。
func (s *OutputModeState) Set(mode OutputMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

// ParseOutputMode 解析输出模式字符串。
func ParseOutputMode(value string) (OutputMode, error) {
	mode := OutputMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case OutputParsed, OutputRaw, OutputBoth:
		return mode, nil
	default:
		return "", fmt.Errorf("无效输出模式 %q，可选: parsed/raw/both", value)
	}
}

// ReadLoop 持续从客户端读取行数据并解析输出
func ReadLoop(c *client.SerialClient, mode *OutputModeState, output io.Writer) {
	c.ReadLoop(func(line string) {
		WriteReceivedLine(output, line, mode.Get())
	})
}

// WriteReceivedLine 按输出模式打印接收到的单行数据。
func WriteReceivedLine(w io.Writer, line string, mode OutputMode) {
	if mode == OutputRaw || mode == OutputBoth {
		fmt.Fprintf(w, "[RAW] %s\n", line)
	}
	if mode == OutputRaw {
		return
	}

	msg, err := parser.ParseLine(line)
	if err != nil {
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "[PARSED] %s\n", payload)
}

// ReadlineInputLoop 从 readline 读取用户输入并通过客户端发送。
func ReadlineInputLoop(rl *readline.Instance, c client.Client, outputMode *OutputModeState) bool {
	line, err := rl.Readline()
	if err == readline.ErrInterrupt {
		return false
	}
	if err != nil {
		return true
	}

	return HandleInputLine(rl.Stdout(), line, c, outputMode)
}

// InputLoop 从 stdin 读取用户输入并通过客户端发送，返回 true 表示用户请求退出
func InputLoop(inputScanner *bufio.Scanner, c client.Client, outputMode *OutputModeState) bool {
	if !inputScanner.Scan() {
		if err := inputScanner.Err(); err != nil {
			fmt.Fprintf(os.Stdout, "读取输入失败: %v\n", err)
		}
		return true
	}

	return HandleInputLine(os.Stdout, inputScanner.Text(), c, outputMode)
}

// HandleInputLine 处理一行用户输入，返回 true 表示用户请求退出。
func HandleInputLine(w io.Writer, line string, c client.Client, outputMode *OutputModeState) bool {
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "exit") {
		fmt.Fprintln(w, "退出。")
		return true
	}
	if HandleLocalCommand(w, line, outputMode) {
		return false
	}
	if line == "" {
		return false
	}

	if err := c.Send(line); err != nil {
		fmt.Fprintf(w, "发送失败: %v\n", err)
		return false
	}
	fmt.Fprintf(w, "[TX] %q\n", line)
	return false
}

// HandleLocalCommand 处理本地 CLI 命令，返回 true 表示该行已处理且不应发送到设备。
func HandleLocalCommand(w io.Writer, line string, outputMode *OutputModeState) bool {
	if !strings.HasPrefix(line, "/") {
		return false
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}

	switch strings.ToLower(fields[0]) {
	case "/mode":
		switch len(fields) {
		case 1:
			fmt.Fprintf(w, "当前输出模式: %s\n", outputMode.Get())
		case 2:
			mode, err := ParseOutputMode(fields[1])
			if err != nil {
				fmt.Fprintln(w, err)
				return true
			}
			outputMode.Set(mode)
			fmt.Fprintf(w, "输出模式已切换为: %s\n", mode)
		default:
			fmt.Fprintln(w, "用法: /mode raw|parsed|both")
		}
	case "/help":
		fmt.Fprintln(w, "本地命令: /mode raw|parsed|both, /mode, /help, exit")
	default:
		fmt.Fprintf(w, "未知本地命令: %s\n", fields[0])
	}

	return true
}

// NewInputScanner 创建标准输入扫描器
func NewInputScanner() *bufio.Scanner {
	return bufio.NewScanner(os.Stdin)
}
