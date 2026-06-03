package gpio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	externalGPIOPath = "/sys/external_gpio"

	valueFilePrefix     = "jwsioc_gpio"
	directionFilePrefix = "jwsioc_inout_gpio"

	attributeFilePerm = 0o644

	directionIn  = "in"
	directionOut = "out"

	directionValueIn  = 0
	directionValueOut = 1

	valueLow  = 0
	valueHigh = 1
)

var (
	sysfsRoot = externalGPIOPath

	writeRetryCount = 5
	writeRetryDelay = 20 * time.Millisecond

	statFile  = os.Stat
	readFile  = os.ReadFile
	writeFile = os.WriteFile
	readDir   = os.ReadDir
	sleep     = time.Sleep
)

// Pin 代表新板 /sys/external_gpio 暴露的一路外部 IO。
type Pin struct {
	Number        int
	valuePath     string // /sys/external_gpio/jwsioc_gpio<N>
	directionPath string // /sys/external_gpio/jwsioc_inout_gpio<N>
}

// NewPin 创建外部 IO 引脚实例。number 是 jwsioc_gpio<N> 中的 N。
func NewPin(number int) *Pin {
	return &Pin{
		Number:        number,
		valuePath:     filepath.Join(sysfsRoot, fmt.Sprintf("%s%d", valueFilePrefix, number)),
		directionPath: filepath.Join(sysfsRoot, fmt.Sprintf("%s%d", directionFilePrefix, number)),
	}
}

// Export 检查外部 IO 文件是否已由内核暴露。
func (p *Pin) Export() error {
	return p.ensureAvailable()
}

// Unexport 将外部 IO 切回输入模式。
func (p *Pin) Unexport() error {
	if !p.IsExported() {
		return nil
	}
	return p.SetDirection(directionIn)
}

// IsExported 检查外部 IO 的电平文件和方向文件是否都存在。
func (p *Pin) IsExported() bool {
	return statOK(p.valuePath) && statOK(p.directionPath)
}

// SetDirection 设置引脚方向: "in" 或 "out"。
func (p *Pin) SetDirection(dir string) error {
	value, err := directionFileValue(dir)
	if err != nil {
		return err
	}
	return p.writeAttributeWithRetry("direction", strconv.Itoa(value), writeRetryCount, writeRetryDelay)
}

// GetDirection 获取当前引脚方向。
func (p *Pin) GetDirection() (string, error) {
	data, err := p.readAttribute("direction")
	if err != nil {
		return "", err
	}
	switch data {
	case strconv.Itoa(directionValueIn):
		return directionIn, nil
	case strconv.Itoa(directionValueOut):
		return directionOut, nil
	case directionIn, directionOut:
		return data, nil
	default:
		return "", fmt.Errorf("解析 IO%d 方向失败: %q", p.Number, data)
	}
}

// SetHigh 设置高电平。
func (p *Pin) SetHigh() error {
	return p.SetValue(valueHigh)
}

// SetLow 设置低电平。
func (p *Pin) SetLow() error {
	return p.SetValue(valueLow)
}

// SetValue 设置引脚值: 0（低电平）或 1（高电平）。
func (p *Pin) SetValue(value int) error {
	if value != valueLow && value != valueHigh {
		return fmt.Errorf("无效电平值: %d，仅支持 0/1", value)
	}
	return p.writeAttribute("value", strconv.Itoa(value))
}

// GetValue 读取引脚当前电平值。
func (p *Pin) GetValue() (int, error) {
	data, err := p.readAttribute("value")
	if err != nil {
		return 0, err
	}
	value, convErr := strconv.Atoi(data)
	if convErr != nil {
		return 0, fmt.Errorf("解析 IO%d 电平失败: %w", p.Number, convErr)
	}
	return value, nil
}

// Setup 检查引脚并设置为输出模式。
func (p *Pin) Setup() error {
	if err := p.Export(); err != nil {
		return err
	}
	if err := p.SetDirection(directionOut); err != nil {
		return err
	}
	return nil
}

// Cleanup 将引脚设为低电平，并保持输出模式以避免输入上拉让外设再次变为高电平。
func (p *Pin) Cleanup() {
	if !p.IsExported() {
		return
	}
	_ = p.SetLow()
}

// ListExternalPins 列出当前可控制的外部 IO 序号。
func ListExternalPins() []int {
	entries, err := readDir(sysfsRoot)
	if err != nil {
		return nil
	}

	values := make(map[int]bool, len(entries))
	directions := make(map[int]bool, len(entries))
	for _, e := range entries {
		number, kind, ok := parseExternalGPIOName(e.Name())
		if !ok {
			continue
		}
		switch kind {
		case "value":
			values[number] = true
		case "direction":
			directions[number] = true
		}
	}

	pins := make([]int, 0, len(values))
	for number := range values {
		if directions[number] {
			pins = append(pins, number)
		}
	}
	sort.Ints(pins)
	return pins
}

// ListExportedPins 列出当前可控制的外部 IO 序号。
//
// Deprecated: use ListExternalPins.
func ListExportedPins() []int {
	return ListExternalPins()
}

// String 返回引脚描述。
func (p *Pin) String() string {
	dir := "未知"
	if currentDir, err := p.GetDirection(); err == nil && currentDir != "" {
		dir = currentDir
	}

	level := "未知"
	if val, err := p.GetValue(); err == nil {
		switch val {
		case valueLow:
			level = "低"
		case valueHigh:
			level = "高"
		default:
			level = strconv.Itoa(val)
		}
	}
	return fmt.Sprintf("IO%d (方向=%s, 电平=%s)", p.Number, dir, level)
}

func (p *Pin) ensureAvailable() error {
	if _, err := statFile(p.valuePath); err != nil {
		return fmt.Errorf("外部 IO%d 电平文件不可用: %w", p.Number, err)
	}
	if _, err := statFile(p.directionPath); err != nil {
		return fmt.Errorf("外部 IO%d 方向文件不可用: %w", p.Number, err)
	}
	return nil
}

func (p *Pin) attributePath(name string) (string, error) {
	switch name {
	case "value":
		return p.valuePath, nil
	case "direction":
		return p.directionPath, nil
	default:
		return "", fmt.Errorf("未知 IO%d 属性: %s", p.Number, name)
	}
}

func (p *Pin) readAttribute(name string) (string, error) {
	path, err := p.attributePath(name)
	if err != nil {
		return "", err
	}
	data, err := readFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (p *Pin) writeAttribute(name, value string) error {
	path, err := p.attributePath(name)
	if err != nil {
		return err
	}
	if err := writeFile(path, []byte(value), attributeFilePerm); err != nil {
		return err
	}
	return nil
}

func (p *Pin) writeAttributeWithRetry(name, value string, retries int, delay time.Duration) error {
	path, err := p.attributePath(name)
	if err != nil {
		return err
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		lastErr = writeFile(path, []byte(value), attributeFilePerm)
		if lastErr == nil {
			return nil
		}
		if i < retries-1 {
			sleep(delay)
		}
	}
	return lastErr
}

func directionFileValue(dir string) (int, error) {
	switch dir {
	case directionIn:
		return directionValueIn, nil
	case directionOut:
		return directionValueOut, nil
	default:
		return 0, fmt.Errorf("无效方向: %s，仅支持 in/out", dir)
	}
}

func statOK(path string) bool {
	_, err := statFile(path)
	return err == nil
}

func parseExternalGPIOName(name string) (int, string, bool) {
	if strings.HasPrefix(name, directionFilePrefix) {
		number, ok := parseExternalNumber(strings.TrimPrefix(name, directionFilePrefix))
		return number, "direction", ok
	}
	if strings.HasPrefix(name, valueFilePrefix) {
		number, ok := parseExternalNumber(strings.TrimPrefix(name, valueFilePrefix))
		return number, "value", ok
	}
	return 0, "", false
}

func parseExternalNumber(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	number, err := strconv.Atoi(raw)
	if err != nil || number < 0 {
		return 0, false
	}
	return number, true
}
