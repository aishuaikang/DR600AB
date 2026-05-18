// Package board 定义当前板卡的 GPIO 引脚映射。
package board

import "fmt"

// PinDefinition 描述板卡对外暴露的一路 GPIO。
type PinDefinition struct {
	ID       string
	Label    string
	Number   int
	Bands    []string
	Reserved bool
}

// DefaultPins 返回板卡的 8 路 GPIO 映射。前三路用于干扰控制，后五路预留。
func DefaultPins() []PinDefinition {
	return []PinDefinition{
		{ID: "io1", Label: "IOC4", Number: 20, Bands: []string{"433", "800", "900", "1.4"}},
		{ID: "io2", Label: "IOC2", Number: 18, Bands: []string{"1.2", "1.5"}},
		{ID: "io3", Label: "IOC3", Number: 19, Bands: []string{"2.4", "5.2", "5.8"}},
		{ID: "io4", Label: "IOC5", Number: 21, Reserved: true},
		{ID: "io5", Label: "I3B4", Number: 108, Reserved: true},
		{ID: "io6", Label: "I3B5", Number: 109, Reserved: true},
		{ID: "io7", Label: "I3C0", Number: 112, Reserved: true},
		{ID: "io8", Label: "I3C1", Number: 113, Reserved: true},
	}
}

// FormatPinUsage 返回适合 CLI 展示的一行 GPIO 映射说明。
func FormatPinUsage(def PinDefinition) string {
	if def.Reserved {
		return fmt.Sprintf("%s: GPIO%d 预留", def.Label, def.Number)
	}
	return fmt.Sprintf("%s: GPIO%d (%s)", def.Label, def.Number, joinBands(def.Bands))
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
