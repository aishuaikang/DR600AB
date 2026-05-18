package main

import (
	"fmt"
	"gpio-controller/board"
	"gpio-controller/gpio"
	"gpio-controller/ui"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

// GPIO编号（一共8个脚，只使用前三个，后面为预留）
// IOC4 20 （433 800 900 1.4）
// IOC2 18 （1.2 1.5）
// IOC3 19 （2.4 5.2 5.8）
// IOC5 21
// I3B4 108
// I3B5 109
// I3C0 112
// I3C1 113

func main() {
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║              GPIO 控制台              ║")
	fmt.Println("║         输出模式 / 电平控制工具       ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println("输入 GPIO 编号后进入控制菜单。")
	fmt.Println()
	fmt.Println("板卡 GPIO 映射:")
	for _, def := range board.DefaultPins() {
		fmt.Printf("  %s\n", board.FormatPinUsage(def))
	}
	fmt.Println()

	// 1. 选择并初始化引脚（支持重试）
	var pinNum int
	var err error
	var pin *gpio.Pin

	if len(os.Args) > 1 {
		pinNum, err = strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatalf("无效的引脚编号: %s", os.Args[1])
		}
		pin = gpio.NewPin(pinNum)
		if err := pin.Setup(); err != nil {
			log.Fatalf("初始化 GPIO%d 失败: %v", pinNum, err)
		}
	} else {
		for {
			pinNum, err = ui.PromptGPIOPin("")
			if err != nil {
				log.Fatalf("输入引脚编号失败: %v", err)
			}
			pin = gpio.NewPin(pinNum)
			if err := pin.Setup(); err != nil {
				fmt.Printf("[错误] 初始化 GPIO%d 失败: %v\n", pinNum, err)
				fmt.Println("[提示] 请重新选择引脚。")
				fmt.Println()
				continue
			}
			break
		}
	}
	fmt.Printf("[成功] 已初始化 GPIO%d，方向: 输出\n", pinNum)

	// 2. 创建 CLI 应用
	app := ui.NewApp(pin)

	// 3. 捕获退出信号，自动清理引脚
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n[提示] 正在清理 GPIO 引脚...")
		app.Cleanup()
		os.Exit(0)
	}()

	// 4. 启动 CLI 应用
	app.Run()

	// 5. 正常退出时清理
	app.Cleanup()
}
