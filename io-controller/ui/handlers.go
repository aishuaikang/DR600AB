package ui

import (
	"fmt"
	"io-controller/device"
	"io-controller/protocol"
	"strconv"
	"strings"
)

// handleStartStrike 开启打击：多选频段 -> 开启对应模块
func handleStartStrike(dev *device.Device) {
	fmt.Println("\n[开启打击] 请选择要打击的频段:")

	selectedBands, err := MultiSelectFreqBands()
	if err != nil {
		fmt.Printf("选择错误: %v\n", err)
		return
	}

	fmt.Printf("  已选频段: ")
	for _, band := range selectedBands {
		fmt.Printf("%s(模块%d) ", band, device.FreqToIndexMap[band])
	}
	fmt.Println()

	result, err := dev.StartStrike(selectedBands)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	printResult("开启打击", result)
}

// handleStopStrike 关闭打击：关闭全部模块
func handleStopStrike(dev *device.Device) {
	fmt.Println("\n[关闭打击] 关闭全部10路模块")

	result, err := dev.StopStrike()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	printResult("关闭打击", result)
}

// handleSetSwitchPower 手动设置N路开关
func handleSetSwitchPower(dev *device.Device) {
	nStr, err := PromptInputValidate("模块数量 (1-10)", "10", func(input string) error {
		v, err := strconv.Atoi(input)
		if err != nil {
			return fmt.Errorf("请输入数字")
		}
		if v < 1 || v > 10 {
			return fmt.Errorf("范围 1-10")
		}
		return nil
	})
	if err != nil {
		fmt.Printf("输入错误: %v\n", err)
		return
	}
	n, _ := strconv.Atoi(nStr)

	settings := make([]protocol.ModelSetting, n)
	for i := 0; i < n; i++ {
		swIdx, _, err := SelectFromList(fmt.Sprintf("模块%d - 开关", i+1), []string{"关 (0)", "开 (1)"})
		if err != nil {
			fmt.Printf("输入错误: %v\n", err)
			return
		}
		settings[i] = protocol.ModelSetting{
			SwitchSetting: byte(swIdx),
			PowerSetting:  device.DefaultPower,
		}
	}

	fmt.Printf("\n[设置开关功率] ID=0x%04X, %d路模块\n", dev.ID, n)
	for i, s := range settings {
		sw := "关"
		if s.SwitchSetting == 1 {
			sw = "开"
		}
		fmt.Printf("  模块%d: 开关=%s\n", i+1, sw)
	}

	result, err := dev.SetSwitchPower(settings)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	printResult("设置", result)
}

// handleQueryModuleInfo 查询模块信息并表格展示
func handleQueryModuleInfo(dev *device.Device) {
	fmt.Printf("\n[查询模块信息] ID=0x%04X\n", dev.ID)

	infos, err := dev.QueryModuleInfo()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("  共 %d 路模块:\n", len(infos))
	fmt.Println("  ┌─────┬──────────┬──────────┬──────┬──────┬──────┬──────────┬──────────┐")
	fmt.Println("  │  #  │ 起始频率 │ 结束频率 │ 功率 │ 温度 │ 开关 │ 功率设置 │   告警   │")
	fmt.Println("  ├─────┼──────────┼──────────┼──────┼──────┼──────┼──────────┼──────────┤")
	for i, info := range infos {
		sw := "关"
		if info.SwitchSetting == 1 {
			sw = "开"
		}
		fmt.Printf("  │ %3d │ %8d │ %8d │ %4d │ %4d │  %s  │   %4d   │ %-8s │\n",
			i+1, info.FreqStart, info.FreqEnd, info.PowerDisp, info.TempDisp,
			sw, info.PowerSetting, protocol.AlarmString(info.Alarm))
	}
	fmt.Println("  └─────┴──────────┴──────────┴──────┴──────┴──────┴──────────┴──────────┘")
}

// handleSendRawHex 发送自定义HEX数据
func handleSendRawHex(dev *device.Device) {
	input, err := PromptInputValidate("HEX数据 (空格分隔)", "AA 01 00 06 05 55", func(input string) error {
		parts := strings.Fields(input)
		if len(parts) == 0 {
			return fmt.Errorf("数据不能为空")
		}
		for _, p := range parts {
			if _, err := strconv.ParseUint(p, 16, 8); err != nil {
				return fmt.Errorf("无效HEX: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("输入错误: %v\n", err)
		return
	}

	parts := strings.Fields(input)
	data := make([]byte, len(parts))
	for i, p := range parts {
		v, _ := strconv.ParseUint(p, 16, 8)
		data[i] = byte(v)
	}

	fmt.Println("\n[发送自定义数据]")
	resp, err := dev.SendRaw(data)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	fmt.Printf("  响应帧: ID=%d, CMD=0x%02X, Data=%s\n",
		resp.ID, resp.Command, protocol.FormatHex(resp.Data))
}

// printResult 打印操作结果
func printResult(action string, result byte) {
	if result == 0x00 {
		fmt.Printf("  结果: %s成功\n", action)
	} else {
		fmt.Printf("  结果: %s失败 (错误码: 0x%02X)\n", action, result)
	}
}
