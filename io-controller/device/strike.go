package device

import "io-controller/protocol"

const (
	// DefaultPower 默认功率设置值
	DefaultPower byte = 0x21
	// TotalModules 总模块数量
	TotalModules = 10
)

// FreqToIndexMap 频段名称到模块索引的映射
var FreqToIndexMap = map[string]int{
	"433M": 1, "915M": 2, "1.2G": 3, "1.4G": 4, "1.5G": 5,
	"1.8G": 6, "3.3G": 7, "5.2G": 8, "2.4G": 9, "5.8G": 10,
}

// FreqBands 所有支持的频段（有序列表）
var FreqBands = []string{
	"433M", "915M", "1.2G", "1.4G", "1.5G",
	"1.8G", "3.3G", "5.2G", "2.4G", "5.8G",
}

// StartStrike 开启打击：开启指定频段对应的模块，其余关闭。
// 返回结果码：0x00=成功
func (d *Device) StartStrike(bands []string) (byte, error) {
	settings := MakeAllOffSettings()
	for _, band := range bands {
		if idx, ok := FreqToIndexMap[band]; ok {
			settings[idx-1].SwitchSetting = 1
		}
	}
	return d.SetSwitchPower(settings)
}

// StopStrike 关闭打击：关闭全部模块。
// 返回结果码：0x00=成功
func (d *Device) StopStrike() (byte, error) {
	return d.SetSwitchPower(MakeAllOffSettings())
}

// MakeAllOffSettings 创建全部关闭的模块设置（10路）
func MakeAllOffSettings() []protocol.ModelSetting {
	settings := make([]protocol.ModelSetting, TotalModules)
	for i := range settings {
		settings[i] = protocol.ModelSetting{
			SwitchSetting: 0,
			PowerSetting:  DefaultPower,
		}
	}
	return settings
}
