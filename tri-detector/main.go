package main

import (
	"flag"
	"fmt"
	"log"

	"tri-detector/client"
	"tri-detector/ui"

	"serialport"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       三合一无人机侦测板 - 数据解析工具       ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	portName := flag.String("port", "", "串口名称，例如 macOS: /dev/tty.usbserial-XXXX")
	baudRate := flag.Int("baud", 115200, "波特率")
	dataBits := flag.Int("data", 8, "数据位")
	stopBits := flag.Int("stop", 1, "停止位(1 或 2)")
	parity := flag.String("parity", "none", "校验位: none/even/odd")
	verbose := flag.Bool("verbose", false, "打印收发数据")
	flag.Parse()

	cfg := serialport.Config{
		PortName: *portName,
		BaudRate: *baudRate,
		DataBits: *dataBits,
		StopBits: *stopBits,
		Parity:   *parity,
	}

	port, err := serialport.Open(&cfg)
	if err != nil {
		log.Fatal(err)
	}

	c := client.NewSerialClient(port, cfg.PortName, *verbose)
	defer c.Close()

	app := ui.NewApp(c, cfg.PortName, cfg.BaudRate)
	app.Run()
}
