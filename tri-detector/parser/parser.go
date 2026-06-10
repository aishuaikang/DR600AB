package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// -------- 消息类型 --------

type MessageType string

const (
	TypeDIDEncrypted MessageType = "did_encrypted" // DID 秘文
	TypeRID          MessageType = "rid"           // Remote ID
	TypeDIDPlain     MessageType = "did_plain"     // DID 明文
	TypeDetect       MessageType = "detect"        // 侦测数据
	TypeHeartbeat    MessageType = "heartbeat"     // 心跳
)

// -------- 统一包装 --------

type Message struct {
	Type MessageType `json:"type"`
	Time time.Time   `json:"time"`
	Raw  string      `json:"raw"`
	Data any         `json:"data"`
}

// -------- GPS 结构 --------

type GPS struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func parseGPS(s string) GPS {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return GPS{}
	}
	return GPS{
		Lat: toFloat(parts[1]),
		Lng: toFloat(parts[0]),
	}
}

// -------- 各类报文结构 --------

// DID 秘文
type DIDEncrypted struct {
	Device      string  `json:"device"`
	EncryptedID string  `json:"encrypted_id"`
	Freq        float64 `json:"freq"`
	RSSI        float64 `json:"rssi"`
	Bytes       string  `json:"bytes"` // 去掉逗号拼接的十六进制串
}

// RID
type RID struct {
	SSID      string  `json:"ssid"`
	Serial    string  `json:"serial"`
	Version   string  `json:"ver"`
	Name      string  `json:"name"`
	Model     string  `json:"model"`
	UAType    string  `json:"ua_type"`
	DroneGPS  GPS     `json:"drone_gps"`
	PilotGPS  GPS     `json:"pilot_gps"`
	Speed     float64 `json:"speed"`
	Vspeed    float64 `json:"vspeed"`
	Direc     float64 `json:"direc"`
	AltitudeP float64 `json:"altitude_p"`
	AltitudeG float64 `json:"altitude_g"`
	HeightAGL float64 `json:"height_agl"`
	MAC       string  `json:"mac"`
	RSSI      float64 `json:"rssi"`
	Freq      float64 `json:"freq"`
}

// DID 明文
type DIDPlain struct {
	Device   string  `json:"device"`
	Serial   string  `json:"serial"`
	Model    string  `json:"model"`
	UUID     string  `json:"uuid"`
	DroneGPS GPS     `json:"drone_gps"`
	HomeGPS  GPS     `json:"home_gps"`
	PilotGPS GPS     `json:"pilot_gps"`
	Height   float64 `json:"height"`
	Altitude float64 `json:"altitude"`
	EastV    float64 `json:"east_v"`
	NothV    float64 `json:"noth_v"`
	UpV      float64 `json:"up_v"`
	Freq     float64 `json:"freq"`
	RSSI     float64 `json:"rssi"`
	Distance string  `json:"distance"`
}

// 侦测数据报文
type Detect struct {
	Device string  `json:"device"`
	Model  string  `json:"model"`
	Freq   float64 `json:"freq"`
	RSSI   float64 `json:"rssi"`
}

// 心跳
type Heartbeat struct {
	Device string `json:"device"`
	Seq    string `json:"seq"`
}

// -------- 解析入口 --------

// fieldBoundary 匹配 ", key=" 边界，key 允许包含空格（如 Encypted Mavic_O4_ID）
var fieldBoundary = regexp.MustCompile(`,\s*[A-Za-z_#][A-Za-z0-9_# ]*=`)

func ParseLine(line string) (*Message, error) {
	payload := strings.TrimSpace(line)
	if payload == "" {
		return nil, fmt.Errorf("empty line")
	}

	msg := &Message{Raw: payload, Time: time.Now()}

	switch {
	case strings.HasPrefix(payload, "RID ") || strings.Contains(payload, "RID ssid="):
		msg.Type = TypeRID
		// 去掉 "RID " 前缀再解析 KV
		kvPayload := strings.TrimPrefix(payload, "RID ")
		fields := parseKeyValues(kvPayload)
		if !looksLikeRID(fields) {
			return nil, fmt.Errorf("unknown message type")
		}
		msg.Data = buildRID(fields)

	case strings.Contains(payload, "Heart Beat"):
		msg.Type = TypeHeartbeat
		// 截取 Heart Beat 之前的部分解析 KV
		idx := strings.Index(payload, "Heart Beat")
		kvPayload := payload[:idx]
		fields := parseKeyValues(kvPayload)
		msg.Data = buildHeartbeat(payload, fields)

	case strings.Contains(payload, "Encypted Mavic_O4_ID=") || strings.Contains(payload, "Encrypted Mavic_O4_ID="):
		msg.Type = TypeDIDEncrypted
		// 截取 byte, 之前的部分解析 KV
		kvPayload := payload
		if bi := strings.Index(payload, "byte,"); bi != -1 {
			kvPayload = payload[:bi]
		}
		fields := parseKeyValues(kvPayload)
		msg.Data = buildDIDEncrypted(payload, fields)

	case strings.Contains(payload, "uuid="):
		msg.Type = TypeDIDPlain
		fields := parseKeyValues(payload)
		msg.Data = buildDIDPlain(fields)

	default:
		fields := parseKeyValues(payload)
		if looksLikeDetect(payload, fields) {
			msg.Type = TypeDetect
			msg.Data = buildDetect(fields)
		} else {
			return nil, fmt.Errorf("unknown message type")
		}
	}

	return msg, nil
}

// -------- 各类构建函数 --------

func buildDIDEncrypted(payload string, f map[string]string) *DIDEncrypted {
	return &DIDEncrypted{
		Device:      f["device"],
		EncryptedID: getEncryptedID(f),
		Freq:        toFloat(f["freq"]),
		RSSI:        toFloat(f["rssi"]),
		Bytes:       extractBytesString(payload),
	}
}

func buildRID(f map[string]string) *RID {
	return &RID{
		SSID:      f["ssid"],
		Serial:    f["serial"],
		Version:   f["ver"],
		Name:      f["name"],
		Model:     f["model"],
		UAType:    f["UA_type"],
		DroneGPS:  parseGPS(f["drone_GPS"]),
		PilotGPS:  parseGPS(f["pilot_GPS"]),
		Speed:     toFloat(f["speed"]),
		Vspeed:    toFloat(f["Vspeed"]),
		Direc:     toFloat(f["direc"]),
		AltitudeP: toFloat(f["AltitudeP"]),
		AltitudeG: toFloat(f["AltitudeG"]),
		HeightAGL: toFloat(f["Height_AGL"]),
		MAC:       f["MAC"],
		RSSI:      toFloat(f["rssi"]),
		Freq:      toFloat(f["freq"]),
	}
}

func buildDIDPlain(f map[string]string) *DIDPlain {
	return &DIDPlain{
		Device:   f["device"],
		Serial:   f["serial"],
		Model:    f["model"],
		UUID:     f["uuid"],
		DroneGPS: parseGPS(f["drone_GPS"]),
		HomeGPS:  parseGPS(f["home_GPS"]),
		PilotGPS: parseGPS(f["pilot_GPS"]),
		Height:   toFloat(f["Height"]),
		Altitude: toFloat(f["Altitude"]),
		EastV:    toFloat(f["EastV"]),
		NothV:    toFloat(f["NothV"]),
		UpV:      toFloat(f["UpV"]),
		Freq:     toFloat(f["freq"]),
		RSSI:     toFloat(f["rssi"]),
		Distance: f["distance"],
	}
}

func buildDetect(f map[string]string) *Detect {
	// model 后可能跟随额外字段(如 wifi mac=, =SSID)，只取第一个逗号之前的部分
	model := f["model"]
	if idx := strings.Index(model, ","); idx != -1 {
		model = strings.TrimSpace(model[:idx])
	}
	return &Detect{
		Device: f["device"],
		Model:  model,
		Freq:   toFloat(f["freq"]),
		RSSI:   toFloat(f["rssi"]),
	}
}

func buildHeartbeat(payload string, f map[string]string) *Heartbeat {
	hb := &Heartbeat{
		Device: f["device"],
	}
	// Heart Beat 后跟的第一个数字为 seq
	idx := strings.Index(payload, "Heart Beat")
	if idx != -1 {
		tail := strings.TrimSpace(payload[idx+len("Heart Beat"):])
		tail = strings.TrimLeft(tail, ", ")
		if parts := strings.SplitN(tail, ",", 2); len(parts) > 0 {
			hb.Seq = strings.TrimSpace(parts[0])
		}
	}
	return hb
}

// -------- 内部工具函数 --------

func parseKeyValues(payload string) map[string]string {
	result := make(map[string]string)
	i := 0

	for i < len(payload) {
		for i < len(payload) && (payload[i] == ',' || payload[i] == ' ') {
			i++
		}
		if i >= len(payload) {
			break
		}

		eq := strings.IndexByte(payload[i:], '=')
		if eq == -1 {
			break
		}
		eq = i + eq

		key := strings.TrimSpace(payload[i:eq])
		if key == "" {
			break
		}

		valueStart := eq + 1
		rest := payload[valueStart:]
		next := fieldBoundary.FindStringIndex(rest)

		var value string
		if next == nil {
			value = strings.TrimSpace(strings.TrimRight(rest, ", "))
			i = len(payload)
		} else {
			value = strings.TrimSpace(rest[:next[0]])
			i = valueStart + next[0] + 1
		}

		result[key] = value
	}

	return result
}

// getEncryptedID 从 fields 中取 Encypted/Encrypted Mavic_O4_ID 的值
func getEncryptedID(f map[string]string) string {
	if v, ok := f["Encypted Mavic_O4_ID"]; ok {
		return v
	}
	if v, ok := f["Encrypted Mavic_O4_ID"]; ok {
		return v
	}
	return ""
}

// extractBytesString 提取 byte, 后的十六进制并拼成无分隔符字符串
func extractBytesString(payload string) string {
	idx := strings.Index(payload, "byte,")
	if idx == -1 {
		return ""
	}
	raw := strings.TrimSpace(payload[idx+len("byte,"):])
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	var sb strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sb.WriteString(p)
	}
	return sb.String()
}

func looksLikeDetect(payload string, fields map[string]string) bool {
	if strings.Contains(payload, "Heart Beat") || strings.Contains(payload, "RID ") {
		return false
	}
	_, okDevice := fields["device"]
	_, okModel := fields["model"]
	_, okRSSI := fields["rssi"]
	return okDevice && okModel && okRSSI
}

func looksLikeRID(fields map[string]string) bool {
	required := []string{
		"ssid",
		"serial",
		"model",
		"UA_type",
		"drone_GPS",
		"pilot_GPS",
		"MAC",
		"rssi",
		"freq",
	}
	for _, key := range required {
		if strings.TrimSpace(fields[key]) == "" {
			return false
		}
	}
	return true
}

func toFloat(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(s), "km"))
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
