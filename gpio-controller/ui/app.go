package ui

import (
	"fmt"
	"gpio-controller/gpio"
	"strconv"
	"sync"
)

// 主菜单选项
var menuItems = []menuItem{
	{Label: "输出高电平", Desc: "将当前 GPIO 的输出值设置为 1。"},
	{Label: "输出低电平", Desc: "将当前 GPIO 的输出值设置为 0。"},
	{Label: "读取当前状态", Desc: "查看当前引脚的方向和电平状态。"},
	{Label: "切换当前电平", Desc: "在高电平和低电平之间快速切换。"},
	{Label: "切换控制引脚", Desc: "释放当前外部 IO，并切换到新的外部 IO 序号。"},
	{Label: "退出程序", Desc: "清理当前 GPIO 后退出程序。"},
}

// App 应用程序主界面
type App struct {
	mu  sync.RWMutex
	pin *gpio.Pin
}

// NewApp 创建应用实例
func NewApp(pin *gpio.Pin) *App {
	return &App{pin: pin}
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
	var pinNumber int
	if err := a.withCurrentPin(func(pin *gpio.Pin) error {
		pinNumber = pin.Number
		return pin.SetHigh()
	}); err != nil {
		printError("设置高电平失败: %v", err)
		return
	}
	printSuccess("IO%d -> 高电平", pinNumber)
}

func (a *App) handleSetLow() {
	var pinNumber int
	if err := a.withCurrentPin(func(pin *gpio.Pin) error {
		pinNumber = pin.Number
		return pin.SetLow()
	}); err != nil {
		printError("设置低电平失败: %v", err)
		return
	}
	printSuccess("IO%d -> 低电平", pinNumber)
}

func (a *App) handleReadValue() {
	status := a.readPinStatus()
	if status.ReadErr != nil {
		printError("读取 IO%d 状态失败: %v", status.Number, status.ReadErr)
		return
	}
	printInfo("IO%d 当前状态", status.Number)
	fmt.Printf("方向 : %s\n", status.Direction)
	fmt.Printf("电平 : %s\n", status.Level)
}

func (a *App) handleToggle() {
	var pinNumber int
	var newVal int
	if err := a.withCurrentPin(func(pin *gpio.Pin) error {
		pinNumber = pin.Number
		val, err := pin.GetValue()
		if err != nil {
			return fmt.Errorf("读取当前电平失败: %w", err)
		}
		newVal = 1 - val
		if err := pin.SetValue(newVal); err != nil {
			return err
		}
		return nil
	}); err != nil {
		printError("切换电平失败: %v", err)
		return
	}
	printSuccess("IO%d -> %s", pinNumber, formatLevel(newVal))
}

func (a *App) handleSwitchPin() {
	currentPinNumber, err := a.currentPinNumber()
	if err != nil {
		printError("读取当前引脚失败: %v", err)
		return
	}

	pinNum, err := PromptGPIOPin(strconv.Itoa(currentPinNumber))
	if err != nil {
		printError("输入错误: %v", err)
		return
	}

	if pinNum == currentPinNumber {
		printInfo("当前已经在控制 IO%d", pinNum)
		return
	}

	// 先初始化新引脚，成功后再清理旧引脚
	newPin := gpio.NewPin(pinNum)
	if err := newPin.Setup(); err != nil {
		printError("初始化 IO%d 失败: %v", pinNum, err)
		return
	}

	a.swapPin(newPin)
	printSuccess("已切换到 IO%d", pinNum)
}

type pinStatus struct {
	Number    int
	Direction string
	Level     string
	ReadErr   error
}

func (a *App) renderStatusPanel() {
	status := a.readPinStatus()

	fmt.Println()
	fmt.Println("──────────────── GPIO 控制台 ────────────────")
	fmt.Printf("当前 IO  : IO%d\n", status.Number)
	fmt.Printf("方向     : %s\n", status.Direction)
	fmt.Printf("电平     : %s\n", status.Level)
	if status.ReadErr != nil {
		fmt.Printf("状态异常 : %v\n", status.ReadErr)
	}
	fmt.Println("────────────────────────────────────────────")
}

func (a *App) readPinStatus() pinStatus {
	status, err := a.withCurrentPinStatus()
	if err != nil {
		return pinStatus{
			Direction: "未知",
			Level:     "未知",
			ReadErr:   err,
		}
	}
	return status
}

func (a *App) withCurrentPinStatus() (pinStatus, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.pin == nil {
		return pinStatus{}, fmt.Errorf("当前没有可用的外部 IO")
	}

	status := pinStatus{
		Number:    a.pin.Number,
		Direction: "未知",
		Level:     "未知",
	}

	dir, dirErr := a.pin.GetDirection()
	if dirErr == nil && dir != "" {
		status.Direction = formatDirection(dir)
	} else if dirErr != nil {
		status.ReadErr = dirErr
	}

	val, valErr := a.pin.GetValue()
	if valErr == nil {
		status.Level = formatLevelWithValue(val)
	} else if status.ReadErr == nil {
		status.ReadErr = valErr
	}

	return status, nil
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

func (a *App) currentPinNumber() (int, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.pin == nil {
		return 0, fmt.Errorf("当前没有可用的外部 IO")
	}
	return a.pin.Number, nil
}

func (a *App) withCurrentPin(fn func(*gpio.Pin) error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.pin == nil {
		return fmt.Errorf("当前没有可用的外部 IO")
	}
	return fn(a.pin)
}

func (a *App) swapPin(next *gpio.Pin) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pin != nil {
		a.pin.Cleanup()
	}
	a.pin = next
}

func (a *App) Cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pin != nil {
		a.pin.Cleanup()
	}
}
