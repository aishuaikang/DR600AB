package client

import (
	"bytes"
	"regexp"
)

var (
	crlf = []byte("\r\n")

	// S1 会连续输出 detect 文本而不带换行，先用完整字段形态恢复这类记录边界。
	detectRecordPattern = regexp.MustCompile(
		`^\s*device=[^,\r\n]+,\s*model=[^,\r\n]+,\s*freq=[^,\r\n]+,\s*rssi=[+-]?(?:\d+(?:\.\d)?|\.\d)`,
	)
	ridRecordPattern = regexp.MustCompile(
		`^\s*RID\s+ssid=[^,\r\n]+,\s*serial=[^,\r\n]+,\s*(?:ver=[^,\r\n]*,\s*)?(?:name=[^,\r\n]*,\s*)?model=[^,\r\n]+,\s*UA_type=[^,\r\n]+,\s*drone_GPS=[^,\r\n]+,[^,\r\n]+,\s*pilot_GPS=[^,\r\n]+,[^,\r\n]+,\s*speed=[^,\r\n]+,\s*Vspeed=[^,\r\n]+,\s*direc=[^,\r\n]+,\s*AltitudeP=[^,\r\n]+,\s*AltitudeG=[^,\r\n]+,\s*Height_AGL=[^,\r\n]+,\s*MAC=[^,\r\n]+,\s*rssi=[+-]?(?:\d+(?:\.\d)?|\.\d),\s*freq=[+-]?(?:\d+(?:\.\d)?|\.\d)`,
	)
	numericDIDPlainRecordPattern = regexp.MustCompile(`^\s*\d+,\s*serial=[^,\r\n]+,\s*(?:model=[^,\r\n]+,\s*)?uuid=`)
	recordBoundaries             = [][]byte{
		[]byte("device="),
		[]byte("#="),
		[]byte("num="),
		[]byte("RID "),
		[]byte("com #="),
	}
)

func scanSerialRecords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) == 0 {
		return 0, nil, nil
	}

	separatorIndex, separatorLen := firstRecordSeparator(data)
	searchLimit := len(data)
	if separatorIndex >= 0 {
		searchLimit = separatorIndex
	}

	if loc := detectRecordPattern.FindIndex(data); loc != nil && loc[0] == 0 {
		switch {
		case separatorIndex >= 0 && loc[1] < separatorIndex:
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		case atEOF && separatorIndex == -1:
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		case loc[1] < len(data) && beginsRecordBoundary(data[loc[1]:]):
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		}
	}

	if beginsRID(data) {
		if next := nextRecordBoundary(data[:searchLimit], len("RID ")); next > 0 {
			return next, bytes.TrimSpace(data[:next]), nil
		}
	}

	if loc := ridRecordPattern.FindIndex(data); loc != nil && loc[0] == 0 {
		switch {
		case separatorIndex >= 0 && loc[1] < separatorIndex:
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		case atEOF && separatorIndex == -1:
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		case loc[1] < len(data) && beginsRecordBoundary(data[loc[1]:]):
			return loc[1], bytes.TrimSpace(data[:loc[1]]), nil
		}
	}

	if next := nextCompleteDetectStart(data[:searchLimit]); next > 0 {
		return next, bytes.TrimSpace(data[:next]), nil
	}

	if separatorIndex >= 0 {
		return separatorIndex + separatorLen, bytes.TrimSpace(data[:separatorIndex]), nil
	}

	if atEOF {
		candidate := bytes.TrimSpace(data)
		return len(data), candidate, nil
	}

	return 0, nil, nil
}

func firstRecordSeparator(data []byte) (index int, sepLen int) {
	lineFeed := bytes.IndexByte(data, '\n')
	carriageReturn := bytes.IndexByte(data, '\r')

	switch {
	case lineFeed == -1 && carriageReturn == -1:
		return -1, 0
	case carriageReturn == -1:
		return lineFeed, 1
	case lineFeed == -1:
		return carriageReturn, 1
	case carriageReturn+1 == lineFeed:
		return carriageReturn, len(crlf)
	case carriageReturn < lineFeed:
		return carriageReturn, 1
	default:
		return lineFeed, 1
	}
}

func beginsRecordBoundary(data []byte) bool {
	data = bytes.TrimLeft(data, " \t")
	for _, prefix := range recordBoundaries {
		if bytes.HasPrefix(data, prefix) {
			return true
		}
	}
	return numericDIDPlainRecordPattern.Match(data)
}

func beginsRID(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, " \t"), []byte("RID "))
}

func nextRecordBoundary(data []byte, searchOffset int) int {
	next := -1
	for _, prefix := range recordBoundaries {
		if idx := bytes.Index(data[searchOffset:], prefix); idx >= 0 {
			idx += searchOffset
			if next == -1 || idx < next {
				next = idx
			}
		}
	}
	if idx := nextNumericDIDPlainBoundary(data, searchOffset); idx >= 0 && (next == -1 || idx < next) {
		next = idx
	}
	return next
}

func nextNumericDIDPlainBoundary(data []byte, searchOffset int) int {
	searchOffset = max(searchOffset, 0)
	for searchOffset < len(data) {
		idx := bytes.IndexByte(data[searchOffset:], ',')
		if idx == -1 {
			return -1
		}
		idx += searchOffset

		start := idx - 1
		for start >= 0 && data[start] >= '0' && data[start] <= '9' {
			start--
		}
		start++

		if start < idx && numericDIDPlainRecordPattern.Match(data[start:]) {
			return start
		}

		searchOffset = idx + 1
	}
	return -1
}

func nextCompleteDetectStart(data []byte) int {
	searchOffset := 1
	for searchOffset < len(data) {
		idx := bytes.Index(data[searchOffset:], []byte("device="))
		if idx == -1 {
			return -1
		}
		idx += searchOffset

		loc := detectRecordPattern.FindIndex(data[idx:])
		if loc != nil && loc[0] == 0 {
			return idx
		}

		searchOffset = idx + len("device=")
	}
	return -1
}
