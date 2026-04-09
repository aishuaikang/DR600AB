package ui

import (
	"fmt"
	"gpio-controller/gpio"
	"strconv"
)

// 主菜单选项
var menuItems = []menuItem{
	{Label: "输出高电平", Desc: "将当前 GPIO 的输出值设置为 1。"},
	{Label: "输出低电平", Desc: "将当前 GPIO 的输出值设置为 0。"},
	{Label: "读取当前状态", Desc: "查看当前引脚的方向和电平状态。"},
	{Label: "切换当前电平", Desc: "在高电平和低电平之间快速切换。"},
	{Label: "切换控制引脚", Desc: "释放当前 GPIO，并切换到新的 GPIO 编号。"},
	{Label: "退出程序", Desc: "清理当前 GPIO 后退出程序。"},
}

// App 应用程序主界面
type App struct {
	Pin *gpio.Pin
}

// NewApp 创建应用实例
func NewApp(pin *gpio.Pin) *App {
	return &App{Pin: pin}
}

// Run 启动主菜单循环
func (a *App) Run() {
	for {
		a.renderStatusPanel()

		idx, _, err := SelectMenuItem("选择操作  (↑↓ 切换，Enter 确认)", menuItems)
		if err != nil {
			printError("菜单选择失败: %v", err)
			return
		}
		fmt.Println()

		switch idx {
		case 0:
			a.handleSetHigh()
		case 1:
			a.handleSetLow()
		case 2:
			a.handleReadValue()
		case 3:
			a.handleToggle()
		case 4:
			a.handleSwitchPin()
		case 5:
			printInfo("退出程序")
			return
		}
	}
}

func (a *App) handleSetHigh() {
	if err := a.Pin.SetHigh(); err != nil {
		printError("设置高电平失败: %v", err)
		return
	}
	printSuccess("GPIO%d -> 高电平", a.Pin.Number)
}

func (a *App) handleSetLow() {
	if err := a.Pin.SetLow(); err != nil {
		printError("设置低电平失败: %v", err)
		return
	}
	printSuccess("GPIO%d -> 低电平", a.Pin.Number)
}

func (a *App) handleReadValue() {
	status := a.readPinStatus()
	printInfo("GPIO%d 当前状态", a.Pin.Number)
	fmt.Printf("方向 : %s\n", status.Direction)
	fmt.Printf("电平 : %s\n", status.Level)
}

func (a *App) handleToggle() {
	val, err := a.Pin.GetValue()
	if err != nil {
		printError("读取当前电平失败: %v", err)
		return
	}
	newVal := 1 - val
	if err := a.Pin.SetValue(newVal); err != nil {
		printError("切换电平失败: %v", err)
		return
	}
	printSuccess("GPIO%d -> %s", a.Pin.Number, formatLevel(newVal))
}

func (a *App) handleSwitchPin() {
	pinNum, err := PromptGPIOPin(strconv.Itoa(a.Pin.Number))
	if err != nil {
		printError("输入错误: %v", err)
		return
	}

	if pinNum == a.Pin.Number {
		printInfo("当前已经在控制 GPIO%d", pinNum)
		return
	}

	// 先初始化新引脚，成功后再清理旧引脚
	newPin := gpio.NewPin(pinNum)
	if err := newPin.Setup(); err != nil {
		printError("初始化 GPIO%d 失败: %v", pinNum, err)
		return
	}

	a.Pin.Cleanup()
	a.Pin = newPin
	printSuccess("已切换到 GPIO%d", pinNum)
}

type pinStatus struct {
	Direction string
	Level     string
}

func (a *App) renderStatusPanel() {
	status := a.readPinStatus()

	fmt.Println()
	fmt.Println("──────────────── GPIO 控制台 ────────────────")
	fmt.Printf("当前引脚 : GPIO%d\n", a.Pin.Number)
	fmt.Printf("方向     : %s\n", status.Direction)
	fmt.Printf("电平     : %s\n", status.Level)
	fmt.Println("────────────────────────────────────────────")
}

func (a *App) readPinStatus() pinStatus {
	direction := "未知"
	if dir, err := a.Pin.GetDirection(); err == nil && dir != "" {
		direction = formatDirection(dir)
	}

	level := "未知"
	if val, err := a.Pin.GetValue(); err == nil {
		level = formatLevelWithValue(val)
	}

	return pinStatus{
		Direction: direction,
		Level:     level,
	}
}

func formatDirection(dir string) string {
	switch dir {
	case "out":
		return "输出"
	case "in":
		return "输入"
	default:
		return dir
	}
}

func formatLevel(value int) string {
	switch value {
	case 1:
		return "高电平"
	case 0:
		return "低电平"
	default:
		return fmt.Sprintf("异常值(%d)", value)
	}
}

func formatLevelWithValue(value int) string {
	switch value {
	case 1:
		return "高电平 (1)"
	case 0:
		return "低电平 (0)"
	default:
		return fmt.Sprintf("异常值 (%d)", value)
	}
}

func printSuccess(format string, args ...any) {
	fmt.Printf("[成功] "+format+"\n", args...)
}

func printError(format string, args ...any) {
	fmt.Printf("[错误] "+format+"\n", args...)
}

func printInfo(format string, args ...any) {
	fmt.Printf("[提示] "+format+"\n", args...)
}
