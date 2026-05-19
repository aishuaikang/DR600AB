package main

import (
	"flag"
	"fmt"
	"log"

	"gnss-spoofer/client"
	"gnss-spoofer/ui"
	"serialport"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║          GNSS 导航诱骗设备串口工具          ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	portName := flag.String("port", "", "串口名称，例如 macOS: /dev/tty.usbserial-XXXX，Linux: /dev/ttyUSB0")
	baudRate := flag.Int("baud", 115200, "波特率")
	dataBits := flag.Int("data", 8, "数据位")
	stopBits := flag.Int("stop", 1, "停止位(1 或 2)")
	parity := flag.String("parity", "none", "校验位: none/even/odd")
	verbose := flag.Bool("verbose", false, "打印原始收发字节")
	flag.Parse()

	cfg := serialport.Config{
		PortName: *portName,
		BaudRate: *baudRate,
		DataBits: *dataBits,
		StopBits: *stopBits,
		Parity:   *parity,
	}
	if cfg.PortName == "" {
		selectedPort, err := serialport.SelectPortWithLabel("选择 GNSS 诱骗设备串口")
		if err != nil {
			log.Fatal(err)
		}
		cfg.PortName = selectedPort
	}

	port, err := serialport.Open(&cfg)
	if err != nil {
		log.Fatalf("打开诱骗设备串口失败: %v", err)
	}

	serialClient := client.NewSerialClient(port, cfg.PortName, *verbose)
	defer serialClient.Close()

	app := ui.NewApp(serialClient, cfg.PortName, cfg.BaudRate)
	app.Run()
}
