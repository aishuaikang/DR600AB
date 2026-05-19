package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"dr600ab-api/internal/model"
	"tri-detector/parser"
)

const (
	o3MQTTDefaultRequestQoS = byte(1)
	o3KeyPacketTTL          = 10 * time.Minute
)

type o3PacketType int

const (
	o3PacketUnknown o3PacketType = iota
	o3PacketDirect
	o3PacketKey
	o3PacketDynamic
)

type mqttO3PlusO4Decoder struct {
	options O3DecryptOptions

	mu            sync.Mutex
	client        mqtt.Client
	subMu         sync.Mutex
	subscriptions sync.Map
	responses     sync.Map
	keyPackets    sync.Map
}

type o3KeyPacket struct {
	EncryptedID string
	Hex         string
	Device      string
	Frequency   float64
	RSSI        float64
	CachedAt    time.Time
}

type o3DecryptRequest struct {
	RequestID   string  `json:"request_id"`
	Data        string  `json:"data"`
	IP          string  `json:"ip"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Country     string  `json:"country"`
	CountryName string  `json:"country_name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
	DeviceLat   float64 `json:"device_lat"`
	DeviceLon   float64 `json:"device_lon"`
}

type o3MQTTDecryptResponse struct {
	RequestID string         `json:"request_id"`
	Success   bool           `json:"success"`
	Message   string         `json:"message"`
	Data      o3DecryptAlert `json:"data"`
}

type o3DecryptAlert struct {
	Msg      string  `json:"msg,omitempty"`
	Note     string  `json:"note,omitempty"`
	SN       string  `json:"sn,omitempty"`
	Model    string  `json:"model,omitempty"`
	Lon      float64 `json:"lon,omitempty"`
	Lat      float64 `json:"lat,omitempty"`
	Alt      float64 `json:"alt,omitempty"`
	Height   float64 `json:"height,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	Z        float64 `json:"z,omitempty"`
	PilotLon float64 `json:"pilot_lon,omitempty"`
	PilotLat float64 `json:"pilot_lat,omitempty"`
	HomeLon  float64 `json:"home_lon,omitempty"`
	HomeLat  float64 `json:"home_lat,omitempty"`
	GPSTime  string  `json:"gps_time,omitempty"`
	SeqNum   int     `json:"seq_num,omitempty"`
	Type     int     `json:"type,omitempty"`
	UUID     string  `json:"uuid,omitempty"`
	Yaw      float64 `json:"yaw,omitempty"`
}

type o3KeygenResult struct {
	Msg     string
	Note    string
	Success bool
	Err     error
	Data    o3DecryptAlert
}

// NewMQTTO3PlusO4Decoder 创建 DID 加密报文的 MQTT 解密器。配置不完整时返回 nil。
func NewMQTTO3PlusO4Decoder(options O3DecryptOptions) O3PlusO4Decoder {
	options.Broker = strings.TrimSpace(options.Broker)
	options.Username = strings.TrimSpace(options.Username)
	if !options.Enabled || options.Broker == "" || options.Port <= 0 || options.Username == "" {
		return nil
	}
	if options.Timeout <= 0 {
		options.Timeout = 10 * time.Second
	}
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = 10 * time.Second
	}
	return &mqttO3PlusO4Decoder{options: options}
}

// ParseO3PlusO4PacketMQTT 借鉴旧项目的 O3+/O4 MQTT 分支，解出目标定位摘要后返回定位目标。
func (d *mqttO3PlusO4Decoder) ParseO3PlusO4PacketMQTT(
	ctx context.Context,
	packet parser.DIDEncrypted,
	deviceSN string,
	receivedAt time.Time,
) (model.ScreenPositionTarget, bool) {
	if d == nil {
		return model.ScreenPositionTarget{}, false
	}

	rawHex := strings.ToLower(strings.TrimSpace(packet.Bytes))
	_, decryptedHex, ok := normalizeO3PacketHex(rawHex)
	if !ok {
		return model.ScreenPositionTarget{}, false
	}

	encryptedID := strings.ToLower(strings.TrimSpace(packet.EncryptedID))
	packetType := getO3PacketType(decryptedHex)
	if encryptedID == "" || packetType == o3PacketUnknown {
		return model.ScreenPositionTarget{}, false
	}

	sn := strings.TrimSpace(deviceSN)
	if sn == "" {
		sn = strings.TrimSpace(packet.Device)
	}
	if sn == "" {
		return model.ScreenPositionTarget{}, false
	}

	switch packetType {
	case o3PacketDirect:
		result, err := d.decrypt(ctx, decryptedHex, sn)
		if err != nil {
			return model.ScreenPositionTarget{}, false
		}
		return d.positionFromDecryptResult(packet, result, receivedAt)

	case o3PacketKey:
		if d.getKeyPacket(encryptedID) != nil {
			return model.ScreenPositionTarget{}, false
		}
		result := d.sendKeyPacket(ctx, decryptedHex, sn)
		if result.Success {
			d.cacheKeyPacket(encryptedID, decryptedHex, packet)
		}
		return model.ScreenPositionTarget{}, false

	case o3PacketDynamic:
		if d.getKeyPacket(encryptedID) == nil {
			return model.ScreenPositionTarget{}, false
		}
		result, err := d.decrypt(ctx, decryptedHex, sn)
		if err != nil {
			return model.ScreenPositionTarget{}, false
		}
		return d.positionFromDecryptResult(packet, result, receivedAt)

	default:
		return model.ScreenPositionTarget{}, false
	}
}

func (d *mqttO3PlusO4Decoder) decrypt(ctx context.Context, hexStr, sn string) (o3DecryptAlert, error) {
	resp, err := d.publishAndWait(ctx, hexStr, sn)
	if err != nil {
		return o3DecryptAlert{}, err
	}
	if resp == nil {
		return o3DecryptAlert{}, fmt.Errorf("empty MQTT decrypt response")
	}
	if !resp.Success {
		return o3DecryptAlert{}, fmt.Errorf("MQTT decrypt failed: %s", resp.Message)
	}
	return resp.Data, nil
}

func (d *mqttO3PlusO4Decoder) sendKeyPacket(ctx context.Context, hexStr, sn string) o3KeygenResult {
	resp, err := d.publishAndWait(ctx, hexStr, sn)
	if err != nil {
		return o3KeygenResult{Err: err}
	}
	if resp == nil {
		return o3KeygenResult{Err: fmt.Errorf("empty MQTT keygen response")}
	}
	msg := strings.TrimSpace(resp.Data.Msg)
	return o3KeygenResult{
		Msg:     msg,
		Note:    resp.Data.Note,
		Success: msg == "keygen_succ" || msg == "key_exist",
		Data:    resp.Data,
	}
}

func (d *mqttO3PlusO4Decoder) publishAndWait(ctx context.Context, hexStr, sn string) (*o3MQTTDecryptResponse, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	if err := d.ensureSubscription(sn); err != nil {
		return nil, err
	}

	timeout := d.options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	requestID := uuid.NewString()
	responseCh := make(chan *o3MQTTDecryptResponse, 1)
	d.responses.Store(requestID, responseCh)
	defer d.responses.Delete(requestID)

	requestBytes, err := json.Marshal(o3DecryptRequest{
		RequestID: requestID,
		Data:      hexStr,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal MQTT decrypt request: %w", err)
	}

	requestTopic := fmt.Sprintf("%s/%s", d.options.Username, sn)
	token := d.client.Publish(requestTopic, o3MQTTDefaultRequestQoS, false, requestBytes)
	if !token.WaitTimeout(2 * time.Second) {
		return nil, fmt.Errorf("publish MQTT decrypt request timeout")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("publish MQTT decrypt request: %w", err)
	}

	select {
	case resp := <-responseCh:
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("MQTT decrypt timeout: %w", ctx.Err())
	}
}

func (d *mqttO3PlusO4Decoder) ensureClient() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil && d.client.IsConnected() {
		return nil
	}

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%d", d.options.Broker, d.options.Port)).
		SetClientID(fmt.Sprintf("dr600ab_%s", uuid.NewString()[:8])).
		SetUsername(d.options.Username).
		SetPassword(d.options.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetCleanSession(false).
		SetResumeSubs(true).
		SetConnectTimeout(d.options.ConnectTimeout)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(d.options.ConnectTimeout) {
		return fmt.Errorf("connect MQTT decrypt server timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("connect MQTT decrypt server: %w", err)
	}

	d.client = client
	d.subscriptions.Range(func(key, _ any) bool {
		d.subscriptions.Delete(key)
		return true
	})
	return nil
}

func (d *mqttO3PlusO4Decoder) ensureSubscription(sn string) error {
	if _, ok := d.subscriptions.Load(sn); ok {
		return nil
	}

	d.subMu.Lock()
	defer d.subMu.Unlock()
	if _, ok := d.subscriptions.Load(sn); ok {
		return nil
	}
	if err := d.ensureClient(); err != nil {
		return err
	}

	responseTopic := fmt.Sprintf("%s/%s/response", d.options.Username, sn)
	token := d.client.Subscribe(responseTopic, o3MQTTDefaultRequestQoS, d.handleMessage)
	if !token.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("subscribe MQTT decrypt response timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribe MQTT decrypt response: %w", err)
	}

	d.subscriptions.Store(sn, struct{}{})
	return nil
}

func (d *mqttO3PlusO4Decoder) handleMessage(_ mqtt.Client, message mqtt.Message) {
	var resp o3MQTTDecryptResponse
	if err := json.Unmarshal(message.Payload(), &resp); err != nil || resp.RequestID == "" {
		return
	}
	value, ok := d.responses.Load(resp.RequestID)
	if !ok {
		return
	}
	ch, ok := value.(chan *o3MQTTDecryptResponse)
	if !ok {
		return
	}
	select {
	case ch <- &resp:
	default:
	}
}

func (d *mqttO3PlusO4Decoder) cacheKeyPacket(encryptedID, hexStr string, packet parser.DIDEncrypted) {
	d.keyPackets.Store(encryptedID, &o3KeyPacket{
		EncryptedID: encryptedID,
		Hex:         hexStr,
		Device:      packet.Device,
		Frequency:   packet.Freq,
		RSSI:        packet.RSSI,
		CachedAt:    time.Now(),
	})
}

func (d *mqttO3PlusO4Decoder) getKeyPacket(encryptedID string) *o3KeyPacket {
	value, ok := d.keyPackets.Load(encryptedID)
	if !ok {
		return nil
	}
	packet, ok := value.(*o3KeyPacket)
	if !ok {
		d.keyPackets.Delete(encryptedID)
		return nil
	}
	if time.Since(packet.CachedAt) > o3KeyPacketTTL {
		d.keyPackets.Delete(encryptedID)
		return nil
	}
	return packet
}

func (d *mqttO3PlusO4Decoder) positionFromDecryptResult(
	packet parser.DIDEncrypted,
	result o3DecryptAlert,
	receivedAt time.Time,
) (model.ScreenPositionTarget, bool) {
	serial := cleanO3String(result.SN)
	if serial == "" {
		serial = strings.TrimSpace(packet.EncryptedID)
	}
	modelName := strings.TrimSpace(result.Model)
	if modelName == "" {
		modelName = "DJI-Drone"
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}

	target := model.ScreenPositionTarget{
		Serial:    serial,
		Model:     modelName,
		Source:    string(parser.TypeDIDEncrypted),
		Frequency: packet.Freq,
		RSSI:      packet.RSSI,
		Devices:   uniqueNonEmpty(packet.Device),
		Drone:     pointFromLatLng(result.Lat, result.Lon),
		Pilot:     pointFromLatLng(result.PilotLat, result.PilotLon),
		Home:      pointFromLatLng(result.HomeLat, result.HomeLon),
		Height:    nonZeroFloatPtr(result.Height),
		Altitude:  nonZeroFloatPtr(result.Alt),
		Speed:     nonZeroFloatPtr(calculateFlightSpeed(result.X, result.Y, result.Z)),
		Cracked:   true,
		FirstSeen: receivedAt,
		LastSeen:  receivedAt,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       string(parser.TypeDIDEncrypted),
			ReceivedAt: receivedAt,
			Device:     packet.Device,
			Serial:     serial,
			Model:      modelName,
			Frequency:  packet.Freq,
			RSSI:       packet.RSSI,
			Cracked:    true,
		},
	}
	return target, screenPositionHasCoordinate(target)
}

func pointFromLatLng(lat, lng float64) *model.ScreenPositionPoint {
	if !validCoordinate(lat, lng) {
		return nil
	}
	return &model.ScreenPositionPoint{Latitude: lat, Longitude: lng}
}

func cleanO3String(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\x00")
	return strings.TrimSpace(value)
}

func getO3PacketType(hexStr string) o3PacketType {
	if len(hexStr) < 2 {
		return o3PacketUnknown
	}
	switch strings.ToLower(hexStr[:2]) {
	case "6d":
		return o3PacketDirect
	case "aa", "a3":
		return o3PacketKey
	case "87", "80":
		return o3PacketDynamic
	default:
		return o3PacketUnknown
	}
}
