// Package protocol implements the GNSS signal source serial protocol.
package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	HeaderFirst  byte = 0xEB
	HeaderSecond byte = 0x90

	HostAddress   byte = 0x63
	DeviceAddress byte = 0x67
	ReservedByte  byte = 0xFF
)

// ControlWord is the protocol message control byte.
type ControlWord byte

const (
	ControlSet         ControlWord = 0xA0
	ControlAck         ControlWord = 0xB0
	ControlQuery       ControlWord = 0xC0
	ControlReport      ControlWord = 0xD0
	ControlDataSend    ControlWord = 0xE0
	ControlDataConfirm ControlWord = 0xF0
)

var (
	ErrFrameTooShort = errors.New("frame too short")
	ErrInvalidHeader = errors.New("invalid frame header")
	ErrInvalidLength = errors.New("invalid frame length")
	ErrChecksum      = errors.New("checksum mismatch")
	ErrBodyTooLarge  = errors.New("frame body too large")
)

// Frame is one decoded protocol frame.
type Frame struct {
	Control  ControlWord
	Source   byte
	Target   byte
	Body     []byte
	Checksum byte
}

// ParsedFrame keeps the decoded frame together with its raw bytes.
type ParsedFrame struct {
	Frame Frame
	Raw   []byte
}

// Command returns the first byte of the body, which is the command code in this protocol.
func (f Frame) Command() byte {
	if len(f.Body) == 0 {
		return 0
	}
	return f.Body[0]
}

// BuildFrame builds one binary frame. Length includes header and checksum.
func BuildFrame(control ControlWord, source byte, target byte, body []byte) ([]byte, error) {
	length := 7 + len(body)
	if length > 0xFF {
		return nil, fmt.Errorf("%w: %d bytes", ErrBodyTooLarge, len(body))
	}

	frame := make([]byte, length)
	frame[0] = HeaderFirst
	frame[1] = HeaderSecond
	frame[2] = byte(length)
	frame[3] = byte(control)
	frame[4] = source
	frame[5] = target
	copy(frame[6:], body)
	frame[length-1] = Checksum(frame[:length-1])
	return frame, nil
}

// ParseFrame decodes and validates one complete frame.
func ParseFrame(data []byte) (Frame, error) {
	if len(data) < 7 {
		return Frame{}, ErrFrameTooShort
	}
	if data[0] != HeaderFirst || data[1] != HeaderSecond {
		return Frame{}, ErrInvalidHeader
	}
	if int(data[2]) != len(data) {
		return Frame{}, fmt.Errorf("%w: declared %d actual %d", ErrInvalidLength, data[2], len(data))
	}
	expected := Checksum(data[:len(data)-1])
	if data[len(data)-1] != expected {
		return Frame{}, fmt.Errorf("%w: got 0x%02X want 0x%02X", ErrChecksum, data[len(data)-1], expected)
	}
	body := append([]byte(nil), data[6:len(data)-1]...)
	return Frame{
		Control:  ControlWord(data[3]),
		Source:   data[4],
		Target:   data[5],
		Body:     body,
		Checksum: data[len(data)-1],
	}, nil
}

// Checksum returns the low 8 bits of the byte sum.
func Checksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// Scanner extracts frames from a streaming byte sequence and resynchronizes on EB 90.
type Scanner struct {
	buffer []byte
}

// Push appends bytes and returns any complete frames found.
func (s *Scanner) Push(chunk []byte) ([]ParsedFrame, []error) {
	if len(chunk) > 0 {
		s.buffer = append(s.buffer, chunk...)
	}

	var frames []ParsedFrame
	var errs []error
	header := []byte{HeaderFirst, HeaderSecond}

	for {
		idx := bytes.Index(s.buffer, header)
		if idx < 0 {
			if len(s.buffer) > 0 && s.buffer[len(s.buffer)-1] == HeaderFirst {
				s.buffer = s.buffer[len(s.buffer)-1:]
			} else {
				s.buffer = s.buffer[:0]
			}
			return frames, errs
		}
		if idx > 0 {
			s.buffer = s.buffer[idx:]
		}
		if len(s.buffer) < 3 {
			return frames, errs
		}

		length := int(s.buffer[2])
		if length < 7 {
			errs = append(errs, fmt.Errorf("%w: declared %d", ErrInvalidLength, length))
			s.buffer = s.buffer[1:]
			continue
		}
		if len(s.buffer) < length {
			return frames, errs
		}

		raw := append([]byte(nil), s.buffer[:length]...)
		frame, err := ParseFrame(raw)
		if err != nil {
			errs = append(errs, err)
			s.buffer = s.buffer[1:]
			continue
		}

		frames = append(frames, ParsedFrame{Frame: frame, Raw: raw})
		s.buffer = s.buffer[length:]
	}
}

func appendUint16(buf []byte, value uint16) []byte {
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], value)
	return append(buf, tmp[:]...)
}

func appendUint32(buf []byte, value uint32) []byte {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], value)
	return append(buf, tmp[:]...)
}

func appendInt32(buf []byte, value int32) []byte {
	return appendUint32(buf, uint32(value))
}
