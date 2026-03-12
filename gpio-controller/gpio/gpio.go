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

const sysfsPath = "/sys/class/gpio"

// Pin 代表一个 GPIO 引脚
type Pin struct {
	Number int
	path   string // /sys/class/gpio/gpio<N>
}

// NewPin 创建 GPIO 引脚实例
func NewPin(number int) *Pin {
	return &Pin{
		Number: number,
		path:   filepath.Join(sysfsPath, fmt.Sprintf("gpio%d", number)),
	}
}

// Export 导出引脚，使其可通过 sysfs 访问
func (p *Pin) Export() error {
	if p.IsExported() {
		return nil
	}
	err := os.WriteFile(
		filepath.Join(sysfsPath, "export"),
		[]byte(strconv.Itoa(p.Number)),
		0o200,
	)
	if err != nil {
		if strings.Contains(err.Error(), "busy") {
			return fmt.Errorf("GPIO%d 已被内核其他驱动占用，请换一个引脚", p.Number)
		}
		return err
	}
	// 等待 sysfs 文件就绪
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(filepath.Join(p.path, "value")); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// Unexport 取消导出引脚
func (p *Pin) Unexport() error {
	if !p.IsExported() {
		return nil
	}
	return os.WriteFile(
		filepath.Join(sysfsPath, "unexport"),
		[]byte(strconv.Itoa(p.Number)),
		0o200,
	)
}

// IsExported 检查引脚是否已导出
func (p *Pin) IsExported() bool {
	_, err := os.Stat(p.path)
	return err == nil
}

// SetDirection 设置引脚方向: "in" 或 "out"
func (p *Pin) SetDirection(dir string) error {
	if dir != "in" && dir != "out" {
		return fmt.Errorf("无效方向: %s，仅支持 in/out", dir)
	}
	// 重试写入，防止 sysfs 未就绪
	var lastErr error
	for i := 0; i < 5; i++ {
		lastErr = os.WriteFile(
			filepath.Join(p.path, "direction"),
			[]byte(dir),
			0o644,
		)
		if lastErr == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lastErr
}

// GetDirection 获取当前引脚方向
func (p *Pin) GetDirection() (string, error) {
	data, err := os.ReadFile(filepath.Join(p.path, "direction"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SetHigh 设置高电平
func (p *Pin) SetHigh() error {
	return p.SetValue(1)
}

// SetLow 设置低电平
func (p *Pin) SetLow() error {
	return p.SetValue(0)
}

// SetValue 设置引脚值: 0（低电平）或 1（高电平）
func (p *Pin) SetValue(value int) error {
	if value != 0 && value != 1 {
		return fmt.Errorf("无效电平值: %d，仅支持 0/1", value)
	}
	return os.WriteFile(
		filepath.Join(p.path, "value"),
		[]byte(strconv.Itoa(value)),
		0o644,
	)
}

// GetValue 读取引脚当前电平值
func (p *Pin) GetValue() (int, error) {
	data, err := os.ReadFile(filepath.Join(p.path, "value"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// Setup 导出引脚并设置为输出模式
func (p *Pin) Setup() error {
	if err := p.Export(); err != nil {
		return fmt.Errorf("导出 GPIO%d 失败: %v", p.Number, err)
	}
	if err := p.SetDirection("out"); err != nil {
		return fmt.Errorf("设置 GPIO%d 方向失败: %v", p.Number, err)
	}
	return nil
}

// Cleanup 将引脚设为低电平并取消导出
func (p *Pin) Cleanup() {
	_ = p.SetLow()
	_ = p.Unexport()
}

// ListExportedPins 列出当前已导出的 GPIO 引脚编号
func ListExportedPins() []int {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return nil
	}
	var pins []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "gpio") || strings.HasPrefix(name, "gpiochip") {
			continue
		}
		num, err := strconv.Atoi(name[4:])
		if err != nil {
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
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return nil
	}
	var chips []GPIOChipInfo
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "gpiochip") {
			continue
		}
		chipPath := filepath.Join(sysfsPath, name)
		info := GPIOChipInfo{Name: name}

		if data, err := os.ReadFile(filepath.Join(chipPath, "label")); err == nil {
			info.Label = strings.TrimSpace(string(data))
		}
		if data, err := os.ReadFile(filepath.Join(chipPath, "base")); err == nil {
			info.Base, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		if data, err := os.ReadFile(filepath.Join(chipPath, "ngpio")); err == nil {
			info.Ngpio, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		chips = append(chips, info)
	}
	sort.Slice(chips, func(i, j int) bool { return chips[i].Base < chips[j].Base })
	return chips
}

// String 返回引脚描述
func (p *Pin) String() string {
	dir, _ := p.GetDirection()
	val, _ := p.GetValue()
	level := "低"
	if val == 1 {
		level = "高"
	}
	return fmt.Sprintf("GPIO%d (方向=%s, 电平=%s)", p.Number, dir, level)
}
