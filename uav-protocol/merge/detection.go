// Package merge contains pure target matching rules shared by services.
package merge

import (
	"math"
	"strings"
)

const DefaultDetectionThresholdMHz = 15.0

type DetectionOptions struct {
	BaseThresholdMHz float64
}

func (o DetectionOptions) threshold() float64 {
	if o.BaseThresholdMHz > 0 {
		return o.BaseThresholdMHz
	}
	return DefaultDetectionThresholdMHz
}

func NormalizeDetectionModel(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	prefix, suffix, ok := strings.Cut(modelName, "-")
	if !ok || strings.TrimSpace(suffix) == "" || !isDecimalString(strings.TrimSpace(prefix)) {
		return modelName
	}
	return strings.TrimSpace(suffix)
}

func DetectionMatches(targetModel string, targetFreq float64, recordModel string, recordFreq float64, opts DetectionOptions) bool {
	targetModel = NormalizeDetectionModel(targetModel)
	recordModel = NormalizeDetectionModel(recordModel)
	if targetModel == "" || recordModel == "" {
		return false
	}
	base := opts.threshold()
	freqDiff := math.Abs(targetFreq - recordFreq)
	switch {
	case IsAutelModel(targetModel) || IsAutelModel(recordModel):
		return freqDiff <= base+25 && (targetModel == recordModel || (IsAutelModel(targetModel) && IsAutelModel(recordModel)))
	case targetModel == "O3+_ofdm_datalink" || recordModel == "O3+_ofdm_datalink":
		return freqDiff <= base+5 && targetModel == recordModel
	default:
		return freqDiff <= base && (targetModel == recordModel || (IsDJIModel(targetModel) && IsDJIModel(recordModel)))
	}
}

func IsAutelModel(model string) bool {
	switch model {
	case "Autel_type1", "Autel_type2", "Autel_type3", "Autel_type4", "Autel_type5":
		return true
	default:
		return false
	}
}

func IsDJIModel(model string) bool {
	return model == "DJI_OC123_10M" || model == "DJI_OC123_20M"
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
