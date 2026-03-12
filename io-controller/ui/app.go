package ui

import (
	"fmt"
	"io-controller/device"
)

// 主菜单选项
var menuItems = []string{
	"开启打击",
	"关闭打击",
	"设置N路开关      (0x04)",
	"查询N路模块信息  (0x05)",
	"发送自定义HEX数据",
	"退出",
}

// App 应用程序主界面
type App struct {
	Dev *device.Device
}

// NewApp 创建应用实例
func NewApp(dev *device.Device) *App {
	return &App{Dev: dev}
}

// Run 启动主菜单循环
func (a *App) Run() {
	for {
		idx, _, err := SelectFromList("请选择命令", menuItems)
		if err != nil {
			fmt.Printf("选择错误: %v\n", err)
			return
		}

		switch idx {
		case 0:
			handleStartStrike(a.Dev)
		case 1:
			handleStopStrike(a.Dev)
		case 2:
			handleSetSwitchPower(a.Dev)
		case 3:
			handleQueryModuleInfo(a.Dev)
		case 4:
			handleSendRawHex(a.Dev)
		case 5:
			fmt.Println("退出程序")
			return
		}
	}
}
