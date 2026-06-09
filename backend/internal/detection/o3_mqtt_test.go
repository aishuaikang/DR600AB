package detection

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"uav-protocol/diddecrypt"
	"uav-protocol/parser"
)

type failingO3DecryptClient struct{}

func (failingO3DecryptClient) Decrypt(context.Context, diddecrypt.Request) (diddecrypt.DecryptResult, error) {
	return diddecrypt.DecryptResult{}, errors.New("timeout")
}

func (failingO3DecryptClient) SendKeyPacket(context.Context, diddecrypt.Request) diddecrypt.KeyResult {
	return diddecrypt.KeyResult{}
}

func TestMQTTO3PlusO4DecoderSkipsUncrackedDJIDrone(t *testing.T) {
	receivedAt := time.Unix(1_700_000_000, 0)
	packet := parser.DIDEncrypted{
		Device:      "device-a",
		EncryptedID: "447e5681",
		Freq:        5776.5,
		RSSI:        -83,
		Bytes:       "8710494e4650447e5681" + strings.Repeat("00", 163) + "a163b7",
	}
	decoder := &mqttO3PlusO4Decoder{}
	decoder.decoder = diddecrypt.NewDecoder(failingO3DecryptClient{}, diddecrypt.Options{
		RequireDecodedCoordinate: true,
	})

	if target, ok := decoder.ParseO3PlusO4PacketMQTT(context.Background(), packet, "device-a", receivedAt); ok {
		t.Fatalf("ParseO3PlusO4PacketMQTT() = %+v, true; want no target for uncracked packet", target)
	}
}
