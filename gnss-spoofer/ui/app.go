package ui

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"

	"gnss-spoofer/client"
	"gnss-spoofer/protocol"
)

const commandTimeout = 5 * time.Second

type App struct {
	client   *client.SerialClient
	portName string
	baudRate int
}

func NewApp(c *client.SerialClient, portName string, baudRate int) *App {
	return &App{
		client:   c,
		portName: portName,
		baudRate: baudRate,
	}
}

func (a *App) Run() {
	fmt.Printf("诱骗设备串口已打开: %s (%d bps)\n", a.portName, a.baudRate)
	fmt.Println("输入 /help 查看命令，输入 exit 退出。")

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "gnss> ",
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
	a.client.Start()
	go printFrames(a.client.Frames(), output)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Fprintln(output, "\n收到退出信号，程序结束。")
			return
		default:
		}

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		}
		if err != nil {
			return
		}
		if a.handleLine(output, line) {
			return
		}
	}
}

func printFrames(frames <-chan client.ReceivedFrame, output io.Writer) {
	for rec := range frames {
		if rec.Err != nil {
			fmt.Fprintf(output, "[RX ERROR] %v\n", rec.Err)
			continue
		}
		fmt.Fprintf(output, "[RX] %s\n", protocol.DescribeFrame(rec.Frame))
	}
}

func (a *App) handleLine(w io.Writer, line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
		fmt.Fprintln(w, "退出。")
		return true
	}
	if !strings.HasPrefix(line, "/") {
		fmt.Fprintln(w, "请输入 /help 查看可用命令。")
		return false
	}

	fields := strings.Fields(line)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "/help":
		printHelp(w)
	case "/status":
		a.sendQuery(w, protocol.QueryDeviceStatus)
	case "/query":
		a.handleQuery(w, args)
	case "/time":
		a.sendAckCommand(w, protocol.CmdSystemTime, func() ([]byte, error) {
			return protocol.BuildSetSystemTime(time.Now().UTC())
		})
	case "/tx":
		a.handleTransmit(w, args)
	case "/atten":
		a.handleAttenuation(w, args)
	case "/delay":
		a.handleDelay(w, args)
	case "/simpos":
		a.handlePosition(w, args, true)
	case "/devicepos":
		a.handlePosition(w, args, false)
	case "/target":
		a.handleTarget(w, args)
	case "/coord":
		a.handleCoordinateControl(w, args)
	case "/circle":
		a.handleSpoofCircle(w, args)
	case "/random":
		a.handleRandom(w, args)
	case "/suppression":
		a.handleSuppression(w, args)
	case "/timedsearch":
		a.handleTimedSearch(w, args)
	case "/reboot":
		a.handleReboot(w, args)
	case "/hex":
		a.handleHex(w, args)
	default:
		fmt.Fprintf(w, "未知命令: %s。输入 /help 查看帮助。\n", cmd)
	}
	return false
}

func (a *App) handleQuery(w io.Writer, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(w, "用法: /query status|tx|version|time|power|target|circle|suppression|signals|location|random|delay|timedsearch")
		return
	}
	queryMap := map[string]byte{
		"status":      protocol.QueryDeviceStatus,
		"tx":          protocol.QueryTransmitSwitch,
		"version":     protocol.QueryFirmwareVersion,
		"time":        protocol.QuerySystemTime,
		"power":       protocol.QueryPowerAttenuation,
		"target":      protocol.QueryTargetPosition,
		"circle":      protocol.QuerySpoofCircle,
		"suppression": protocol.QuerySuppression,
		"signals":     protocol.QueryDeviceSignal,
		"location":    protocol.QueryDevicePosition,
		"random":      protocol.QueryRandomPosition,
		"delay":       protocol.QuerySignalDelay,
		"timedsearch": protocol.QueryTimedSearch,
	}
	query, ok := queryMap[strings.ToLower(args[0])]
	if !ok {
		fmt.Fprintf(w, "不支持的查询项: %s\n", args[0])
		return
	}
	a.sendQuery(w, query)
}

func (a *App) handleTransmit(w io.Writer, args []string) {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(w, "用法: /tx on|off [all|gps,bds,glo,gal]")
		return
	}
	enabled, err := parseOnOff(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	mask := uint16(0)
	if enabled {
		mask = protocol.SignalAllSupported
		if len(args) == 2 {
			mask, err = parseSignalMask(args[1])
			if err != nil {
				fmt.Fprintln(w, err)
				return
			}
		}
	}
	a.sendAckCommand(w, protocol.CmdTransmitSwitch, func() ([]byte, error) {
		return protocol.BuildSetTransmitSwitch(mask)
	})
}

func (a *App) handleAttenuation(w io.Writer, args []string) {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(w, "用法: /atten <0-80dB> [all|gps,bds,glo,gal]")
		return
	}
	db, err := parseUint8(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	mask := protocol.SignalAllSupported
	if len(args) == 2 {
		mask, err = parseSignalMask(args[1])
		if err != nil {
			fmt.Fprintln(w, err)
			return
		}
	}
	a.sendAckCommand(w, protocol.CmdPowerAttenuation, func() ([]byte, error) {
		return protocol.BuildSetPowerAttenuation(mask, db)
	})
}

func (a *App) handleDelay(w io.Writer, args []string) {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(w, "用法: /delay <ns> [all|gps,bds,glo,gal]")
		return
	}
	delay, err := parseFloat32(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	mask := protocol.SignalAllSupported
	if len(args) == 2 {
		mask, err = parseSignalMask(args[1])
		if err != nil {
			fmt.Fprintln(w, err)
			return
		}
	}
	a.sendAckCommand(w, protocol.CmdSignalDelay, func() ([]byte, error) {
		return protocol.BuildSetSignalDelay(mask, delay)
	})
}

func (a *App) handlePosition(w io.Writer, args []string, simulated bool) {
	if len(args) != 3 {
		if simulated {
			fmt.Fprintln(w, "用法: /simpos <经度> <纬度> <高度m>")
		} else {
			fmt.Fprintln(w, "用法: /devicepos <经度> <纬度> <高度m>")
		}
		return
	}
	lon, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		fmt.Fprintln(w, "经度格式错误")
		return
	}
	lat, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		fmt.Fprintln(w, "纬度格式错误")
		return
	}
	alt, err := parseInt32(args[2])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	command := protocol.CmdDevicePosition
	builder := func() ([]byte, error) {
		return protocol.BuildSetDevicePosition(lon, lat, alt)
	}
	if simulated {
		command = protocol.CmdSimulatedPosition
		builder = func() ([]byte, error) {
			return protocol.BuildSetSimulatedPosition(lon, lat, alt)
		}
	}
	a.sendAckCommand(w, command, builder)
}

func (a *App) handleTarget(w io.Writer, args []string) {
	if len(args) != 4 {
		fmt.Fprintln(w, "用法: /target <距离m> <高度m> <方向角deg> <航向角deg>")
		return
	}
	distance, height, direction, heading, ok := parseTargetArgs(w, args)
	if !ok {
		return
	}
	a.sendAckCommand(w, protocol.CmdTargetPosition, func() ([]byte, error) {
		return protocol.BuildSetTargetPosition(distance, height, direction, heading)
	})
}

func (a *App) handleCoordinateControl(w io.Writer, args []string) {
	if len(args) != 5 {
		fmt.Fprintln(w, "用法: /coord <水平步进m> <水平方向0-4> <垂直步进m> <垂直方向0-2> <持续秒>")
		return
	}
	values := make([]int32, len(args))
	for i, arg := range args {
		value, err := parseInt32(arg)
		if err != nil {
			fmt.Fprintln(w, err)
			return
		}
		values[i] = value
	}
	a.sendAckCommand(w, protocol.CmdCoordinateControl, func() ([]byte, error) {
		return protocol.BuildSetCoordinateControl(values[0], values[1], values[2], values[3], values[4])
	})
}

func (a *App) handleSpoofCircle(w io.Writer, args []string) {
	if len(args) != 7 {
		fmt.Fprintln(w, "用法: /circle <距离m> <高度m> <方向角deg> <航向角deg> <半径m> <周期s> <cw|ccw>")
		return
	}
	distance, height, direction, heading, ok := parseTargetArgs(w, args[:4])
	if !ok {
		return
	}
	radius, err := parseFloat32(args[4])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	period, err := parseFloat32(args[5])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	rotateDirection, err := parseRotateDirection(args[6])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	a.sendAckCommand(w, protocol.CmdSpoofCircle, func() ([]byte, error) {
		return protocol.BuildSetSpoofCircle(distance, height, direction, heading, radius, period, rotateDirection)
	})
}

func (a *App) handleRandom(w io.Writer, args []string) {
	if len(args) != 3 {
		fmt.Fprintln(w, "用法: /random on|off <半径m> <刷新周期s>")
		return
	}
	enabled, err := parseOnOff(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	radius, err := parseUint32(args[1])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	period, err := parseUint32(args[2])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	a.sendAckCommand(w, protocol.CmdRandomPosition, func() ([]byte, error) {
		return protocol.BuildSetRandomPosition(enabled, radius, period)
	})
}

func (a *App) handleSuppression(w io.Writer, args []string) {
	if len(args) != 2 {
		fmt.Fprintln(w, "用法: /suppression <波形掩码> on|off")
		return
	}
	waveformMask, err := parseInt32(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	enabled, err := parseOnOff(args[1])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	a.sendAckCommand(w, protocol.CmdSuppression, func() ([]byte, error) {
		return protocol.BuildSetSuppression(waveformMask, enabled)
	})
}

func (a *App) handleTimedSearch(w io.Writer, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(w, "用法: /timedsearch on|off")
		return
	}
	enabled, err := parseOnOff(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	a.sendAckCommand(w, protocol.CmdTimedSearch, func() ([]byte, error) {
		return protocol.BuildSetTimedSearch(enabled)
	})
}

func (a *App) handleReboot(w io.Writer, args []string) {
	if len(args) != 1 || strings.ToLower(args[0]) != "confirm" {
		fmt.Fprintln(w, "设备重启需要确认: /reboot confirm")
		return
	}
	a.sendAckCommand(w, protocol.CmdDeviceReboot, protocol.BuildReboot)
}

func (a *App) handleHex(w io.Writer, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(w, "用法: /hex EB 90 ...")
		return
	}
	raw := strings.Join(args, "")
	raw = strings.ReplaceAll(raw, "0x", "")
	raw = strings.ReplaceAll(raw, "0X", "")
	data, err := hex.DecodeString(raw)
	if err != nil {
		fmt.Fprintf(w, "十六进制格式错误: %v\n", err)
		return
	}
	if err := a.client.Send(data); err != nil {
		fmt.Fprintf(w, "发送失败: %v\n", err)
		return
	}
	fmt.Fprintf(w, "[TX] %s\n", protocol.Hex(data))
}

func (a *App) sendQuery(w io.Writer, command byte) {
	data, err := protocol.BuildQuery(command)
	if err != nil {
		fmt.Fprintf(w, "构建查询失败: %v\n", err)
		return
	}
	a.sendAndWait(w, data, protocol.ControlReport, command)
}

func (a *App) sendAckCommand(w io.Writer, command byte, build func() ([]byte, error)) {
	data, err := build()
	if err != nil {
		fmt.Fprintf(w, "构建命令失败: %v\n", err)
		return
	}
	rec, err := a.sendAndWait(w, data, protocol.ControlAck, command)
	if err != nil {
		return
	}
	ack, err := protocol.ParseAck(rec.Frame)
	if err != nil {
		fmt.Fprintf(w, "解析应答失败: %v\n", err)
		return
	}
	if !ack.Success() {
		fmt.Fprintf(w, "命令失败: return=%d error=%d(%s)\n", ack.ReturnValue, ack.ErrorCode, protocol.AckErrorText(ack.ErrorCode))
		return
	}
	fmt.Fprintln(w, "命令执行成功。")
}

func (a *App) sendAndWait(w io.Writer, data []byte, control protocol.ControlWord, command byte) (client.ReceivedFrame, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	fmt.Fprintf(w, "[TX] %s\n", protocol.Hex(data))
	rec, err := a.client.SendAndWait(ctx, data, func(frame protocol.Frame) bool {
		return frame.Control == control && frame.Command() == command
	})
	if err != nil {
		fmt.Fprintf(w, "等待响应失败: %v\n", err)
		return client.ReceivedFrame{}, err
	}
	return rec, nil
}

func parseTargetArgs(w io.Writer, args []string) (int32, int32, float32, float32, bool) {
	distance, err := parseInt32(args[0])
	if err != nil {
		fmt.Fprintln(w, err)
		return 0, 0, 0, 0, false
	}
	height, err := parseInt32(args[1])
	if err != nil {
		fmt.Fprintln(w, err)
		return 0, 0, 0, 0, false
	}
	direction, err := parseFloat32(args[2])
	if err != nil {
		fmt.Fprintln(w, err)
		return 0, 0, 0, 0, false
	}
	heading, err := parseFloat32(args[3])
	if err != nil {
		fmt.Fprintln(w, err)
		return 0, 0, 0, 0, false
	}
	return distance, height, direction, heading, true
}

func parseSignalMask(value string) (uint16, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "all" {
		return protocol.SignalAllSupported, nil
	}
	if value == "none" {
		return 0, nil
	}

	var mask uint16
	for _, part := range strings.Split(value, ",") {
		switch strings.TrimSpace(part) {
		case "gps", "gps_l1ca", "l1":
			mask |= protocol.SignalGPSL1CA
		case "bds", "bds_b1i", "b1":
			mask |= protocol.SignalBDSB1I
		case "glo", "glo_l1":
			mask |= protocol.SignalGLOL1
		case "gal", "gal_e1", "e1":
			mask |= protocol.SignalGALE1
		default:
			return 0, fmt.Errorf("未知信号类型: %s", part)
		}
	}
	return mask, nil
}

func parseOnOff(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "1", "true", "open", "enable":
		return true, nil
	case "off", "0", "false", "close", "disable":
		return false, nil
	default:
		return false, fmt.Errorf("开关值必须是 on 或 off")
	}
}

func parseRotateDirection(value string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cw", "0", "clockwise":
		return 0, nil
	case "ccw", "1", "counterclockwise":
		return 1, nil
	default:
		return 0, fmt.Errorf("运动方向必须是 cw 或 ccw")
	}
}

func parseInt32(value string) (int32, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("整数格式错误: %s", value)
	}
	return int32(parsed), nil
}

func parseUint32(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("无符号整数格式错误: %s", value)
	}
	return uint32(parsed), nil
}

func parseUint8(value string) (byte, error) {
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("0~255 整数格式错误: %s", value)
	}
	return byte(parsed), nil
}

func parseFloat32(value string) (float32, error) {
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return 0, fmt.Errorf("数字格式错误: %s", value)
	}
	return float32(parsed), nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "可用命令:")
	fmt.Fprintln(w, "  /status                                      查询设备状态")
	fmt.Fprintln(w, "  /query status|tx|version|time|power|target|circle|suppression|signals|location|random|delay|timedsearch")
	fmt.Fprintln(w, "  /time                                        同步 UTC 系统时间")
	fmt.Fprintln(w, "  /tx on|off [all|gps,bds,glo,gal]             设置发射开关")
	fmt.Fprintln(w, "  /atten <0-80dB> [all|gps,bds,glo,gal]        设置功率衰减")
	fmt.Fprintln(w, "  /delay <ns> [all|gps,bds,glo,gal]            设置信号时延")
	fmt.Fprintln(w, "  /simpos <经度> <纬度> <高度m>                 设置模拟位置")
	fmt.Fprintln(w, "  /devicepos <经度> <纬度> <高度m>              设置设备位置")
	fmt.Fprintln(w, "  /target <距离m> <高度m> <方向角> <航向角>      设置目标位置")
	fmt.Fprintln(w, "  /coord <水平步进> <水平方向> <垂直步进> <垂直方向> <持续秒>")
	fmt.Fprintln(w, "  /circle <距离> <高度> <方向角> <航向角> <半径> <周期> <cw|ccw>")
	fmt.Fprintln(w, "  /random on|off <半径m> <刷新周期s>            设置随机坐标")
	fmt.Fprintln(w, "  /suppression <波形掩码> on|off               设置压制信号发射")
	fmt.Fprintln(w, "  /timedsearch on|off                          设置定时搜星使能")
	fmt.Fprintln(w, "  /reboot confirm                              重启设备")
	fmt.Fprintln(w, "  /hex EB 90 ...                               发送原始十六进制帧")
	fmt.Fprintln(w, "  exit                                         退出")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "方向说明: 水平方向 0不移动 1前进 2后退 3左移 4右移；垂直方向 0不移动 1升高 2降低。")
}
