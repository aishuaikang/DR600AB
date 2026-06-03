// Package board 定义当前板卡的外部 IO 通道映射。
package board

import (
	"fmt"
	"sort"

	"gpio-controller/gpio"
)

var interferenceBands = [][]string{
	{"433", "800", "900", "1.4"},
	{"1.2", "1.5"},
	{"2.4", "5.2", "5.8"},
}

var interferencePinOrder = []int{2, 3, 1}
var reservedPinOrder = []int{4, 0, 5}

// PinDefinition 描述板卡对外暴露的一路外部 IO。
type PinDefinition struct {
	ID       string
	Label    string
	Number   int
	Bands    []string
	Reserved bool
}

// DefaultPins 返回当前系统 /sys/external_gpio 中实际可控制的外部 IO 映射。
func DefaultPins() []PinDefinition {
	return PinsFromNumbers(gpio.ListExternalPins())
}

// PinsFromNumbers 根据外部 IO 序号生成稳定的通道定义。IO2、IO3、IO1 为干扰通道，IO4、IO0、IO5 为预留通道。
func PinsFromNumbers(numbers []int) []PinDefinition {
	if len(numbers) == 0 {
		return nil
	}

	available := make(map[int]bool, len(numbers))
	for _, number := range numbers {
		if number >= 0 {
			available[number] = true
		}
	}

	definitions := make([]PinDefinition, 0, len(available))
	used := make(map[int]bool, len(available))
	for bandIndex, number := range interferencePinOrder {
		if !available[number] {
			continue
		}
		definitions = append(definitions, PinDefinition{
			ID:       fmt.Sprintf("io%d", bandIndex+1),
			Label:    fmt.Sprintf("IO%d", number),
			Number:   number,
			Bands:    append([]string(nil), interferenceBands[bandIndex]...),
			Reserved: false,
		})
		used[number] = true
	}

	for reservedIndex, number := range reservedPinOrder {
		if !available[number] || used[number] {
			continue
		}
		definitions = append(definitions, PinDefinition{
			ID:       fmt.Sprintf("io%d", len(interferencePinOrder)+reservedIndex+1),
			Label:    fmt.Sprintf("IO%d", number),
			Number:   number,
			Bands:    []string{},
			Reserved: true,
		})
		used[number] = true
	}

	extraReserved := make([]int, 0, len(available)-len(used))
	for number := range available {
		if !used[number] {
			extraReserved = append(extraReserved, number)
		}
	}
	sort.Ints(extraReserved)
	nextExtraID := len(interferencePinOrder) + len(reservedPinOrder) + 1
	for _, number := range extraReserved {
		definitions = append(definitions, PinDefinition{
			ID:       fmt.Sprintf("io%d", nextExtraID),
			Label:    fmt.Sprintf("IO%d", number),
			Number:   number,
			Bands:    []string{},
			Reserved: true,
		})
		nextExtraID++
	}
	return definitions
}

// FormatPinUsage 返回适合 CLI 展示的一行外部 IO 映射说明。
func FormatPinUsage(def PinDefinition) string {
	if def.Reserved {
		return fmt.Sprintf("%s: 预留", def.Label)
	}
	return fmt.Sprintf("%s: %s", def.Label, joinBands(def.Bands))
}

func joinBands(bands []string) string {
	if len(bands) == 0 {
		return "-"
	}
	result := bands[0]
	for _, band := range bands[1:] {
		result += " " + band
	}
	return result
}
