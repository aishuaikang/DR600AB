package model

import "strings"

var uavModelDisplayNames = map[string]string{
	"Lightbridge_type1": "DJI P4/Spark",
	"Lightbridge_type2": "Autel Evo2V2/Feimi",
	"eWifi_5M":          "DJI Mini1/Air1",
	"eWifi_10M":         "HUBSAN Zino/HUBSAN Eagle",
	"PAL Analog":        "Analog PAL",
	"NTSC Analog":       "Analog NTSC",
	"DJI_OC123_10M":     "DJI-O1/O2/O3 Series Mini 2,3, 4k, Air 2, 3, Mavic 2,3, Avata, P4-2.0",
	"DJI_OC123_20M":     "DJI-O1/O2/O3 Series Mini 2,3, 4k,Air 2, 3, Mavic 2,3, Avata, P4-2.0",
	"DJI_O4_type":       "DJI-O4 Series Mini4, Air3s, Avata2, Neo",
	"Autel_type1":       "Autel nano/nano+/ lite/lite+",
	"Autel_type2":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Autel_type3":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Autel_type4":       "Autel Evo2_V3, MAX4T, lite, lite+",
	"Datalink_type1":    "P900/P840/Mavlink",
	"Datalink_type2":    "P900/P840/Mavlink",
	"Datalink_type3":    "DJI/Autel remote",
	"LTE_type0":         "LTE-image feed",
	"LORA":              "LoRa drone",
	"Walksnail":         "Walksnail drone",
	"DJI_O3+":           "Mavic 3 series",
	"O3+_ofdm_datalink": "Drone/RC",
}

// DisplayModelName returns the friendly model label for user-facing views.
// Unknown values fall back to a normalized original model string.
func DisplayModelName(modelName string) string {
	modelName = normalizeDisplayModelName(modelName)
	if modelName == "" {
		return ""
	}
	if displayName, ok := uavModelDisplayNames[modelName]; ok {
		return displayName
	}
	return modelName
}

// IsUncrackedDJIDroneModel reports whether modelName is the encrypted DID placeholder.
func IsUncrackedDJIDroneModel(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), "DJI-Drone")
}

// IsUncrackedDJIDronePosition reports whether target is the encrypted DID placeholder target.
func IsUncrackedDJIDronePosition(target ScreenPositionTarget) bool {
	return !target.Cracked && IsUncrackedDJIDroneModel(target.Model)
}

func normalizeDisplayModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	prefix, suffix, ok := strings.Cut(modelName, "-")
	if !ok {
		return modelName
	}
	prefix = strings.TrimSpace(prefix)
	suffix = strings.TrimSpace(suffix)
	if prefix == "" || suffix == "" || !isDecimalString(prefix) {
		return modelName
	}
	return suffix
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
