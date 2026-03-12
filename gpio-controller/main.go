package main

import (
	"fmt"
	"gpio-controller/gpio"
	"gpio-controller/ui"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║         GPIO 控制工具 - 高低电平控制          ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// 1. 显示系统 GPIO 信息
	printGPIOInfo()

	// 2. 选择并初始化引脚（支持重试）
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
				fmt.Printf("初始化 GPIO%d 失败: %v\n请重新选择引脚\n\n", pinNum, err)
				continue
			}
			break
		}
	}
	fmt.Printf("已初始化: GPIO%d (输出模式)\n", pinNum)

	// 3. 创建 CLI 应用
	app := ui.NewApp(pin)

	// 4. 捕获退出信号，自动清理引脚
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在清理 GPIO 引脚...")
		app.Pin.Cleanup()
		os.Exit(0)
	}()

	// 5. 启动 CLI 应用
	app.Run()

	// 6. 正常退出时清理
	app.Pin.Cleanup()
}

// printGPIOInfo 启动时打印系统 GPIO 信息
func printGPIOInfo() {
	chips := gpio.ListGPIOChips()
	if len(chips) > 0 {
		fmt.Println("系统 GPIO 控制器:")
		fmt.Println("  ┌──────────────┬────────────────────┬──────┬──────┬──────────────┐")
		fmt.Println("  │     芯片     │        标签        │ 起始 │ 数量 │   引脚范围   │")
		fmt.Println("  ├──────────────┼────────────────────┼──────┼──────┼──────────────┤")
		for _, c := range chips {
			fmt.Printf("  │ %-12s │ %-18s │ %4d │ %4d │ %4d - %-5d │\n",
				c.Name, c.Label, c.Base, c.Ngpio, c.Base, c.Base+c.Ngpio-1)
		}
		fmt.Println("  └──────────────┴────────────────────┴──────┴──────┴──────────────┘")
	}

	exported := gpio.ListExportedPins()
	if len(exported) > 0 {
		fmt.Printf("  已导出的引脚: ")
		for i, num := range exported {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("GPIO%d", num)
		}
		fmt.Println()
	}
	fmt.Println()
}
