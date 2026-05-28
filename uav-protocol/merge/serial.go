package merge

import (
	"strings"

	"uav-protocol/model"
)

const ridSerialPrefix = "1581"

func SerialMatches(existing, incoming string) bool {
	existingRaw := strings.ToUpper(strings.TrimSpace(existing))
	incomingRaw := strings.ToUpper(strings.TrimSpace(incoming))
	existing = CanonicalSerial(existingRaw)
	incoming = CanonicalSerial(incomingRaw)
	if existing == "" || incoming == "" {
		return false
	}
	if existing == incoming {
		return true
	}
	if TrimRIDSerialPrefix(existing) == incoming {
		return true
	}
	if TrimRIDSerialPrefix(incoming) == existing {
		return true
	}
	return serialSuffixMatches(existing, incoming, existingRaw, incomingRaw)
}

func CanonicalSerial(serial string) string {
	var builder strings.Builder
	builder.Grow(len(serial))
	for _, r := range strings.ToUpper(strings.TrimSpace(serial)) {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func TrimRIDSerialPrefix(serial string) string {
	if len(serial) <= len(ridSerialPrefix) || !strings.HasPrefix(serial, ridSerialPrefix) {
		return serial
	}
	return strings.TrimPrefix(serial, ridSerialPrefix)
}

func DIDEncryptedCorrelationID(encryptedID string) string {
	encryptedID = strings.ToLower(strings.TrimSpace(encryptedID))
	if encryptedID == "" {
		return ""
	}
	return "did_encrypted:" + encryptedID
}

func PositionMatches(existing, incoming model.PositionTarget) bool {
	if SerialMatches(existing.Serial, incoming.Serial) {
		return true
	}
	return pendingEncryptedTargetMatches(existing, incoming)
}

func ShouldKeepDecodedPositionFields(existing, incoming model.PositionTarget) bool {
	if !existing.Cracked || incoming.Cracked {
		return false
	}
	existingCorrelationID := strings.TrimSpace(existing.CorrelationID)
	incomingCorrelationID := strings.TrimSpace(incoming.CorrelationID)
	if existingCorrelationID != "" && incomingCorrelationID != "" && existingCorrelationID == incomingCorrelationID {
		return true
	}
	return SerialMatches(existing.Serial, incoming.Serial)
}

func pendingEncryptedTargetMatches(existing, incoming model.PositionTarget) bool {
	existingCorrelationID := strings.TrimSpace(existing.CorrelationID)
	incomingCorrelationID := strings.TrimSpace(incoming.CorrelationID)
	return existingCorrelationID != "" &&
		incomingCorrelationID != "" &&
		existingCorrelationID == incomingCorrelationID &&
		(isPendingEncrypted(existing) || isPendingEncrypted(incoming))
}

func isPendingEncrypted(target model.PositionTarget) bool {
	if target.Cracked || strings.TrimSpace(target.CorrelationID) == "" {
		return false
	}
	return strings.TrimPrefix(strings.TrimSpace(target.CorrelationID), "did_encrypted:") == strings.ToLower(strings.TrimSpace(target.Serial))
}

func serialSuffixMatches(existing, incoming, existingRaw, incomingRaw string) bool {
	const minSuffixLength = 10
	shorter, longer := existing, incoming
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	commonSuffixLength := commonSuffixLength(existing, incoming)
	if commonSuffixLength < minSuffixLength {
		return false
	}
	if serialHasCorruptedPrefix(existingRaw, existing, commonSuffixLength) ||
		serialHasCorruptedPrefix(incomingRaw, incoming, commonSuffixLength) {
		return true
	}
	return len(shorter) == commonSuffixLength && len(longer)-len(shorter) >= 4
}

func serialHasCorruptedPrefix(raw, canonical string, suffixLength int) bool {
	if suffixLength >= len(canonical) {
		return false
	}
	if len(canonical)-suffixLength > 3 {
		return false
	}
	for _, r := range raw {
		return !serialRuneIsCanonical(r)
	}
	return false
}

func serialRuneIsCanonical(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z')
}

func commonSuffixLength(left, right string) int {
	count := 0
	for count < len(left) && count < len(right) {
		if left[len(left)-1-count] != right[len(right)-1-count] {
			break
		}
		count++
	}
	return count
}
