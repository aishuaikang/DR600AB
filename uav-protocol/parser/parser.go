// Package parser parses detector packets into shared protocol models.
package parser

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"uav-protocol/model"
	"uav-protocol/spectrum"
)

var fieldBoundary = regexp.MustCompile(`,\s*[A-Za-z_#][A-Za-z0-9_# ]*=`)

type Options struct {
	Now func() time.Time
}

type Parser struct {
	now func() time.Time
}

func New(opts Options) *Parser {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Parser{now: now}
}

func ParseLine(line string) (*model.Message, error) {
	return New(Options{}).ParseLine(line)
}

func (p *Parser) ParseLine(line string) (*model.Message, error) {
	payload := strings.TrimSpace(line)
	if payload == "" {
		return nil, fmt.Errorf("empty line")
	}
	if p == nil || p.now == nil {
		p = New(Options{})
	}
	if msg, ok := p.parseSpectrumHex(payload); ok {
		msg.Raw = payload
		return msg, nil
	}

	msg := &model.Message{Raw: payload, Time: p.now()}

	switch {
	case strings.HasPrefix(payload, "RID ") || strings.Contains(payload, "RID ssid="):
		msg.Type = model.TypeRID
		fields := parseKeyValues(strings.TrimPrefix(payload, "RID "))
		if !looksLikeRID(fields) {
			return nil, fmt.Errorf("unknown message type")
		}
		msg.Data = buildRID(fields)

	case strings.Contains(payload, "Heart Beat"):
		msg.Type = model.TypeHeartbeat
		idx := strings.Index(payload, "Heart Beat")
		fields := parseKeyValues(payload[:idx])
		msg.Data = buildHeartbeat(payload, fields)

	case strings.Contains(payload, "Encypted Mavic_O4_ID=") || strings.Contains(payload, "Encrypted Mavic_O4_ID="):
		msg.Type = model.TypeDIDEncrypted
		kvPayload := payload
		if bi := strings.Index(payload, "byte,"); bi != -1 {
			kvPayload = payload[:bi]
		}
		msg.Data = buildDIDEncrypted(payload, parseKeyValues(kvPayload))

	case strings.Contains(payload, "uuid="):
		msg.Type = model.TypeDIDPlain
		msg.Data = buildDIDPlain(parseKeyValues(payload))

	case strings.Contains(payload, "Empty packet"):
		msg.Type = model.TypeEmpty
		kvPayload := payload
		if idx := strings.Index(payload, ","); idx != -1 {
			kvPayload = payload[idx+1:]
		}
		msg.Data = buildEmpty(parseKeyValues(kvPayload))

	default:
		fields := parseKeyValues(payload)
		if !looksLikeDetect(payload, fields) {
			return nil, fmt.Errorf("unknown message type")
		}
		msg.Type = model.TypeDetect
		msg.Data = buildDetect(fields)
	}

	return msg, nil
}

func ParseBytes(data []byte) (*model.Message, bool, error) {
	return New(Options{}).ParseBytes(data)
}

func (p *Parser) ParseBytes(data []byte) (*model.Message, bool, error) {
	if p == nil || p.now == nil {
		p = New(Options{})
	}
	if utf8.Valid(data) {
		msg, err := p.ParseLine(string(data))
		if err == nil {
			return msg, msg.Type == model.TypeSpectrum, nil
		}
		if msg, ok := p.ParseSpectrum(data); ok {
			return msg, true, nil
		}
		return nil, false, err
	}
	if msg, ok := p.ParseSpectrum(data); ok {
		return msg, true, nil
	}
	return nil, false, fmt.Errorf("unknown binary packet")
}

func ParseHex(payload string) (*model.Message, error) {
	return New(Options{}).ParseHex(payload)
}

func (p *Parser) ParseHex(payload string) (*model.Message, error) {
	if p == nil || p.now == nil {
		p = New(Options{})
	}
	msg, ok := p.parseSpectrumHex(strings.TrimSpace(payload))
	if !ok {
		return nil, fmt.Errorf("unknown spectrum hex packet")
	}
	msg.Raw = strings.TrimSpace(payload)
	return msg, nil
}

func ParseSpectrum(data []byte) (*model.Message, bool) {
	return New(Options{}).ParseSpectrum(data)
}

func (p *Parser) ParseSpectrum(data []byte) (*model.Message, bool) {
	if p == nil || p.now == nil {
		p = New(Options{})
	}
	frame, ok := spectrum.ParseFrame(data)
	if !ok {
		return nil, false
	}
	return &model.Message{
		Type: model.TypeSpectrum,
		Time: p.now(),
		Raw:  strings.ToUpper(hex.EncodeToString(data)),
		Data: &frame,
	}, true
}

func buildDIDEncrypted(payload string, f map[string]string) *model.DIDEncrypted {
	return &model.DIDEncrypted{
		Device:      f["device"],
		EncryptedID: getEncryptedID(f),
		Freq:        toFloat(f["freq"]),
		RSSI:        toFloat(f["rssi"]),
		Bytes:       extractBytesString(payload),
	}
}

func buildRID(f map[string]string) *model.RID {
	return &model.RID{
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

func buildDIDPlain(f map[string]string) *model.DIDPlain {
	return &model.DIDPlain{
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
		NorthV:   toFloat(f["NothV"]),
		UpV:      toFloat(f["UpV"]),
		Freq:     toFloat(f["freq"]),
		RSSI:     toFloat(f["rssi"]),
		Distance: f["distance"],
	}
}

func buildDetect(f map[string]string) *model.Detect {
	modelName := f["model"]
	if idx := strings.Index(modelName, ","); idx != -1 {
		modelName = strings.TrimSpace(modelName[:idx])
	}
	return &model.Detect{
		Device:    f["device"],
		Model:     modelName,
		Freq:      toFloat(f["freq"]),
		RSSI:      toFloat(f["rssi"]),
		Bandwidth: parseBandwidth(f["bw"]),
		Seq:       toInt64(f["seq"]),
		GPIO:      toInt64(f["gpio"]),
	}
}

func buildHeartbeat(payload string, f map[string]string) *model.Heartbeat {
	hb := &model.Heartbeat{Device: f["device"]}
	idx := strings.Index(payload, "Heart Beat")
	if idx == -1 {
		return hb
	}
	tail := strings.TrimLeft(strings.TrimSpace(payload[idx+len("Heart Beat"):]), ", ")
	if parts := strings.SplitN(tail, ",", 2); len(parts) > 0 {
		hb.Seq = strings.TrimSpace(parts[0])
	}
	return hb
}

func buildEmpty(f map[string]string) *model.Empty {
	return &model.Empty{
		Freq: toFloat(f["freq"]),
		RSSI: toFloat(f["rssi"]),
	}
}

func (p *Parser) parseSpectrumHex(payload string) (*model.Message, bool) {
	data, ok := decodeHexPayload(payload)
	if !ok {
		return nil, false
	}
	return p.ParseSpectrum(data)
}

func decodeHexPayload(payload string) ([]byte, bool) {
	normalized, ok := normalizeHexPayload(payload)
	if !ok {
		return nil, false
	}
	data, err := hex.DecodeString(normalized)
	if err != nil {
		return nil, false
	}
	return data, true
}

func normalizeHexPayload(payload string) (string, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", false
	}
	var builder strings.Builder
	builder.Grow(len(payload))
	for i := 0; i < len(payload); i++ {
		ch := payload[i]
		if ch == '0' && i+1 < len(payload) && (payload[i+1] == 'x' || payload[i+1] == 'X') {
			i++
			continue
		}
		switch {
		case isHexByte(ch):
			builder.WriteByte(ch)
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ',' || ch == ':' || ch == '-':
			continue
		default:
			return "", false
		}
	}
	if builder.Len() == 0 || builder.Len()%2 != 0 {
		return "", false
	}
	return builder.String(), true
}

func parseGPS(s string) model.GPS {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return model.GPS{}
	}
	return model.GPS{Lat: toFloat(parts[1]), Lng: toFloat(parts[0])}
}

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
		if next == nil {
			result[key] = strings.TrimSpace(strings.TrimRight(rest, ", "))
			i = len(payload)
			continue
		}
		result[key] = strings.TrimSpace(rest[:next[0]])
		i = valueStart + next[0] + 1
	}
	return result
}

func getEncryptedID(f map[string]string) string {
	if v, ok := f["Encypted Mavic_O4_ID"]; ok {
		return v
	}
	if v, ok := f["Encrypted Mavic_O4_ID"]; ok {
		return v
	}
	return ""
}

func extractBytesString(payload string) string {
	idx := strings.Index(payload, "byte,")
	if idx == -1 {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(payload[idx+len("byte,"):]), ",")
	var sb strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			sb.WriteString(part)
		}
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
	required := []string{"ssid", "serial", "model", "UA_type", "drone_GPS", "pilot_GPS", "MAC", "rssi", "freq"}
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

func parseBandwidth(s string) float64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimSuffix(strings.TrimSuffix(s, "MHZ"), "M")
	return toFloat(s)
}

func toInt64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func isHexByte(ch byte) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}
