package ui

import (
	"fmt"
	"strconv"

	"github.com/manifoldco/promptui"
)

// SelectFromList 使用 promptui Select 从列表中选择一项
func SelectFromList(label string, items []string) (int, string, error) {
	prompt := promptui.Select{
		Label: label,
		Items: items,
		Size:  10,
	}
	return prompt.Run()
}

// PromptGPIOPin 输入 GPIO 引脚编号
func PromptGPIOPin(defaultPin string) (int, error) {
	prompt := promptui.Prompt{
		Label:   "GPIO引脚编号",
		Default: defaultPin,
		Validate: func(input string) error {
			v, err := strconv.Atoi(input)
			if err != nil {
				return fmt.Errorf("请输入数字")
			}
			if v < 0 || v > 1023 {
				return fmt.Errorf("引脚编号范围 0-1023")
			}
			return nil
		},
	}
	s, err := prompt.Run()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}
