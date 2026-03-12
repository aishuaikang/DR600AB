# DR600AB 工作区 — Copilot 指南

## 项目概览

Go 1.25.3 多模块工作区（`go.work`），包含三个模块，面向反无人机硬件设备：

| 模块 | 用途 | 协议 | 波特率 |
|------|------|------|--------|
| `serialport` | **共享串口基础库**（配置、发现、选择、打开） | — | — |
| `io-controller` | 10路射频干扰器控制（IO控制器） | 自定义二进制帧 `0xAA…0x55` | 9600 |
| `tri-detector` | 三合一无人机侦测数据解析 | 文本行 KV 格式 | 115200 |

## 架构

### serialport — 共享串口基础层

```
config.go    Config 结构体 + DefaultConfig 工厂（8N1 默认配置）
port.go      ListPorts / BuildMode / Open（PortName 为空时自动交互选择）
prompt.go    SelectPort（promptui 交互式选择 + 手动输入兜底）
```

- `Open(*Config)` 接受指针，打开后回填 `PortName`，调用方可直接读取
- 两个业务模块通过 `import "serialport"` 使用，不再各自实现串口基础逻辑

### io-controller — 三层架构（传输→协议→业务）

```
ui/          CLI 交互（promptui 菜单）→ 调用 device 层
device/      业务封装（打击控制、模块查询）→ 调用 protocol + client
protocol/    二进制帧编解码（帧头 0xAA / 帧尾 0x55 / 小端序）
client/      二进制帧协议客户端（SerialClient 包装已打开的 serial.Port）
```

- `client.NewSerialClient(port, portName, verbose)` — 接收 `serialport.Open` 返回的端口
- `device.Device` 持有 `client.Client` 接口，不直接依赖串口实现
- 协议帧格式：`| 帧头(1B) | 设备ID(2B LE) | 长度(1B) | 命令码(1B) | 数据域(NB) | 帧尾(1B) |`
- 命令码：`0x04`（开关功率）、`0x05`（模块查询）、`0x10`（下载地址）、`0x11`（查询地址）

### tri-detector — 三层架构（UI→处理→解析）

```
ui/          App 应用生命周期（信号处理、goroutine 管理、主循环）
handler/     全双工读写循环（goroutine 读取 + 主线程输入）
parser/      文本行 → 结构化 Message → JSON 输出
```

- 串口打开直接使用 `serialport.Open`，`ui.NewApp(port, portName, baudRate)` 接管运行
- 5 种报文类型：`did_encrypted`、`rid`、`did_plain`、`detect`、`heartbeat`
- 解析使用正则分割 KV 字段边界：`fieldBoundary = /,\s*[A-Za-z_#][A-Za-z0-9_# ]*=/`

## 关键约定

- **中文注释和错误消息** — 所有用户面消息使用中文
- **编译期接口检查** — `var _ Client = (*SerialClient)(nil)` 模式
- **小端序** — 二进制协议多字节字段统一使用 `binary.LittleEndian`
- **接口驱动设计** — `handler/` 和 `client/` 基于 `io.Reader/Writer` 或自定义接口，便于测试替换
- **串口参数 8N1** — 两个模块都使用 8 数据位、无校验、1 停止位

## 构建与测试

```bash
# 工作区级别构建（三个模块）
go build ./serialport/... && go build ./io-controller/... && go build ./tri-detector/...

# tri-detector 有 parser 单元测试（覆盖全部 5 种报文类型）
go test ./tri-detector/parser/ -v

# io-controller 目前无单元测试
```

## 依赖

仅两个外部依赖，由 `serialport` 模块统一引入：
- `github.com/manifoldco/promptui` — 终端交互式选择/输入
- `go.bug.st/serial` — 跨平台串口通信

## 开发注意事项

- 新增命令码时：在 `protocol/protocol.go` 添加常量和构建/解析函数，在 `device/` 封装业务方法，在 `ui/handlers.go` 添加菜单项
- 新增报文类型时：在 `parser/parser.go` 的 `ParseLine` 中添加类型判定分支和对应结构体，同步补充 `parser_test.go` 测试用例
- 频段映射定义在 `device/strike.go` 的 `FreqBands` 切片中（索引+1 = 模块路号）
- `protocol.FindFrame` 处理粘包/拆包，修改帧格式时需同步更新
- **新增串口设备模块时**：直接 `import "serialport"` 使用共享配置和打开逻辑，只需实现各自的协议处理层
