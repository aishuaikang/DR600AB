// Package spectrum parses and assembles detector spectrum frames.
package spectrum

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"time"
)

const (
	DefaultFreqStepMHz = 0.5
	headerLen          = 4
	pointByteSize      = 2
)

type Frame struct {
	CenterFreqHz int64
	Values       []int16
	FFTSize      int
}

type Snapshot struct {
	MinFreqMHz float64
	MaxFreqMHz float64
	FreqsMHz   []float64
	Values     []int
}

type ApplyResult struct {
	Updated       bool
	Reason        string
	CenterFreqMHz float64
	Index         int
	UpdatedPoints int
	MinFreqMHz    float64
	MaxFreqMHz    float64
}

func IsFrame(data []byte) bool {
	if len(data) <= headerLen || (len(data)-headerLen)%pointByteSize != 0 {
		return false
	}
	frequencyHz, ok := ParseFrequencyHeaderHz(data)
	return ok && validFrequencyHz(frequencyHz)
}

func ParseFrequencyHeaderHz(data []byte) (int64, bool) {
	if len(data) < headerLen {
		return 0, false
	}
	freqVal := binary.BigEndian.Uint32(data[:headerLen])
	switch {
	case freqVal == 0:
		return 0, false
	case freqVal <= 10000:
		return int64(freqVal) * 1_000_000, true
	default:
		return int64(freqVal) * 1_000, true
	}
}

func ParseFrame(data []byte) (Frame, bool) {
	centerFreq, ok := ParseFrequencyHeaderHz(data)
	if !ok || !validFrequencyHz(centerFreq) || len(data) < headerLen || (len(data)-headerLen)%pointByteSize != 0 {
		return Frame{}, false
	}
	fftSize := (len(data) - headerLen) / pointByteSize
	values := make([]int16, fftSize)
	for i := 0; i < fftSize; i++ {
		raw := int16(binary.BigEndian.Uint16(data[headerLen+pointByteSize*i : headerLen+pointByteSize*(i+1)]))
		values[i] = raw/100 - 180
	}
	return Frame{CenterFreqHz: centerFreq, Values: values, FFTSize: fftSize}, true
}

func validFrequencyHz(frequencyHz int64) bool {
	frequencyMHz := frequencyHz / 1_000_000
	return frequencyMHz >= 1 && frequencyMHz <= 10000
}

func NewSnapshot(minFreqMHz, maxFreqMHz float64, stepMHz float64) Snapshot {
	if stepMHz <= 0 {
		stepMHz = DefaultFreqStepMHz
	}
	pointCount := (maxFreqMHz - minFreqMHz) / stepMHz
	numPoints := int(math.Floor(pointCount)) + 1
	if numPoints < 0 {
		numPoints = 0
	}
	freqs := make([]float64, numPoints)
	values := make([]int, numPoints)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < numPoints; i++ {
		freqs[i] = math.Round((minFreqMHz+float64(i)*stepMHz)*10) / 10
		values[i] = -100 + r.Intn(11)
	}
	if numPoints > 0 && freqs[numPoints-1] < maxFreqMHz {
		freqs = append(freqs, math.Round(maxFreqMHz*10)/10)
		values = append(values, -100+r.Intn(11))
	}
	return Snapshot{MinFreqMHz: minFreqMHz, MaxFreqMHz: maxFreqMHz, FreqsMHz: freqs, Values: values}
}

func ApplyFrame(snapshot Snapshot, frame Frame, stepMHz float64) (Snapshot, ApplyResult) {
	if stepMHz <= 0 {
		stepMHz = DefaultFreqStepMHz
	}
	result := ApplyResult{
		CenterFreqMHz: float64(frame.CenterFreqHz) / 1e6,
		Index:         -1,
		MinFreqMHz:    snapshot.MinFreqMHz,
		MaxFreqMHz:    snapshot.MaxFreqMHz,
	}
	centerFreqMHz := result.CenterFreqMHz
	if len(snapshot.Values) == 0 {
		result.Reason = "spectrum snapshot not initialized"
		return snapshot, result
	}
	if centerFreqMHz < snapshot.MinFreqMHz || centerFreqMHz > snapshot.MaxFreqMHz {
		result.Reason = "frequency out of initialized range"
		return snapshot, result
	}
	index := int(math.Round((centerFreqMHz - snapshot.MinFreqMHz) / stepMHz))
	result.Index = index
	if index < 0 || index >= len(snapshot.Values) {
		result.Reason = "calculated index out of range"
		return snapshot, result
	}
	next := cloneSnapshot(snapshot)
	if index < len(next.FreqsMHz) {
		next.FreqsMHz[index] = centerFreqMHz
	}
	for i := 0; i < frame.FFTSize && i < len(frame.Values); i++ {
		targetIndex := index + i
		if targetIndex < len(next.Values) {
			next.Values[targetIndex] = int(frame.Values[i])
			result.UpdatedPoints++
		}
	}
	if result.UpdatedPoints == 0 {
		result.Reason = "no spectrum points updated"
		return snapshot, result
	}
	result.Updated = true
	return next, result
}

func BuildAnalysisCommand(fStart, fStop int) string {
	return fmt.Sprintf("start -fft 64 -band %d,%d, -gain 40\n", fStart, fStop)
}

func BuildStopCommand() string {
	return "start -fft 0,\n"
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	next := snapshot
	next.FreqsMHz = append([]float64(nil), snapshot.FreqsMHz...)
	next.Values = append([]int(nil), snapshot.Values...)
	return next
}
