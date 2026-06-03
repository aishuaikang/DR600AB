package ui

import (
	"fmt"
	"strconv"

	"github.com/manifoldco/promptui"
)

type menuItem struct {
	Label string
	Desc  string
}

var menuSelectTemplates = &promptui.SelectTemplates{
	Label:    "{{ . }}",
	Active:   "▸ {{ .Label | cyan }}",
	Inactive: "  {{ .Label }}",
	Selected: "已选择: {{ .Label | green }}",
	Details: `
──────── 操作说明 ────────
{{ .Desc | faint }}`,
}

var gpioPinPromptTemplates = &promptui.PromptTemplates{
	Prompt:  "{{ . | cyan }} ",
	Valid:   "{{ . | green }} ",
	Invalid: "{{ . | red }} ",
	Success: "{{ . | bold }} ",
}

// SelectMenuItem 使用 promptui Select 从菜单中选择一项
func SelectMenuItem(label string, items []menuItem) (int, menuItem, error) {
	prompt := promptui.Select{
		Label:     label,
		Items:     items,
		Size:      len(items),
		Templates: menuSelectTemplates,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return 0, menuItem{}, err
	}
	return idx, items[idx], nil
}

// PromptGPIOPin 输入外部 IO 序号。
func PromptGPIOPin(defaultPin string) (int, error) {
	prompt := promptui.Prompt{
		Label:     "输入外部 IO 序号",
		Default:   defaultPin,
		Templates: gpioPinPromptTemplates,
		Validate: func(input string) error {
			v, err := strconv.Atoi(input)
			if err != nil {
				return fmt.Errorf("请输入数字")
			}
			if v < 0 || v > 1023 {
				return fmt.Errorf("外部 IO 序号范围 0-1023")
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
