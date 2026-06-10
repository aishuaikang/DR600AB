// Package model defines pure protocol data structures shared by parsers,
// decryptors, and merge rules.
package model

import "time"

type MessageType string

const (
	TypeUnknown      MessageType = "unknown"
	TypeDIDEncrypted MessageType = "did_encrypted"
	TypeRID          MessageType = "rid"
	TypeDIDPlain     MessageType = "did_plain"
	TypeDetect       MessageType = "detect"
	TypeHeartbeat    MessageType = "heartbeat"
	TypeEmpty        MessageType = "empty"
	TypeSpectrum     MessageType = "spectrum"
)

type Message struct {
	Type MessageType `json:"type"`
	Time time.Time   `json:"time"`
	Raw  string      `json:"raw"`
	Data any         `json:"data"`
}

type GPS struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type DIDEncrypted struct {
	Device      string  `json:"device"`
	EncryptedID string  `json:"encrypted_id"`
	Freq        float64 `json:"freq"`
	RSSI        float64 `json:"rssi"`
	Bytes       string  `json:"bytes"`
}

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
	NorthV   float64 `json:"north_v"`
	UpV      float64 `json:"up_v"`
	Freq     float64 `json:"freq"`
	RSSI     float64 `json:"rssi"`
	Distance string  `json:"distance"`
}

type Detect struct {
	Device    string  `json:"device"`
	Model     string  `json:"model"`
	Freq      float64 `json:"freq"`
	RSSI      float64 `json:"rssi"`
	Bandwidth float64 `json:"bandwidth,omitempty"`
	Seq       int64   `json:"seq,omitempty"`
	GPIO      int64   `json:"gpio,omitempty"`
}

type Heartbeat struct {
	Device string `json:"device"`
	Seq    string `json:"seq"`
}

type Empty struct {
	Freq float64 `json:"freq"`
	RSSI float64 `json:"rssi"`
}

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type TrackPoint struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Speed     *float64  `json:"speed,omitempty"`
	Height    *float64  `json:"height,omitempty"`
	Time      time.Time `json:"time"`
}

type PositionTarget struct {
	CorrelationID    string
	Serial           string
	Model            string
	Source           MessageType
	Frequency        float64
	RSSI             float64
	Device           string
	Drone            *Point
	Pilot            *Point
	Home             *Point
	Height           *float64
	Altitude         *float64
	Speed            *float64
	TrajectorySpeed  *float64
	TrajectoryHeight *float64
	Cracked          bool
	FirstSeen        time.Time
	LastSeen         time.Time
}
