package serialport

import (
	"fmt"

	"github.com/manifoldco/promptui"
)

const manualPortItem = "[ 手动输入串口名称 ]"

// SelectPort 交互式选择串口。
// 列出可用串口供用户选择，同时提供手动输入选项；
// 若系统无可用串口，直接引导用户手动输入。
func SelectPort() (string, error) {
	return SelectPortWithLabel("选择串口")
}

// SelectPortWithLabel 使用指定标签交互式选择串口。
func SelectPortWithLabel(label string) (string, error) {
	if label == "" {
		label = "选择串口"
	}

	ports, err := ListPorts()
	if err != nil {
		return "", fmt.Errorf("获取串口列表失败: %w", err)
	}

	if len(ports) == 0 {
		fmt.Println("未检测到可用串口")
		return promptInput(label+"（手动输入）", "/dev/tty.usbserial-110")
	}

	// 追加手动输入选项
	items := make([]string, len(ports)+1)
	copy(items, ports)
	items[len(ports)] = manualPortItem

	prompt := promptui.Select{
		Label: label,
		Items: items,
		Size:  10,
	}

	_, result, err := prompt.Run()
	if err != nil {
		return "", fmt.Errorf("串口选择已取消: %w", err)
	}

	if result == manualPortItem {
		return promptInput(label+"（手动输入）", "/dev/tty.usbserial-110")
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
