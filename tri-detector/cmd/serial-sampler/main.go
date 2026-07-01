package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.bug.st/serial"

	"serialport"
	"tri-detector/client"
	"tri-detector/parser"
)

type sampleEvent struct {
	source string
	line   string
	at     time.Time
}

type sampleStats struct {
	records    int
	parsed     int
	unknown    int
	suspicious int
	types      map[parser.MessageType]int
	keys       map[string]int
}

func newSampleStats() *sampleStats {
	return &sampleStats{
		types: map[parser.MessageType]int{},
		keys:  map[string]int{},
	}
}

type reportEntry struct {
	at      time.Time
	source  string
	status  string
	summary string
	raw     string
}

type sampleReport struct {
	startedAt     time.Time
	endedAt       time.Time
	s1Port        string
	s2Port        string
	baudRate      int
	command       string
	duration      time.Duration
	problemCounts map[string]int
	samples       []reportEntry
	problems      []reportEntry
}

func newSampleReport(
	s1Port string,
	s2Port string,
	baudRate int,
	command string,
	duration time.Duration,
) *sampleReport {
	return &sampleReport{
		startedAt:     time.Now(),
		s1Port:        s1Port,
		s2Port:        s2Port,
		baudRate:      baudRate,
		command:       strings.TrimSpace(command),
		duration:      duration,
		problemCounts: map[string]int{},
		samples:       []reportEntry{},
		problems:      []reportEntry{},
	}
}

func (r *sampleReport) finish() {
	r.endedAt = time.Now()
}

func (r *sampleReport) recordSample(ev sampleEvent, msg *parser.Message) {
	r.samples = append(r.samples, reportEntry{
		at:      ev.at,
		source:  ev.source,
		status:  string(msg.Type),
		summary: businessSummary(msg),
		raw:     trimLine(ev.line, 300),
	})
}

func (r *sampleReport) recordProblem(ev sampleEvent, status string, err error) {
	r.problemCounts[status]++
	summary := status
	if err != nil {
		summary = status + ": " + err.Error()
	}
	r.problems = append(r.problems, reportEntry{
		at:      ev.at,
		source:  ev.source,
		status:  status,
		summary: summary,
		raw:     trimLine(ev.line, 300),
	})
}

func main() {
	s1Port := flag.String("s1", "", "S1 串口，可收可发")
	s2Port := flag.String("s2", "", "S2 串口，仅接收")
	baudRate := flag.Int("baud", 115200, "波特率")
	command := flag.String("cmd", "start -freq 1, -pathb 1, -gain 60", "启动后发送到 S1 的命令，留空则不发送")
	duration := flag.Duration("duration", 30*time.Second, "采样时长，0 表示直到 Ctrl+C")
	interval := flag.Duration("interval", 5*time.Second, "摘要输出间隔，0 表示不输出周期摘要")
	showUnknown := flag.Bool("unknown", false, "实时打印未知记录")
	showRaw := flag.Bool("raw", false, "实时打印所有原始记录")
	colorMode := flag.String("color", "always", "颜色输出: auto/always/never")
	reportPath := flag.String("report", "./serial-sampler-report.md", "采样结束后写入 Markdown 诊断报告，留空则只打印到控制台")
	flag.Parse()

	if strings.TrimSpace(*s1Port) == "" || strings.TrimSpace(*s2Port) == "" {
		log.Fatal("请通过 -s1 和 -s2 指定串口")
	}

	colorEnabled, err := resolveColorMode(*colorMode, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
	theme := newColorTheme(colorEnabled)

	s1Raw, s2Raw, err := openSamplerPorts(*s1Port, *s2Port, *baudRate)
	if err != nil {
		log.Fatal(err)
	}

	s1 := client.NewSerialClient(s1Raw, *s1Port, false)
	s2 := client.NewSerialClient(s2Raw, *s2Port, false)
	s1.SetOutput(io.Discard)
	s2.SetOutput(io.Discard)

	events := make(chan sampleEvent, 4096)
	var wg sync.WaitGroup
	startReader(&wg, events, "S1", s1)
	startReader(&wg, events, "S2", s2)

	fmt.Printf("S1: %s\n", *s1Port)
	fmt.Printf("S2: %s\n", *s2Port)
	fmt.Printf("baud: %d\n", *baudRate)
	if *duration > 0 {
		fmt.Printf("duration: %s\n", duration.String())
	} else {
		fmt.Println("duration: until Ctrl+C")
	}

	time.Sleep(300 * time.Millisecond)
	if strings.TrimSpace(*command) != "" {
		if err := s1.Send(*command); err != nil {
			log.Fatalf("发送命令失败: %v", err)
		}
		fmt.Printf("[TX S1] %q\n", *command)
	}

	stats := map[string]*sampleStats{
		"S1": newSampleStats(),
		"S2": newSampleStats(),
	}
	report := newSampleReport(
		*s1Port,
		*s2Port,
		*baudRate,
		*command,
		*duration,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var done <-chan time.Time
	if *duration > 0 {
		timer := time.NewTimer(*duration)
		defer timer.Stop()
		done = timer.C
	}

	var tickerC <-chan time.Time
	var ticker *time.Ticker
	if *interval > 0 {
		ticker = time.NewTicker(*interval)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	stopping := false
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				report.finish()
				printSummary("final", stats, theme)
				printDiagnosticReport(report, stats, *reportPath)
				return
			}
			handleEvent(ev, stats[ev.source], report, *showRaw, *showUnknown, theme)
		case <-tickerC:
			printSummary("interval", stats, theme)
		case <-done:
			if !stopping {
				stopping = true
				stopReaders(s1, s2, &wg, events)
				done = nil
				tickerC = nil
			}
		case <-sigCh:
			if !stopping {
				stopping = true
				stopReaders(s1, s2, &wg, events)
				done = nil
				tickerC = nil
			}
		}
	}
}

func openSamplerPorts(s1Port, s2Port string, baudRate int) (serial.Port, serial.Port, error) {
	s1Cfg := serialport.DefaultConfig(baudRate)
	s1Cfg.PortName = s1Port
	s1, err := serialport.Open(&s1Cfg)
	if err != nil {
		return nil, nil, err
	}

	s2Cfg := serialport.DefaultConfig(baudRate)
	s2Cfg.PortName = s2Port
	s2, err := serialport.Open(&s2Cfg)
	if err != nil {
		s1.Close()
		return nil, nil, err
	}

	return s1, s2, nil
}

func startReader(wg *sync.WaitGroup, events chan<- sampleEvent, source string, c *client.SerialClient) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.ReadLoop(func(line string) {
			events <- sampleEvent{
				source: source,
				line:   line,
				at:     time.Now(),
			}
		})
	}()
}

func stopReaders(s1 *client.SerialClient, s2 *client.SerialClient, wg *sync.WaitGroup, events chan sampleEvent) {
	s1.Close()
	s2.Close()
	go func() {
		wg.Wait()
		close(events)
	}()
}

func handleEvent(
	ev sampleEvent,
	stats *sampleStats,
	report *sampleReport,
	showRaw bool,
	showUnknown bool,
	theme colorTheme,
) {
	stats.records++

	msg, err := parser.ParseLine(ev.line)
	if err != nil {
		stats.unknown++
		if isSuspicious(ev.line) {
			stats.suspicious++
			report.recordProblem(ev, "SUSPICIOUS", err)
			fmt.Printf(
				"[%s] %s %s %s\n",
				theme.muted(ev.at.Format("15:04:05.000")),
				theme.source(ev.source),
				theme.status("SUSPICIOUS"),
				trimLine(ev.line, 240),
			)
			return
		}
		report.recordProblem(ev, "UNKNOWN", err)
		if showRaw || showUnknown {
			fmt.Printf(
				"[%s] %s %s %s\n",
				theme.muted(ev.at.Format("15:04:05.000")),
				theme.source(ev.source),
				theme.status("UNKNOWN"),
				trimLine(ev.line, 240),
			)
		}
		return
	}

	if isSuspicious(ev.line) {
		stats.suspicious++
		report.recordProblem(ev, "SUSPICIOUS", fmt.Errorf("parsed record still contains suspicious boundaries"))
	}

	stats.parsed++
	stats.types[msg.Type]++
	stats.keys[businessKey(msg)]++
	if reason := malformedBusinessReason(msg); reason != "" {
		report.recordProblem(ev, "MALFORMED", fmt.Errorf("%s", reason))
	}
	report.recordSample(ev, msg)

	if showRaw {
		fmt.Printf(
			"[%s] %s %s %s\n",
			theme.muted(ev.at.Format("15:04:05.000")),
			theme.source(ev.source),
			theme.status("RAW"),
			trimLine(ev.line, 240),
		)
	}
	fmt.Printf(
		"[%s] %s %-13s %s\n",
		theme.muted(ev.at.Format("15:04:05.000")),
		theme.source(ev.source),
		theme.messageType(msg.Type),
		businessSummary(msg),
	)
}

func printSummary(label string, stats map[string]*sampleStats, theme colorTheme) {
	fmt.Printf("\n%s\n", theme.heading(fmt.Sprintf("=== %s summary %s ===", label, time.Now().Format("15:04:05"))))
	for _, source := range []string{"S1", "S2"} {
		s := stats[source]
		fmt.Printf(
			"%s records=%d parsed=%d unknown=%d suspicious=%d types=%s\n",
			theme.source(source),
			s.records,
			s.parsed,
			s.unknown,
			s.suspicious,
			formatTypes(s.types),
		)
	}

	mismatches := compareKeys(stats["S1"].keys, stats["S2"].keys)
	fmt.Printf("%s=%d\n", theme.label("business_mismatch"), len(mismatches))
	for i, m := range mismatches {
		if i >= 12 {
			fmt.Printf("... %d more\n", len(mismatches)-i)
			break
		}
		fmt.Printf("  %s S1=%d S2=%d\n", m.key, m.s1, m.s2)
	}
	fmt.Println()
}

func printDiagnosticReport(report *sampleReport, stats map[string]*sampleStats, reportPath string) {
	content := buildDiagnosticReport(report, stats)
	fmt.Println(content)

	if strings.TrimSpace(reportPath) == "" {
		return
	}
	if err := os.WriteFile(reportPath, []byte(content), 0o644); err != nil {
		log.Printf("写入诊断报告失败: %v", err)
		return
	}
	fmt.Printf("诊断报告已写入: %s\n", reportPath)
}

func buildDiagnosticReport(report *sampleReport, stats map[string]*sampleStats) string {
	var b strings.Builder
	mismatches := compareKeys(stats["S1"].keys, stats["S2"].keys)
	problemCounts := report.problemCounts

	b.WriteString("\n# 串口采样诊断报告\n\n")
	b.WriteString("## 采样配置\n\n")
	fmt.Fprintf(&b, "- 开始时间: %s\n", report.startedAt.Format("2006-01-02 15:04:05"))
	if !report.endedAt.IsZero() {
		fmt.Fprintf(&b, "- 结束时间: %s\n", report.endedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&b, "- 实际耗时: %s\n", report.endedAt.Sub(report.startedAt).Round(time.Millisecond))
	}
	fmt.Fprintf(&b, "- S1: %s\n", report.s1Port)
	fmt.Fprintf(&b, "- S2: %s\n", report.s2Port)
	fmt.Fprintf(&b, "- baud: %d\n", report.baudRate)
	if report.duration > 0 {
		fmt.Fprintf(&b, "- 计划采样时长: %s\n", report.duration)
	} else {
		b.WriteString("- 计划采样时长: until Ctrl+C\n")
	}
	if report.command != "" {
		fmt.Fprintf(&b, "- S1 发送命令: `%s`\n", report.command)
	}

	b.WriteString("\n## 采样统计\n\n")
	for _, source := range []string{"S1", "S2"} {
		s := stats[source]
		fmt.Fprintf(
			&b,
			"- %s: records=%d parsed=%d unknown=%d suspicious=%d types=%s\n",
			source,
			s.records,
			s.parsed,
			s.unknown,
			s.suspicious,
			formatTypes(s.types),
		)
	}
	fmt.Fprintf(&b, "- business_mismatch=%d\n", len(mismatches))
	fmt.Fprintf(&b, "- problem_logs=%s\n", formatProblemCounts(problemCounts))

	b.WriteString("\n## 诊断结论\n\n")
	for _, line := range diagnosticConclusions(stats, mismatches, problemCounts) {
		fmt.Fprintf(&b, "- %s\n", line)
	}

	b.WriteString("\n## 解决建议\n\n")
	for _, line := range diagnosticSuggestions(stats, mismatches, problemCounts) {
		fmt.Fprintf(&b, "- %s\n", line)
	}

	b.WriteString("\n## Mismatch 明细\n\n")
	if len(mismatches) == 0 {
		b.WriteString("- 无\n")
	} else {
		for i, m := range mismatches {
			if i >= 50 {
				fmt.Fprintf(&b, "- ... 还有 %d 条\n", len(mismatches)-i)
				break
			}
			fmt.Fprintf(&b, "- `%s` S1=%d S2=%d\n", m.key, m.s1, m.s2)
		}
	}

	b.WriteString("\n## 采样日志\n\n")
	writeReportEntries(&b, report.samples)

	b.WriteString("\n## 问题日志\n\n")
	writeReportEntries(&b, report.problems)

	return b.String()
}

func diagnosticConclusions(
	stats map[string]*sampleStats,
	mismatches []keyMismatch,
	problemCounts map[string]int,
) []string {
	s1 := stats["S1"]
	s2 := stats["S2"]
	conclusions := []string{}

	if s1.suspicious == 0 && s2.suspicious == 0 {
		conclusions = append(conclusions, "未发现明显粘包吞帧特征，suspicious=0。")
	} else {
		conclusions = append(conclusions, "存在疑似粘包或边界未拆开的记录，需要优先查看问题日志中的 SUSPICIOUS 原始内容。")
	}
	if problemCounts["MALFORMED"] > 0 {
		conclusions = append(conclusions, "存在已解析但关键字段不完整的业务帧，可能是半帧被提前释放或新增报文格式未完全覆盖。")
	}

	if s1.parsed == s2.parsed && equalMessageTypeCounts(s1.types, s2.types) {
		conclusions = append(conclusions, "S1/S2 解析出的业务帧数量和类型数量一致。")
	} else {
		conclusions = append(conclusions, "S1/S2 解析出的业务帧数量或类型数量不一致，存在少解、多解或源数据差异。")
	}

	if len(mismatches) == 0 {
		conclusions = append(conclusions, "业务 key 完全一致，当前采样未发现 S1 相对 S2 丢帧。")
	} else if allMismatchesAreRID(mismatches) {
		conclusions = append(conclusions, "剩余 mismatch 都是 RID，通常是 GPS/RSSI 等瞬时字段不同或采样时序差异，不一定是切包错误。")
	} else {
		conclusions = append(conclusions, "存在非 RID 业务 mismatch，需要结合 Mismatch 明细判断具体类型。")
	}

	return conclusions
}

func diagnosticSuggestions(
	stats map[string]*sampleStats,
	mismatches []keyMismatch,
	problemCounts map[string]int,
) []string {
	s1 := stats["S1"]
	s2 := stats["S2"]
	suggestions := []string{}

	if s1.suspicious > 0 || s2.suspicious > 0 {
		suggestions = append(suggestions, "先用 `-unknown -raw` 复采，定位 SUSPICIOUS 原始串，再在 `client/split.go` 增加对应边界规则。")
	}
	if problemCounts["MALFORMED"] > 0 {
		suggestions = append(suggestions, "优先查看问题日志里的 MALFORMED 原始内容；如果是半帧，应收紧 `parser` 的必填字段校验或在 `client/split.go` 延迟释放该类记录。")
	}
	if s1.parsed != s2.parsed || !equalMessageTypeCounts(s1.types, s2.types) {
		suggestions = append(suggestions, "按类型对比 parsed 计数，优先检查少的一侧是否有 UNKNOWN 或 SUSPICIOUS 原始记录。")
	}
	if len(mismatches) > 0 && allMismatchesAreRID(mismatches) {
		suggestions = append(suggestions, "如果只想检查是否丢帧，RID 对比 key 可考虑忽略 GPS/RSSI 等高频变化字段，改用 ssid/serial/model/freq 等稳定字段。")
	}
	if len(mismatches) == 0 && s1.suspicious == 0 && s2.suspicious == 0 {
		suggestions = append(suggestions, "当前切包规则可继续使用；后续重点观察新增报文格式是否进入 UNKNOWN。")
	}
	if s1.unknown > 0 || s2.unknown > 0 {
		suggestions = append(suggestions, "UNKNOWN 不一定是错误，可能是启动残留、半帧或暂不支持的日志；需要时查看问题日志里的原始内容决定是否新增解析规则。")
	}

	return suggestions
}

func writeReportEntries(b *strings.Builder, entries []reportEntry) {
	if len(entries) == 0 {
		b.WriteString("- 无\n")
		return
	}
	for _, entry := range entries {
		fmt.Fprintf(
			b,
			"- %s %s %-13s %s\n",
			entry.at.Format("15:04:05.000"),
			entry.source,
			entry.status,
			entry.summary,
		)
		if entry.raw != "" {
			fmt.Fprintf(b, "  raw: `%s`\n", escapeBackticks(entry.raw))
		}
	}
}

func formatProblemCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func malformedBusinessReason(msg *parser.Message) string {
	switch data := msg.Data.(type) {
	case *parser.DIDEncrypted:
		if data.Device == "" || data.EncryptedID == "" || data.Freq == 0 || data.RSSI == 0 || data.Bytes == "" {
			return "did_encrypted 关键字段不完整，可能是半帧被解析成业务帧"
		}
	case *parser.DIDPlain:
		if data.Device == "" || data.Serial == "" || data.Model == "" || data.Freq == 0 || data.RSSI == 0 {
			return "did_plain 关键字段不完整，可能是半帧或新增格式"
		}
	case *parser.Detect:
		if data.Device == "" || data.Model == "" || data.Freq == 0 || data.RSSI == 0 {
			return "detect 关键字段不完整，可能是半帧或新增格式"
		}
	case *parser.Heartbeat:
		if data.Device == "" || data.Seq == "" {
			return "heartbeat 关键字段不完整，可能是半帧或新增格式"
		}
	case *parser.RID:
		if data.SSID == "" || data.Serial == "" || data.Model == "" || data.Freq == 0 || data.RSSI == 0 {
			return "rid 关键字段不完整，可能是半帧或新增格式"
		}
	}
	return ""
}

type keyMismatch struct {
	key string
	s1  int
	s2  int
}

func compareKeys(s1 map[string]int, s2 map[string]int) []keyMismatch {
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(s1)+len(s2))
	for key := range s1 {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range s2 {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	mismatches := []keyMismatch{}
	for _, key := range keys {
		if s1[key] != s2[key] {
			mismatches = append(mismatches, keyMismatch{
				key: key,
				s1:  s1[key],
				s2:  s2[key],
			})
		}
	}
	return mismatches
}

func businessKey(msg *parser.Message) string {
	switch data := msg.Data.(type) {
	case *parser.Detect:
		return strings.Join([]string{
			string(msg.Type),
			data.Device,
			data.Model,
			floatKey(data.Freq),
			floatKey(data.RSSI),
		}, "|")
	case *parser.Heartbeat:
		return strings.Join([]string{
			string(msg.Type),
			data.Device,
			data.Seq,
		}, "|")
	case *parser.DIDEncrypted:
		return strings.Join([]string{
			string(msg.Type),
			data.Device,
			data.EncryptedID,
			floatKey(data.Freq),
			floatKey(data.RSSI),
		}, "|")
	case *parser.DIDPlain:
		return strings.Join([]string{
			string(msg.Type),
			data.Device,
			data.Serial,
			data.Model,
			data.UUID,
			floatKey(data.Freq),
			floatKey(data.RSSI),
		}, "|")
	case *parser.RID:
		return strings.Join([]string{
			string(msg.Type),
			data.SSID,
			data.Serial,
			data.Model,
			floatKey(data.PilotGPS.Lat),
			floatKey(data.PilotGPS.Lng),
			floatKey(data.Freq),
			floatKey(data.RSSI),
		}, "|")
	default:
		return string(msg.Type) + "|" + msg.Raw
	}
}

func businessSummary(msg *parser.Message) string {
	switch data := msg.Data.(type) {
	case *parser.Detect:
		return fmt.Sprintf("device=%s model=%s freq=%.1f rssi=%.1f", data.Device, data.Model, data.Freq, data.RSSI)
	case *parser.Heartbeat:
		return fmt.Sprintf("device=%s seq=%s", data.Device, data.Seq)
	case *parser.DIDEncrypted:
		return fmt.Sprintf(
			"device=%s id=%s freq=%.1f rssi=%.1f bytes=%d",
			data.Device,
			data.EncryptedID,
			data.Freq,
			data.RSSI,
			len(data.Bytes)/2,
		)
	case *parser.DIDPlain:
		return fmt.Sprintf(
			"device=%s serial=%s model=%s uuid=%s freq=%.1f rssi=%.1f",
			data.Device,
			data.Serial,
			data.Model,
			data.UUID,
			data.Freq,
			data.RSSI,
		)
	case *parser.RID:
		return fmt.Sprintf(
			"ssid=%s serial=%s model=%s pilot=%.6f,%.6f freq=%.1f rssi=%.1f",
			data.SSID,
			data.Serial,
			data.Model,
			data.PilotGPS.Lat,
			data.PilotGPS.Lng,
			data.Freq,
			data.RSSI,
		)
	default:
		return trimLine(msg.Raw, 200)
	}
}

func formatTypes(types map[parser.MessageType]int) string {
	if len(types) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(types))
	for key := range types {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, types[parser.MessageType(key)]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func equalMessageTypeCounts(a map[parser.MessageType]int, b map[parser.MessageType]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func allMismatchesAreRID(mismatches []keyMismatch) bool {
	if len(mismatches) == 0 {
		return false
	}
	for _, mismatch := range mismatches {
		if !strings.HasPrefix(mismatch.key, string(parser.TypeRID)+"|") {
			return false
		}
	}
	return true
}

func isSuspicious(line string) bool {
	if strings.Count(line, "device=") > 1 {
		return true
	}
	if strings.Contains(line, "RID ssid=") && (strings.Contains(line, "#=") || strings.Contains(line, "com #=")) {
		return true
	}
	return false
}

func trimLine(line string, max int) string {
	line = strings.TrimSpace(line)
	if len(line) <= max {
		return line
	}
	return line[:max] + "..."
}

func floatKey(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func escapeBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "\\`")
}

type colorTheme struct {
	enabled bool
}

func newColorTheme(enabled bool) colorTheme {
	return colorTheme{enabled: enabled}
}

func resolveColorMode(mode string, output *os.File) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		stat, err := output.Stat()
		if err != nil {
			return false, err
		}
		return stat.Mode()&os.ModeCharDevice != 0, nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("无效颜色模式 %q，可选: auto/always/never", mode)
	}
}

func (t colorTheme) color(code string, value string) string {
	if !t.enabled {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func (t colorTheme) source(source string) string {
	switch source {
	case "S1":
		return t.color("36;1", fmt.Sprintf("%-2s", source))
	case "S2":
		return t.color("35;1", fmt.Sprintf("%-2s", source))
	default:
		return fmt.Sprintf("%-2s", source)
	}
}

func (t colorTheme) messageType(messageType parser.MessageType) string {
	value := fmt.Sprintf("%-13s", messageType)
	switch messageType {
	case parser.TypeDetect:
		return t.color("32;1", value)
	case parser.TypeHeartbeat:
		return t.color("34;1", value)
	case parser.TypeDIDEncrypted:
		return t.color("33;1", value)
	case parser.TypeDIDPlain:
		return t.color("36", value)
	case parser.TypeRID:
		return t.color("35", value)
	default:
		return value
	}
}

func (t colorTheme) status(status string) string {
	value := fmt.Sprintf("%-13s", status)
	switch status {
	case "SUSPICIOUS":
		return t.color("31;1", value)
	case "UNKNOWN":
		return t.color("90", value)
	case "RAW":
		return t.color("90", value)
	default:
		return value
	}
}

func (t colorTheme) muted(value string) string {
	return t.color("90", value)
}

func (t colorTheme) heading(value string) string {
	return t.color("1", value)
}

func (t colorTheme) label(value string) string {
	return t.color("1", value)
}
