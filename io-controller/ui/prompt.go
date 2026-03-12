package ui

import (
	"fmt"
	"io-controller/device"

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

// PromptInput 使用 promptui Prompt 输入文本
func PromptInput(label string, defaultVal string) (string, error) {
	prompt := promptui.Prompt{
		Label:   label,
		Default: defaultVal,
	}
	return prompt.Run()
}

// PromptInputValidate 带验证函数的文本输入
func PromptInputValidate(label string, defaultVal string, validate promptui.ValidateFunc) (string, error) {
	prompt := promptui.Prompt{
		Label:    label,
		Default:  defaultVal,
		Validate: validate,
	}
	return prompt.Run()
}

// MultiSelectFreqBands 频段多选（toggle 模式）
// 返回用户选中的频段列表（有序）
func MultiSelectFreqBands() ([]string, error) {
	selected := make(map[string]bool)

	for {
		// 构建带勾选状态的选项列表
		items := make([]string, 0, len(device.FreqBands)+1)
		for _, band := range device.FreqBands {
			mark := "  "
			if selected[band] {
				mark = "\u2713 "
			}
			items = append(items, fmt.Sprintf("%s%s (模块%d)", mark, band, device.FreqToIndexMap[band]))
		}
		items = append(items, ">>> 确认选择 <<<")

		prompt := promptui.Select{
			Label: fmt.Sprintf("选择频段 (已选 %d 个，选择切换勾选)", len(selected)),
			Items: items,
			Size:  12,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			return nil, err
		}

		// 确认选择
		if idx == len(device.FreqBands) {
			break
		}

		// 切换勾选
		band := device.FreqBands[idx]
		if selected[band] {
			delete(selected, band)
		} else {
			selected[band] = true
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("未选择任何频段")
	}

	// 按 FreqBands 顺序返回
	var result []string
	for _, band := range device.FreqBands {
		if selected[band] {
			result = append(result, band)
		}
	}
	return result, nil
}
