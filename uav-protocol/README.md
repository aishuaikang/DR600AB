# uav-protocol

`uav-protocol` 是无人机侦测协议公共库，抽离自 600AB 主服务和 `uav_defender` 中可复用的解析、解密和合并规则。这个库只包含纯协议逻辑，不包含数据库、缓存锁、SSE/MQTT 推送、白名单、设备配置或页面 DTO。

## 包结构

- `model`: 报文、坐标、目标和轨迹等标准数据结构。
- `parser`: 报文解析入口，支持文本报文、频谱原始二进制、频谱 HEX 字符串。
- `spectrum`: 频谱二进制帧识别、解析和频谱数组组装。
- `diddecrypt`: O3/O4 DID 加密报文解密状态机，业务侧注入 MQTT/HTTP 客户端。
- `merge`: 侦测匹配、定位序列号容错、DID 关联、距离关系和轨迹合并策略。
- 根包 `uavprotocol`: 文本和频谱的统一解析入口。

## 导入

```go
import (
    uavprotocol "uav-protocol"
    "uav-protocol/diddecrypt"
    "uav-protocol/merge"
    "uav-protocol/model"
    "uav-protocol/parser"
    "uav-protocol/spectrum"
)
```

## 统一解析入口

优先使用根包 `uavprotocol`。它会把文本、频谱二进制和频谱 HEX 都包装成统一的 `model.Message`。

```go
p := uavprotocol.NewParser(uavprotocol.Options{})

msg, err := p.ParseText(line)
if err != nil {
    // 未知文本报文
}

msg, err = p.ParseHex(spectrumHex)

msg, isSpectrum, err := p.ParseBytes(raw)
_ = isSpectrum
_ = msg
```

`ParseBytes` 的输入可以是：

- 文本报文原始 bytes。
- 频谱原始二进制帧。
- 频谱 HEX 字符串 bytes。

频谱 HEX 支持连续字符串，也支持常见分隔格式：

```text
0036ee80465046b4
00 36 ee 80 46 50 46 b4
0x00,0x36,0xee,0x80,0x46,0x50,0x46,0xb4
00:36:ee:80:46:50:46:b4
```

`parser` 包也可以直接使用：

```go
msg, err := parser.ParseLine(lineOrSpectrumHex)
msg, err = parser.ParseHex(spectrumHex)
msg, isSpectrum, err := parser.ParseBytes(rawOrSpectrumHex)
msg, ok := parser.ParseSpectrum(rawBinary)
_ = ok
```

频谱数据会被包装成：

```go
msg := model.Message{
    Type: model.TypeSpectrum,
    Data: frame, // *spectrum.Frame
}
```

## MessageType

当前标准报文类型定义在 `model` 包中，`parser` 包保留了同名别名，方便兼容原有使用方式。

```go
const (
    model.TypeUnknown
    model.TypeDIDEncrypted
    model.TypeRID
    model.TypeDIDPlain
    model.TypeDetect
    model.TypeHeartbeat
    model.TypeEmpty
    model.TypeSpectrum
)
```

文本报文会按内容解析成 `TypeDetect`、`TypeRID`、`TypeDIDPlain`、`TypeDIDEncrypted`、`TypeHeartbeat` 或 `TypeEmpty`。频谱报文统一解析成 `TypeSpectrum`，`Data` 为 `*spectrum.Frame`。

## 频谱处理

频谱不参与目标合并。解析后业务侧只需要更新频谱视图或频谱快照：

```go
msg, ok := parser.ParseSpectrum(rawBinary)
if ok && msg.Type == model.TypeSpectrum {
    frame := msg.Data.(*spectrum.Frame)
    nextSnapshot, result := spectrum.ApplyFrame(snapshot, *frame, spectrum.DefaultFreqStepMHz)
    _ = nextSnapshot
    _ = result
}
```

常用命令组装：

```go
startCommand := spectrum.BuildAnalysisCommand(2400, 2500)
stopCommand := spectrum.BuildStopCommand()
_ = startCommand
_ = stopCommand
```

## 合并策略

侦测和定位是两套合并逻辑：

- 侦测数据按机型和频点容错匹配。
- 定位数据按序列号、RID 前缀、序列号后缀容错和 DID 加密关联匹配。
- 频谱数据不做目标合并。

```go
sameDetection := merge.DetectionMatches(
    existingModel,
    existingFrequency,
    incomingModel,
    incomingFrequency,
    merge.DetectionOptions{},
)

samePosition := merge.PositionMatches(existing, incoming)
relations := merge.PositionRelations(devicePoint, incoming.Drone, incoming.Pilot)
```

`PositionRelations` 只在设备坐标有效时计算距离和方位。只要有设备坐标和无人机/飞手坐标，就会计算：

- `DroneDistanceM`: 设备到无人机距离。
- `PilotDistanceM`: 设备到飞手距离。
- `DroneDirectionDeg`: 设备指向无人机方位角。
- `DeviceDirectionDeg`: 无人机指向设备方位角。

轨迹策略默认：

- `<= 3m`: 抖动合并。
- `> 500m`: GPS 跳变，丢弃旧轨迹并从新点重新开始。
- 默认不按数量裁剪；如显式设置 `TrajectoryOptions.Limit > 0`，只保留最新的 `Limit` 个点。

```go
trajectory = merge.AppendTrajectory(
    trajectory,
    incoming.Drone,
    incoming.LastSeen,
    incoming.Speed,
    incoming.Height,
    merge.TrajectoryOptions{},
)
```

## DID 加密报文

`diddecrypt` 只维护 O3/O4 解密状态机，不直接依赖具体 MQTT/HTTP 实现。业务侧实现 `diddecrypt.Client` 后注入。

```go
decoder := diddecrypt.NewDecoder(client, diddecrypt.Options{
    EmitUncrackedTarget: false,
})

out := decoder.Decode(ctx, packet, deviceSN, receivedAt)
if out.HasTarget {
    target := out.Target
    _ = target
}
```

状态含义：

- `StatusDecoded`: 已解密。
- `StatusKeyCached`: 密钥包已缓存。
- `StatusKeyAlreadyCached`: 密钥包已存在。
- `StatusPendingKey`: 动态包等待密钥包。
- `StatusUncracked`: 解密失败或暂未破解。
- `StatusUnsupported`: 非支持的 O3/O4 包类型。
- `StatusInvalid`: 输入或客户端状态无效。

## 验证

从仓库根目录执行：

```bash
go test ./uav-protocol/...
go build ./uav-protocol/...
```
