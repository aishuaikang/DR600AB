package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chzyer/readline"

	"tri-detector/client"
	"tri-detector/handler"
)

// App 三合一侵测板应用程序
type App struct {
	client     *client.SerialClient
	portName   string
	baudRate   int
	outputMode *handler.OutputModeState
}

// NewApp 创建应用实例
func NewApp(c *client.SerialClient, portName string, baudRate int) *App {
	return &App{
		client:     c,
		portName:   portName,
		baudRate:   baudRate,
		outputMode: handler.NewOutputModeState(handler.OutputRaw),
	}
}

// Run 启动应用主循环：
// 1. 后台 goroutine 持续读取串口数据并解析输出
// 2. 前台接收用户输入并通过客户端发送到串口
// 3. 支持 Ctrl+C / SIGTERM 优雅退出
func (a *App) Run() {
	fmt.Printf("串口已打开: %s (%d bps)\n", a.portName, a.baudRate)
	fmt.Println("输入要发送的命令后回车，输入 exit 退出。")
	fmt.Printf("当前接收输出模式: %s。输入 /mode raw|parsed|both 切换。\n", a.outputMode.Get())

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		HistoryLimit:    -1,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("初始化命令行失败: %v\n", err)
		return
	}
	defer rl.Close()

	output := rl.Stdout()
	a.client.SetOutput(output)

	go handler.ReadLoop(a.client, a.outputMode, output)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n收到退出信号，程序结束。")
			return
		default:
		}

		if handler.ReadlineInputLoop(rl, a.client, a.outputMode) {
			return
		}
	}
}
