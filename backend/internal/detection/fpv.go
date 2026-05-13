package detection

import "strings"

// fpvBand 定义用于识别模拟图传信号的频段。
type fpvBand struct {
	key      string
	label    string
	minMHz   float64
	maxMHz   float64
	keywords []string
}

// fpvBands 按频率范围和型号关键字列出已知图传频段。
var fpvBands = []fpvBand{
	{key: "433", label: "433M", minMHz: 400, maxMHz: 470, keywords: []string{"433"}},
	{key: "800", label: "800M", minMHz: 780, maxMHz: 860, keywords: []string{"800"}},
	{key: "900", label: "900M", minMHz: 860, maxMHz: 960, keywords: []string{"900"}},
	{key: "1.2", label: "1.2G", minMHz: 1160, maxMHz: 1300, keywords: []string{"1.2"}},
	{key: "1.4", label: "1.4G", minMHz: 1360, maxMHz: 1450, keywords: []string{"1.4"}},
	{key: "1.5", label: "1.5G", minMHz: 1450, maxMHz: 1530, keywords: []string{"1.5"}},
	{key: "2.4", label: "2.4G", minMHz: 2300, maxMHz: 2500, keywords: []string{"2.4", "2400"}},
	{key: "5.2", label: "5.2G", minMHz: 5100, maxMHz: 5350, keywords: []string{"5.2", "5200"}},
	{key: "5.8", label: "5.8G", minMHz: 5700, maxMHz: 5900, keywords: []string{"5.8", "5800"}},
}

// classifyFPV 根据频率或描述文本匹配已知图传频段。
func classifyFPV(freq float64, values ...string) (string, string, bool) {
	for _, band := range fpvBands {
		if freq >= band.minMHz && freq <= band.maxMHz {
			return band.key, band.label, true
		}
	}

	combined := strings.ToLower(strings.Join(values, " "))
	for _, band := range fpvBands {
		for _, keyword := range band.keywords {
			if strings.Contains(combined, keyword) {
				return band.key, band.label, true
			}
		}
	}
	return "", "", false
}
