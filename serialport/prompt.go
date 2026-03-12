package serialport

import (
	"fmt"

	"github.com/manifoldco/promptui"
)

// SelectPort 交互式选择串口。
// 列出可用串口供用户选择，同时提供手动输入选项；
// 若系统无可用串口，直接引导用户手动输入。
func SelectPort() (string, error) {
	ports, err := ListPorts()
	if err != nil {
		return "", fmt.Errorf("获取串口列表失败: %w", err)
	}

	if len(ports) == 0 {
		fmt.Println("未检测到可用串口")
		return promptInput("请手动输入串口名称", "/dev/tty.usbserial-110")
	}

	// 追加手动输入选项
	items := make([]string, len(ports)+1)
	copy(items, ports)
	items[len(ports)] = "[ 手动输入串口名称 ]"

	prompt := promptui.Select{
		Label: "选择串口",
		Items: items,
		Size:  10,
	}

	_, result, err := prompt.Run()
	if err != nil {
		return "", fmt.Errorf("串口选择已取消: %w", err)
	}

	if result == "[ 手动输入串口名称 ]" {
		return promptInput("请输入串口名称", "/dev/tty.usbserial-110")
	}
	return result, nil
}

// promptInput 文本输入提示（内部使用）
func promptInput(label, defaultVal string) (string, error) {
	prompt := promptui.Prompt{
		Label:   label,
		Default: defaultVal,
	}
	return prompt.Run()
}
