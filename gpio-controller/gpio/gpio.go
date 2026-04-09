package gpio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	sysfsPath = "/sys/class/gpio"

	controlFilePerm   = 0o200
	attributeFilePerm = 0o644

	directionIn  = "in"
	directionOut = "out"

	valueLow  = 0
	valueHigh = 1
)

var (
	sysfsRoot = sysfsPath

	exportReadyRetries = 50
	exportReadyDelay   = 10 * time.Millisecond
	writeRetryCount    = 5
	writeRetryDelay    = 20 * time.Millisecond

	statFile  = os.Stat
	readFile  = os.ReadFile
	writeFile = os.WriteFile
	readDir   = os.ReadDir
	sleep     = time.Sleep
)

// Pin 代表一个 GPIO 引脚
type Pin struct {
	Number int
	path   string // /sys/class/gpio/gpio<N>
}

// NewPin 创建 GPIO 引脚实例
func NewPin(number int) *Pin {
	return &Pin{
		Number: number,
		path:   filepath.Join(sysfsRoot, fmt.Sprintf("gpio%d", number)),
	}
}

// Export 导出引脚，使其可通过 sysfs 访问
func (p *Pin) Export() error {
	if p.IsExported() {
		return p.busyError()
	}
	if err := writeControlFile("export", p.Number); err != nil {
		if isBusyError(err) {
			return p.busyError()
		}
		return fmt.Errorf("导出 GPIO%d 失败: %w", p.Number, err)
	}

	if err := p.waitForAttribute("value", exportReadyRetries, exportReadyDelay); err != nil {
		return fmt.Errorf("导出 GPIO%d 后等待就绪失败: %w", p.Number, err)
	}
	return nil
}

// Unexport 取消导出引脚
func (p *Pin) Unexport() error {
	if !p.IsExported() {
		return nil
	}
	if err := writeControlFile("unexport", p.Number); err != nil {
		return fmt.Errorf("取消导出 GPIO%d 失败: %w", p.Number, err)
	}
	return nil
}

// IsExported 检查引脚是否已导出
func (p *Pin) IsExported() bool {
	_, err := statFile(p.path)
	return err == nil
}

// SetDirection 设置引脚方向: "in" 或 "out"
func (p *Pin) SetDirection(dir string) error {
	if dir != directionIn && dir != directionOut {
		return fmt.Errorf("无效方向: %s，仅支持 in/out", dir)
	}
	return p.writeAttributeWithRetry("direction", dir, writeRetryCount, writeRetryDelay)
}

// GetDirection 获取当前引脚方向
func (p *Pin) GetDirection() (string, error) {
	return p.readAttribute("direction")
}

// SetHigh 设置高电平
func (p *Pin) SetHigh() error {
	return p.SetValue(valueHigh)
}

// SetLow 设置低电平
func (p *Pin) SetLow() error {
	return p.SetValue(valueLow)
}

// SetValue 设置引脚值: 0（低电平）或 1（高电平）
func (p *Pin) SetValue(value int) error {
	if value != valueLow && value != valueHigh {
		return fmt.Errorf("无效电平值: %d，仅支持 0/1", value)
	}
	return p.writeAttribute("value", strconv.Itoa(value))
}

// GetValue 读取引脚当前电平值
func (p *Pin) GetValue() (int, error) {
	data, err := p.readAttribute("value")
	if err != nil {
		return 0, err
	}
	value, convErr := strconv.Atoi(data)
	if convErr != nil {
		return 0, fmt.Errorf("解析 GPIO%d/value 失败: %w", p.Number, convErr)
	}
	return value, nil
}

// Setup 导出引脚并设置为输出模式
func (p *Pin) Setup() error {
	if err := p.Export(); err != nil {
		return err
	}
	if err := p.SetDirection(directionOut); err != nil {
		return fmt.Errorf("设置 GPIO%d 为输出模式失败: %w", p.Number, err)
	}
	return nil
}

// Cleanup 将引脚设为低电平并取消导出
func (p *Pin) Cleanup() {
	if !p.IsExported() {
		return
	}
	_ = p.SetLow()
	_ = p.Unexport()
}

// ListExportedPins 列出当前已导出的 GPIO 引脚编号
func ListExportedPins() []int {
	entries, err := readDir(sysfsRoot)
	if err != nil {
		return nil
	}
	pins := make([]int, 0, len(entries))
	for _, e := range entries {
		num, ok := parseExportedPinNumber(e.Name())
		if !ok {
			continue
		}
		pins = append(pins, num)
	}
	sort.Ints(pins)
	return pins
}

// GPIOChipInfo GPIO 控制器芯片信息
type GPIOChipInfo struct {
	Name  string // 芯片名称，如 gpiochip0
	Label string // 芯片标签
	Base  int    // 起始引脚编号
	Ngpio int    // 引脚数量
}

// ListGPIOChips 列出系统中的 GPIO 控制器芯片及其引脚范围
func ListGPIOChips() []GPIOChipInfo {
	entries, err := readDir(sysfsRoot)
	if err != nil {
		return nil
	}
	chips := make([]GPIOChipInfo, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "gpiochip") {
			continue
		}
		chipPath := filepath.Join(sysfsRoot, name)
		chips = append(chips, GPIOChipInfo{
			Name:  name,
			Label: readOptionalTrimmedFile(filepath.Join(chipPath, "label")),
			Base:  readOptionalIntFile(filepath.Join(chipPath, "base")),
			Ngpio: readOptionalIntFile(filepath.Join(chipPath, "ngpio")),
		})
	}
	sort.Slice(chips, func(i, j int) bool { return chips[i].Base < chips[j].Base })
	return chips
}

// String 返回引脚描述
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
	return fmt.Sprintf("GPIO%d (方向=%s, 电平=%s)", p.Number, dir, level)
}

func (p *Pin) attributePath(name string) string {
	return filepath.Join(p.path, name)
}

func (p *Pin) busyError() error {
	return fmt.Errorf("导出 GPIO%d 失败: 引脚已被其他进程导出或被内核占用", p.Number)
}

func (p *Pin) readAttribute(name string) (string, error) {
	data, err := readFile(p.attributePath(name))
	if err != nil {
		return "", fmt.Errorf("读取 GPIO%d/%s 失败: %w", p.Number, name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (p *Pin) writeAttribute(name, value string) error {
	if err := writeFile(p.attributePath(name), []byte(value), attributeFilePerm); err != nil {
		return fmt.Errorf("写入 GPIO%d/%s 失败: %w", p.Number, name, err)
	}
	return nil
}

func (p *Pin) writeAttributeWithRetry(name, value string, retries int, delay time.Duration) error {
	var lastErr error
	path := p.attributePath(name)
	for i := 0; i < retries; i++ {
		lastErr = writeFile(path, []byte(value), attributeFilePerm)
		if lastErr == nil {
			return nil
		}
		if i < retries-1 {
			sleep(delay)
		}
	}
	return fmt.Errorf("写入 GPIO%d/%s 失败: %w", p.Number, name, lastErr)
}

func (p *Pin) waitForAttribute(name string, retries int, delay time.Duration) error {
	path := p.attributePath(name)
	for i := 0; i < retries; i++ {
		if _, err := statFile(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("检查 GPIO%d/%s 失败: %w", p.Number, name, err)
		}

		if i < retries-1 {
			sleep(delay)
		}
	}
	return fmt.Errorf("等待 GPIO%d/%s 就绪超时", p.Number, name)
}

func writeControlFile(name string, number int) error {
	return writeFile(
		filepath.Join(sysfsRoot, name),
		[]byte(strconv.Itoa(number)),
		controlFilePerm,
	)
}

func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, syscall.EBUSY) || strings.Contains(strings.ToLower(err.Error()), "busy")
}

func parseExportedPinNumber(name string) (int, bool) {
	if !strings.HasPrefix(name, "gpio") || strings.HasPrefix(name, "gpiochip") {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(name, "gpio"))
	if err != nil {
		return 0, false
	}
	return number, true
}

func readOptionalTrimmedFile(path string) string {
	data, err := readFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readOptionalIntFile(path string) int {
	data := readOptionalTrimmedFile(path)
	if data == "" {
		return 0
	}
	value, err := strconv.Atoi(data)
	if err != nil {
		return 0
	}
	return value
}
