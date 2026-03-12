package main

import (
	"fmt"
	"io-controller/client"
	"io-controller/device"
	"io-controller/ui"
	"log"
	"time"

	"serialport"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║    TTL 串口控制工具 - 副本模块协议 V1.0      ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// 1. 选择并打开串口
	cfg := serialport.DefaultConfig(9600)
	cfg.ReadTimeout = 2 * time.Second

	port, err := serialport.Open(&cfg)
	if err != nil {
		log.Fatalf("打开串口失败: %v", err)
	}
	fmt.Printf("已打开串口: %s (波特率: %d)\n", cfg.PortName, cfg.BaudRate)

	// 2. 创建串口客户端（协议层包装）
	serialClient := client.NewSerialClient(port, cfg.PortName, true)
	defer serialClient.Close()

	// 3. 创建设备（ID 固定 0xFFFF）
	dev := device.New(serialClient, 0xFFFF)
	fmt.Printf("设备ID: 0x%04X\n", dev.ID)

	// 4. 启动 CLI 应用
	app := ui.NewApp(dev)
	app.Run()
}
