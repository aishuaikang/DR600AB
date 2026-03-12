package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tri-detector/client"
	"tri-detector/handler"
)

// App 三合一侵测板应用程序
type App struct {
	client   *client.SerialClient
	portName string
	baudRate int
}

// NewApp 创建应用实例
func NewApp(c *client.SerialClient, portName string, baudRate int) *App {
	return &App{
		client:   c,
		portName: portName,
		baudRate: baudRate,
	}
}

// Run 启动应用主循环：
// 1. 后台 goroutine 持续读取串口数据并解析输出
// 2. 前台接收用户输入并通过客户端发送到串口
// 3. 支持 Ctrl+C / SIGTERM 优雅退出
func (a *App) Run() {
	fmt.Printf("串口已打开: %s (%d bps)\n", a.portName, a.baudRate)
	fmt.Println("输入要发送的命令后回车，输入 exit 退出。")

	go handler.ReadLoop(a.client)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	inputScanner := handler.NewInputScanner()
	for {
		select {
		case <-sigChan:
			fmt.Println("\n收到退出信号，程序结束。")
			return
		default:
		}

		if handler.InputLoop(inputScanner, a.client) {
			return
		}
	}
}
