package main

import (
	"flag"
	"fmt"
	"log"

	"go.bug.st/serial"

	"tri-detector/client"
	"tri-detector/ui"

	"serialport"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       三合一无人机侦测板 - 数据解析工具       ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	portName := flag.String("port", "", "兼容参数：接收和发送使用同一个串口")
	rxPortName := flag.String("rx-port", "", "接收数据串口名称，例如 macOS: /dev/tty.usbserial-XXXX")
	txPortName := flag.String("tx-port", "", "发送命令串口名称，例如 macOS: /dev/tty.usbserial-YYYY")
	baudRate := flag.Int("baud", 460800, "波特率")
	dataBits := flag.Int("data", 8, "数据位")
	stopBits := flag.Int("stop", 1, "停止位(1 或 2)")
	parity := flag.String("parity", "none", "校验位: none/even/odd")
	verbose := flag.Bool("verbose", false, "打印收发数据")
	flag.Parse()

	baseCfg := serialport.Config{
		BaudRate: *baudRate,
		DataBits: *dataBits,
		StopBits: *stopBits,
		Parity:   *parity,
	}

	readPort, readPortName, writePort, writePortName, err := openDetectorPorts(baseCfg, *portName, *rxPortName, *txPortName)
	if err != nil {
		log.Fatal(err)
	}

	c := client.NewDuplexSerialClient(readPort, readPortName, writePort, writePortName, *verbose)
	defer c.Close()

	app := ui.NewDuplexApp(c, readPortName, writePortName, baseCfg.BaudRate)
	app.Run()
}

func openDetectorPorts(baseCfg serialport.Config, legacyPortName, rxPortName, txPortName string) (serial.Port, string, serial.Port, string, error) {
	if rxPortName == "" {
		rxPortName = legacyPortName
	}
	if txPortName == "" {
		txPortName = legacyPortName
	}

	if rxPortName == "" {
		selectedPort, err := serialport.SelectPortWithLabel("选择接收数据串口")
		if err != nil {
			return nil, "", nil, "", err
		}
		rxPortName = selectedPort
	}
	if txPortName == "" {
		selectedPort, err := serialport.SelectPortWithLabel("选择发送命令串口")
		if err != nil {
			return nil, "", nil, "", err
		}
		txPortName = selectedPort
	}

	rxCfg := baseCfg
	rxCfg.PortName = rxPortName
	readPort, err := serialport.Open(&rxCfg)
	if err != nil {
		return nil, "", nil, "", fmt.Errorf("打开接收数据串口失败: %w", err)
	}

	if txPortName == rxCfg.PortName {
		return readPort, rxCfg.PortName, readPort, rxCfg.PortName, nil
	}

	txCfg := baseCfg
	txCfg.PortName = txPortName
	writePort, err := serialport.Open(&txCfg)
	if err != nil {
		readPort.Close()
		return nil, "", nil, "", fmt.Errorf("打开发送命令串口失败: %w", err)
	}

	return readPort, rxCfg.PortName, writePort, txCfg.PortName, nil
}
