package protocol

import (
	"fmt"
	"math"
	"time"
)

const (
	CmdSimulatedPosition byte = 0x51
	CmdTransmitSwitch    byte = 0x52
	CmdPowerAttenuation  byte = 0x53
	CmdSystemTime        byte = 0x54
	CmdDeviceReboot      byte = 0x56
	CmdInitialVelocity   byte = 0x57
	CmdAcceleration      byte = 0x58
	CmdSimulatedCircle   byte = 0x59
	CmdTrackImport       byte = 0x5A
	CmdMaxSpeed          byte = 0x5B
	CmdDevicePosition    byte = 0x5C
	CmdTargetPosition    byte = 0x5D
	CmdCoordinateControl byte = 0x5E
	CmdNoFlyZone         byte = 0x5F
	CmdSpoofCircle       byte = 0x60
	CmdRandomPosition    byte = 0x64
	CmdSignalDelay       byte = 0x65
)

const (
	QuerySimulatedPosition byte = 0x51
	QueryTransmitSwitch    byte = 0x52
	QueryDeviceStatus      byte = 0x53
	QueryFirmwareVersion   byte = 0x54
	QuerySystemTime        byte = 0x55
	QueryPowerAttenuation  byte = 0x56
	QueryTargetPosition    byte = 0x57
	QueryNoFlyZone         byte = 0x58
	QuerySpoofCircle       byte = 0x59
	QueryDeviceSignal      byte = 0x5D
	QueryDevicePosition    byte = 0x5E
	QueryRandomPosition    byte = 0x5F
	QuerySignalDelay       byte = 0x60
)

const (
	SignalGPSL1CA uint16 = 1 << 0
	SignalBDSB1I  uint16 = 1 << 1
	SignalGLOL1   uint16 = 1 << 2
	SignalGALE1   uint16 = 1 << 6

	SignalAllSupported = SignalGPSL1CA | SignalBDSB1I | SignalGLOL1 | SignalGALE1
)

// BuildQuery builds a C0 query frame.
func BuildQuery(command byte) ([]byte, error) {
	return BuildFrame(ControlQuery, HostAddress, DeviceAddress, []byte{command, ReservedByte})
}

// BuildSetTransmitSwitch builds the A0/0x52 transmit switch command.
func BuildSetTransmitSwitch(mask uint16) ([]byte, error) {
	body := []byte{CmdTransmitSwitch, ReservedByte}
	body = appendUint16(body, mask)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

// BuildSetPowerAttenuation builds A0/0x53. attenuationDB must be in 0..80.
func BuildSetPowerAttenuation(mask uint16, attenuationDB byte) ([]byte, error) {
	if attenuationDB > 80 {
		return nil, fmt.Errorf("功率衰减值必须在 0~80 dB 之间")
	}
	body := []byte{CmdPowerAttenuation, ReservedByte}
	body = appendUint16(body, mask)
	body = append(body, attenuationDB)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

// BuildSetSystemTime builds A0/0x54 using UTC time.
func BuildSetSystemTime(t time.Time) ([]byte, error) {
	t = t.UTC()
	year := t.Year() - 2000
	if year < 0 || year > 255 {
		return nil, fmt.Errorf("年份超出协议范围: %d", t.Year())
	}
	body := []byte{
		CmdSystemTime,
		byte(year),
		byte(t.Month()),
		byte(t.Day()),
		byte(t.Hour()),
		byte(t.Minute()),
		byte(t.Second()),
	}
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

// BuildReboot builds A0/0x56 with verification key 0xDB55.
func BuildReboot() ([]byte, error) {
	body := []byte{CmdDeviceReboot}
	body = appendUint16(body, 0xDB55)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetSimulatedPosition(longitude float64, latitude float64, altitudeM int32) ([]byte, error) {
	if err := validateLocation(longitude, latitude); err != nil {
		return nil, err
	}
	body := []byte{CmdSimulatedPosition, ReservedByte}
	body = appendFloat64(body, longitude)
	body = appendFloat64(body, latitude)
	body = appendInt32(body, altitudeM)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetDevicePosition(longitude float64, latitude float64, altitudeM int32) ([]byte, error) {
	if err := validateLocation(longitude, latitude); err != nil {
		return nil, err
	}
	body := []byte{CmdDevicePosition, ReservedByte}
	body = appendFloat64(body, longitude)
	body = appendFloat64(body, latitude)
	body = appendInt32(body, altitudeM)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetInitialVelocity(speedMPS float32, directionDeg float32) ([]byte, error) {
	body := []byte{CmdInitialVelocity, ReservedByte}
	body = appendFloat32(body, speedMPS)
	body = appendFloat32(body, directionDeg)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetAcceleration(accelerationMPS2 float32, directionDeg float32) ([]byte, error) {
	body := []byte{CmdAcceleration, ReservedByte}
	body = appendFloat32(body, accelerationMPS2)
	body = appendFloat32(body, directionDeg)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetSimulatedCircle(radiusM float32, periodS float32, direction int32) ([]byte, error) {
	body := []byte{CmdSimulatedCircle, ReservedByte}
	body = appendFloat32(body, radiusM)
	body = appendFloat32(body, periodS)
	body = appendInt32(body, direction)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetMaxSpeed(speedMPS float32) ([]byte, error) {
	body := []byte{CmdMaxSpeed, ReservedByte}
	body = appendFloat32(body, speedMPS)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetTargetPosition(distanceM int32, heightM int32, directionDeg float32, headingDeg float32) ([]byte, error) {
	body := []byte{CmdTargetPosition, ReservedByte}
	body = appendInt32(body, distanceM)
	body = appendInt32(body, heightM)
	body = appendFloat32(body, directionDeg)
	body = appendFloat32(body, headingDeg)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetCoordinateControl(horizontalStepM int32, horizontalDirection int32, verticalStepM int32, verticalDirection int32, durationS int32) ([]byte, error) {
	body := []byte{CmdCoordinateControl, ReservedByte}
	body = appendInt32(body, horizontalStepM)
	body = appendInt32(body, horizontalDirection)
	body = appendInt32(body, verticalStepM)
	body = appendInt32(body, verticalDirection)
	body = appendInt32(body, durationS)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetSpoofCircle(distanceM int32, heightM int32, directionDeg float32, headingDeg float32, radiusM float32, periodS float32, rotateDirection int32) ([]byte, error) {
	body := []byte{CmdSpoofCircle, ReservedByte}
	body = appendInt32(body, distanceM)
	body = appendInt32(body, heightM)
	body = appendFloat32(body, directionDeg)
	body = appendFloat32(body, headingDeg)
	body = appendFloat32(body, radiusM)
	body = appendFloat32(body, periodS)
	body = appendInt32(body, rotateDirection)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetRandomPosition(enabled bool, radiusM uint32, refreshPeriodS uint32) ([]byte, error) {
	var enabledValue uint32
	if enabled {
		enabledValue = 1
	}
	body := []byte{CmdRandomPosition, ReservedByte}
	body = appendUint32(body, enabledValue)
	body = appendUint32(body, radiusM)
	body = appendUint32(body, refreshPeriodS)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func BuildSetSignalDelay(mask uint16, delayNS float32) ([]byte, error) {
	body := []byte{CmdSignalDelay, ReservedByte}
	body = appendUint16(body, mask)
	body = appendFloat32(body, delayNS)
	return BuildFrame(ControlSet, HostAddress, DeviceAddress, body)
}

func appendFloat32(buf []byte, value float32) []byte {
	return appendUint32(buf, math.Float32bits(value))
}

func appendFloat64(buf []byte, value float64) []byte {
	lo := math.Float64bits(value)
	var tmp [8]byte
	for i := 0; i < 8; i++ {
		tmp[i] = byte(lo >> (8 * i))
	}
	return append(buf, tmp[:]...)
}

func validateLocation(longitude float64, latitude float64) error {
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("经度必须在 -180~180 之间")
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("纬度必须在 -90~90 之间")
	}
	return nil
}
