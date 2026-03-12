package ui

import (
	"fmt"
	"gpio-controller/gpio"
)

// 主菜单选项
var menuItems = []string{
	"设置高电平",
	"设置低电平",
	"读取引脚状态",
	"切换电平 (Toggle)",
	"查看可用引脚",
	"切换引脚",
	"退出",
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
		fmt.Printf("\n当前引脚: %s\n", a.Pin)

		idx, _, err := SelectFromList("请选择操作", menuItems)
		if err != nil {
			fmt.Printf("选择错误: %v\n", err)
			return
		}

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
			a.handleListPins()
		case 5:
			a.handleSwitchPin()
		case 6:
			fmt.Println("退出程序")
			return
		}
	}
}

func (a *App) handleSetHigh() {
	if err := a.Pin.SetHigh(); err != nil {
		fmt.Printf("设置高电平失败: %v\n", err)
		return
	}
	fmt.Printf("GPIO%d -> 高电平\n", a.Pin.Number)
}

func (a *App) handleSetLow() {
	if err := a.Pin.SetLow(); err != nil {
		fmt.Printf("设置低电平失败: %v\n", err)
		return
	}
	fmt.Printf("GPIO%d -> 低电平\n", a.Pin.Number)
}

func (a *App) handleReadValue() {
	val, err := a.Pin.GetValue()
	if err != nil {
		fmt.Printf("读取失败: %v\n", err)
		return
	}
	level := "低电平"
	if val == 1 {
		level = "高电平"
	}
	dir, _ := a.Pin.GetDirection()
	fmt.Printf("GPIO%d: 方向=%s, 电平=%s (%d)\n", a.Pin.Number, dir, level, val)
}

func (a *App) handleToggle() {
	val, err := a.Pin.GetValue()
	if err != nil {
		fmt.Printf("读取当前电平失败: %v\n", err)
		return
	}
	newVal := 1 - val
	if err := a.Pin.SetValue(newVal); err != nil {
		fmt.Printf("切换电平失败: %v\n", err)
		return
	}
	level := "低电平"
	if newVal == 1 {
		level = "高电平"
	}
	fmt.Printf("GPIO%d -> %s\n", a.Pin.Number, level)
}

func (a *App) handleListPins() {
	// 显示 GPIO 控制器芯片信息
	chips := gpio.ListGPIOChips()
	if len(chips) > 0 {
		fmt.Println("\nGPIO 控制器:")
		fmt.Println("  ┌──────────────┬────────────────────┬──────┬──────┬──────────────┐")
		fmt.Println("  │     芯片     │        标签        │ 起始 │ 数量 │   引脚范围   │")
		fmt.Println("  ├──────────────┼────────────────────┼──────┼──────┼──────────────┤")
		for _, c := range chips {
			fmt.Printf("  │ %-12s │ %-18s │ %4d │ %4d │ %4d - %-5d │\n",
				c.Name, c.Label, c.Base, c.Ngpio, c.Base, c.Base+c.Ngpio-1)
		}
		fmt.Println("  └──────────────┴────────────────────┴──────┴──────┴──────────────┘")
	} else {
		fmt.Println("\n未发现 GPIO 控制器（可能不在 Linux 环境或无 sysfs 支持）")
	}

	// 显示已导出的引脚
	exported := gpio.ListExportedPins()
	if len(exported) > 0 {
		fmt.Printf("\n已导出的引脚 (%d 个):\n", len(exported))
		for _, num := range exported {
			pin := gpio.NewPin(num)
			fmt.Printf("  - %s\n", pin)
		}
	} else {
		fmt.Println("\n当前无已导出的引脚")
	}
}

func (a *App) handleSwitchPin() {
	pinNum, err := PromptGPIOPin("")
	if err != nil {
		fmt.Printf("输入错误: %v\n", err)
		return
	}

	// 先初始化新引脚，成功后再清理旧引脚
	newPin := gpio.NewPin(pinNum)
	if err := newPin.Setup(); err != nil {
		fmt.Printf("初始化 GPIO%d 失败: %v\n", pinNum, err)
		return
	}

	a.Pin.Cleanup()
	a.Pin = newPin
	fmt.Printf("已切换到 GPIO%d\n", pinNum)
}
