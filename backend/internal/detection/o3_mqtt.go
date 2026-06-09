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
	"uav-protocol/diddecrypt"
	protocolmodel "uav-protocol/model"
	"uav-protocol/parser"
)

const o3MQTTDefaultRequestQoS = byte(1)

type mqttO3PlusO4Decoder struct {
	options O3DecryptOptions

	decoderMu sync.Mutex
	decoder   *diddecrypt.Decoder

	mu            sync.Mutex
	client        mqtt.Client
	subMu         sync.Mutex
	subscriptions sync.Map
	responses     sync.Map
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
	RequestID string                   `json:"request_id"`
	Success   bool                     `json:"success"`
	Message   string                   `json:"message"`
	Data      diddecrypt.DecryptResult `json:"data"`
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
	decoder := &mqttO3PlusO4Decoder{options: options}
	decoder.decoder = diddecrypt.NewDecoder(decoder, diddecrypt.Options{
		RequireDecodedCoordinate: true,
	})
	return decoder
}

// ParseO3PlusO4PacketMQTT 解密 O3/O4 DID 加密报文并返回定位目标。
func (d *mqttO3PlusO4Decoder) ParseO3PlusO4PacketMQTT(
	ctx context.Context,
	packet parser.DIDEncrypted,
	deviceSN string,
	receivedAt time.Time,
) (model.ScreenPositionTarget, bool) {
	if d == nil {
		return model.ScreenPositionTarget{}, false
	}
	out := d.didDecoder().Decode(ctx, packet, deviceSN, receivedAt)
	if out.Err != nil || !out.HasTarget || !out.Target.Cracked {
		return model.ScreenPositionTarget{}, false
	}
	target := screenPositionFromProtocolTarget(out.Target)
	clearUncrackedDIDFallbackCoordinates(&target)
	return target, true
}

func (d *mqttO3PlusO4Decoder) didDecoder() *diddecrypt.Decoder {
	d.decoderMu.Lock()
	defer d.decoderMu.Unlock()
	if d.decoder == nil {
		d.decoder = diddecrypt.NewDecoder(d, diddecrypt.Options{
			RequireDecodedCoordinate: true,
		})
	}
	return d.decoder
}

func (d *mqttO3PlusO4Decoder) Decrypt(
	ctx context.Context,
	req diddecrypt.Request,
) (diddecrypt.DecryptResult, error) {
	resp, err := d.publishAndWait(ctx, req.DecryptedHex, req.DeviceSN)
	if err != nil {
		return diddecrypt.DecryptResult{}, err
	}
	if resp == nil {
		return diddecrypt.DecryptResult{}, fmt.Errorf("empty MQTT decrypt response")
	}
	if !resp.Success {
		return diddecrypt.DecryptResult{}, fmt.Errorf("MQTT decrypt failed: %s", resp.Message)
	}
	return resp.Data, nil
}

func (d *mqttO3PlusO4Decoder) SendKeyPacket(
	ctx context.Context,
	req diddecrypt.Request,
) diddecrypt.KeyResult {
	resp, err := d.publishAndWait(ctx, req.DecryptedHex, req.DeviceSN)
	if err != nil {
		return diddecrypt.KeyResult{Err: err}
	}
	if resp == nil {
		return diddecrypt.KeyResult{Err: fmt.Errorf("empty MQTT keygen response")}
	}
	msg := strings.TrimSpace(resp.Data.Msg)
	return diddecrypt.KeyResult{
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

func screenPositionFromProtocolTarget(target protocolmodel.PositionTarget) model.ScreenPositionTarget {
	return model.ScreenPositionTarget{
		CorrelationID:    target.CorrelationID,
		Serial:           target.Serial,
		Model:            target.Model,
		Source:           string(target.Source),
		Frequency:        target.Frequency,
		RSSI:             target.RSSI,
		Device:           target.Device,
		Drone:            screenPointFromProtocolPoint(target.Drone),
		Pilot:            screenPointFromProtocolPoint(target.Pilot),
		Home:             screenPointFromProtocolPoint(target.Home),
		Height:           cloneProtocolFloat64Ptr(target.Height),
		Altitude:         cloneProtocolFloat64Ptr(target.Altitude),
		Speed:            cloneProtocolFloat64Ptr(target.Speed),
		TrajectorySpeed:  cloneProtocolFloat64Ptr(target.TrajectorySpeed),
		TrajectoryHeight: cloneProtocolFloat64Ptr(target.TrajectoryHeight),
		Cracked:          target.Cracked,
		FirstSeen:        target.FirstSeen,
		LastSeen:         target.LastSeen,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       string(target.Source),
			ReceivedAt: target.LastSeen,
			Device:     target.Device,
			Serial:     target.Serial,
			Model:      target.Model,
			Frequency:  target.Frequency,
			RSSI:       target.RSSI,
			Cracked:    target.Cracked,
		},
	}
}

func clearUncrackedDIDFallbackCoordinates(target *model.ScreenPositionTarget) {
	if target == nil || target.Cracked || target.Model != diddecrypt.FallbackModel {
		return
	}
	target.Drone = nil
	target.Pilot = nil
	target.Home = nil
	target.DroneTrajectory = nil
	target.PilotTrajectory = nil
	target.TrajectorySpeed = nil
	target.TrajectoryHeight = nil
}

func screenPointFromProtocolPoint(point *protocolmodel.Point) *model.ScreenPositionPoint {
	if point == nil {
		return nil
	}
	return &model.ScreenPositionPoint{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
	}
}

func cloneProtocolFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
