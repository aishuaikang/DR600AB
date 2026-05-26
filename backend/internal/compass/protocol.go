package compass

import (
	"errors"
	"fmt"
	"math"
)

const (
	frameStart byte = 0x77

	commandReadHeading           byte = 0x03
	commandReadHeadingResp       byte = 0x83
	commandReadPitchRollHeading  byte = 0x04
	commandPitchRollHeadingResp  byte = 0x84
	commandSetAutoOutputRate     byte = 0x0C
	commandSetAutoOutputRateResp byte = 0x8C

	defaultDeviceAddr byte = 0x00
	maxFrameLength    byte = 32
)

const (
	autoOutputRateReplyMode byte = 0x00
	autoOutputRate5Hz       byte = 0x01
	autoOutputRate10Hz      byte = 0x02
	autoOutputRate15Hz      byte = 0x03
	autoOutputRate25Hz      byte = 0x04
	autoOutputRate50Hz      byte = 0x05
)

var (
	errDataTooShort   = errors.New("data too short")
	errInvalidHeader  = errors.New("invalid frame header")
	errInvalidLength  = errors.New("invalid frame length")
	errChecksumFailed = errors.New("checksum verification failed")
	errInvalidCommand = errors.New("invalid command code")
	errInvalidAngle   = errors.New("invalid angle data")
	errInvalidRate    = errors.New("invalid auto output rate")
	errInvalidStatus  = errors.New("invalid status data")
)

type frame struct {
	start    byte
	length   byte
	address  byte
	command  byte
	data     []byte
	checksum byte
}

type headingResponse struct {
	address byte
	heading float64
	rawData []byte
}

type pitchRollHeadingResponse struct {
	address byte
	pitch   float64
	roll    float64
	heading float64
	rawData []byte
}

type setAutoOutputRateResponse struct {
	address byte
	status  byte
	success bool
}

func buildReadHeadingCmd(addr byte) []byte {
	return buildFrame(normalizeAddr(addr), commandReadHeading, nil)
}

func buildReadPitchRollHeadingCmd(addr byte) []byte {
	return buildFrame(normalizeAddr(addr), commandReadPitchRollHeading, nil)
}

func buildSetAutoOutputRateCmd(addr byte, rate byte) ([]byte, error) {
	if !isValidAutoOutputRate(rate) {
		return nil, errInvalidRate
	}
	return buildFrame(normalizeAddr(addr), commandSetAutoOutputRate, []byte{rate}), nil
}

func isValidAutoOutputRate(rate byte) bool {
	return rate >= autoOutputRateReplyMode && rate <= autoOutputRate50Hz
}

func parseFrame(data []byte) (*frame, error) {
	if len(data) < 5 {
		return nil, errDataTooShort
	}
	if data[0] != frameStart {
		return nil, errInvalidHeader
	}

	length := int(data[1])
	if len(data) != length+1 || length < 4 {
		return nil, errInvalidLength
	}
	checksum := calcChecksum(data[1 : len(data)-1])
	if checksum != data[len(data)-1] {
		return nil, errChecksumFailed
	}

	parsed := &frame{
		start:    data[0],
		length:   data[1],
		address:  data[2],
		command:  data[3],
		checksum: data[len(data)-1],
	}
	if len(data) > 5 {
		parsed.data = append([]byte(nil), data[4:len(data)-1]...)
	}
	return parsed, nil
}

func parseHeadingResponse(data []byte) (*headingResponse, error) {
	parsed, err := parseFrame(data)
	if err != nil {
		return nil, err
	}
	if parsed.command != commandReadHeadingResp {
		return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", errInvalidCommand, commandReadHeadingResp, parsed.command)
	}
	if len(parsed.data) != 3 {
		return nil, errInvalidAngle
	}

	heading, err := decodeAngleBCD(parsed.data)
	if err != nil {
		return nil, err
	}

	return &headingResponse{
		address: parsed.address,
		heading: heading,
		rawData: append([]byte(nil), parsed.data...),
	}, nil
}

func parsePitchRollHeadingResponse(data []byte) (*pitchRollHeadingResponse, error) {
	parsed, err := parseFrame(data)
	if err != nil {
		return nil, err
	}
	if parsed.command != commandPitchRollHeadingResp {
		return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", errInvalidCommand, commandPitchRollHeadingResp, parsed.command)
	}
	if len(parsed.data) != 9 {
		return nil, errInvalidAngle
	}

	pitch, err := decodeAngleBCD(parsed.data[0:3])
	if err != nil {
		return nil, err
	}
	roll, err := decodeAngleBCD(parsed.data[3:6])
	if err != nil {
		return nil, err
	}
	heading, err := decodeAngleBCD(parsed.data[6:9])
	if err != nil {
		return nil, err
	}

	return &pitchRollHeadingResponse{
		address: parsed.address,
		pitch:   pitch,
		roll:    roll,
		heading: heading,
		rawData: append([]byte(nil), parsed.data...),
	}, nil
}

func parseSetAutoOutputRateResponse(data []byte) (*setAutoOutputRateResponse, error) {
	parsed, err := parseFrame(data)
	if err != nil {
		return nil, err
	}
	if parsed.command != commandSetAutoOutputRateResp {
		return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", errInvalidCommand, commandSetAutoOutputRateResp, parsed.command)
	}
	if len(parsed.data) != 1 || (parsed.data[0] != 0x00 && parsed.data[0] != 0xFF) {
		return nil, errInvalidStatus
	}

	return &setAutoOutputRateResponse{
		address: parsed.address,
		status:  parsed.data[0],
		success: parsed.data[0] == 0x00,
	}, nil
}

func buildHeadingResponse(addr byte, heading float64) ([]byte, error) {
	data, err := encodeAngleBCD(heading)
	if err != nil {
		return nil, err
	}
	return buildFrame(normalizeAddr(addr), commandReadHeadingResp, data), nil
}

func buildPitchRollHeadingResponse(addr byte, pitch float64, roll float64, heading float64) ([]byte, error) {
	pitchData, err := encodeAngleBCD(pitch)
	if err != nil {
		return nil, err
	}
	rollData, err := encodeAngleBCD(roll)
	if err != nil {
		return nil, err
	}
	headingData, err := encodeAngleBCD(heading)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 0, 9)
	data = append(data, pitchData...)
	data = append(data, rollData...)
	data = append(data, headingData...)
	return buildFrame(normalizeAddr(addr), commandPitchRollHeadingResp, data), nil
}

func buildSetAutoOutputRateResponse(addr byte, success bool) []byte {
	status := byte(0xFF)
	if success {
		status = 0x00
	}
	return buildFrame(normalizeAddr(addr), commandSetAutoOutputRateResp, []byte{status})
}

func buildFrame(addr byte, command byte, data []byte) []byte {
	length := byte(len(data) + 4)
	out := make([]byte, 0, int(length)+1)
	out = append(out, frameStart, length, addr, command)
	out = append(out, data...)
	out = append(out, calcChecksum(out[1:]))
	return out
}

func calcChecksum(data []byte) byte {
	var sum uint16
	for _, value := range data {
		sum += uint16(value)
	}
	return byte(sum & 0xFF)
}

func encodeAngleBCD(angle float64) ([]byte, error) {
	if math.IsNaN(angle) || math.IsInf(angle, 0) {
		return nil, errInvalidAngle
	}

	negative := angle < 0
	if negative {
		angle = -angle
	}
	value := int(math.Round(angle * 100))
	if value > 99999 {
		return nil, errInvalidAngle
	}

	hundreds := (value / 10000) % 10
	tens := (value / 1000) % 10
	ones := (value / 100) % 10
	decimalTens := (value / 10) % 10
	decimalOnes := value % 10

	first := byte(hundreds & 0x0F)
	if negative {
		first |= 0x10
	}
	return []byte{
		first,
		byte((tens << 4) | ones),
		byte((decimalTens << 4) | decimalOnes),
	}, nil
}

func decodeAngleBCD(data []byte) (float64, error) {
	if len(data) != 3 {
		return 0, errInvalidAngle
	}

	sign := data[0] >> 4
	hundreds := int(data[0] & 0x0F)
	tens := int(data[1] >> 4)
	ones := int(data[1] & 0x0F)
	decimalTens := int(data[2] >> 4)
	decimalOnes := int(data[2] & 0x0F)
	if sign > 1 || hundreds > 9 || tens > 9 || ones > 9 || decimalTens > 9 || decimalOnes > 9 {
		return 0, errInvalidAngle
	}

	valueCentiDegrees := (hundreds*100+tens*10+ones)*100 + decimalTens*10 + decimalOnes
	value := float64(valueCentiDegrees) / 100
	if sign != 0 {
		value = -value
	}
	return value, nil
}

func normalizeAddr(addr byte) byte {
	if addr == 0 {
		return defaultDeviceAddr
	}
	return addr
}
