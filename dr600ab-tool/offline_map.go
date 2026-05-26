package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type OfflineMapUploadRequest struct {
	InstallDir  string `json:"installDir"`
	PackagePath string `json:"packagePath"`
	KeepBackup  bool   `json:"keepBackup"`
}

type OfflineMapUploadResult struct {
	InstallDir string `json:"installDir"`
	TileCount  int    `json:"tileCount"`
	Message    string `json:"message"`
}

type OfflineMapCleanupRequest struct {
	InstallDir string `json:"installDir"`
}

func (a *App) UploadOfflineMap(req OfflineMapUploadRequest) (OfflineMapUploadResult, error) {
	req.InstallDir = a.getInstallDir(req.InstallDir)
	req.PackagePath = strings.TrimSpace(req.PackagePath)
	if req.PackagePath == "" {
		return OfflineMapUploadResult{}, fmt.Errorf("请选择离线地图 ZIP 包")
	}
	if !strings.EqualFold(filepath.Ext(req.PackagePath), ".zip") {
		return OfflineMapUploadResult{}, fmt.Errorf("离线地图只支持 .zip 文件")
	}
	if _, err := a.getSSHClient(); err != nil {
		return OfflineMapUploadResult{}, err
	}
	a.updateConfig(func(cfg *AppConfig) {
		cfg.InstallDir = req.InstallDir
		cfg.MapPackage = req.PackagePath
	})

	a.emitProgress("offline-map-progress", 0, "校验地图包", "正在校验并规范化离线地图", "running", 0, nil)
	preparedPath, tileCount, cleanup, err := prepareOfflineMapPackage(req.PackagePath)
	if err != nil {
		a.emitProgress("offline-map-progress", 0, "校验地图包", "地图包校验失败", "error", 0, err)
		return OfflineMapUploadResult{}, err
	}
	defer cleanup()
	a.emitProgress("offline-map-progress", 0, "校验地图包", fmt.Sprintf("地图包校验完成，共 %d 个瓦片", tileCount), "success", 100, nil)

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	remoteZip := remoteJoin(req.InstallDir, "static", "map", ".uploads", taskID, "offline-map.zip")
	a.emitProgress("offline-map-progress", 1, "上传地图包", "正在上传离线地图", "running", 0, nil)
	if err := a.uploadFile(preparedPath, remoteZip, func(read, total int64) {
		progress := 0
		if total > 0 {
			progress = int(float64(read) / float64(total) * 100)
		}
		a.emitProgress("offline-map-progress", 1, "上传地图包", fmt.Sprintf("已上传 %s / %s", formatBytes(read), formatBytes(total)), "running", progress, nil)
	}); err != nil {
		a.emitProgress("offline-map-progress", 1, "上传地图包", "上传失败", "error", 0, err)
		return OfflineMapUploadResult{}, err
	}
	a.emitProgress("offline-map-progress", 1, "上传地图包", "上传完成", "success", 100, nil)

	a.emitProgress("offline-map-progress", 2, "切换地图", "正在远程解压并切换地图目录", "running", 40, nil)
	output, err := a.runCommand(buildOfflineMapInstallScript(req.InstallDir, remoteZip, taskID, req.KeepBackup))
	if err != nil {
		wrapped := fmt.Errorf("%w%s", err, commandOutputSuffix(output))
		a.emitProgress("offline-map-progress", 2, "切换地图", "切换失败", "error", 40, wrapped)
		return OfflineMapUploadResult{}, wrapped
	}
	a.emitProgress("offline-map-progress", 2, "切换地图", "地图目录已切换", "success", 100, nil)
	a.emitProgress("offline-map-progress", 3, "完成", "离线地图已上传并切换", "success", 100, nil)
	return OfflineMapUploadResult{InstallDir: req.InstallDir, TileCount: tileCount, Message: "离线地图上传完成"}, nil
}

func (a *App) CleanupOfflineMapBackup(req OfflineMapCleanupRequest) (string, error) {
	installDir := a.getInstallDir(req.InstallDir)
	backupDir := remoteJoin(installDir, "static", "map", ".backup")
	if _, err := a.runCommand("rm -rf " + shellQuote(backupDir)); err != nil {
		return "", err
	}
	return "已清理离线地图备份目录", nil
}

func buildOfflineMapInstallScript(installDir, remoteZip, taskID string, keepBackup bool) string {
	keep := "0"
	if keepBackup {
		keep = "1"
	}
	return fmt.Sprintf(`set -eu
INSTALL_DIR=%s
REMOTE_ZIP=%s
TASK_ID=%s
KEEP_BACKUP=%s
SUDO=
if [ "$(id -u)" != "0" ]; then
  SUDO=sudo
fi
MAP_DIR="$INSTALL_DIR/static/map"
STAGING_DIR="$MAP_DIR/.staging/$TASK_ID"
BACKUP_ROOT="$MAP_DIR/.backup"
CURRENT_DT="$MAP_DIR/dt"
BACKUP_DT="$BACKUP_ROOT/dt_$(date +%%Y%%m%%d%%H%%M%%S)"
$SUDO mkdir -p "$MAP_DIR/.uploads" "$STAGING_DIR" "$BACKUP_ROOT"
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"
if command -v unzip >/dev/null 2>&1; then
  unzip -q "$REMOTE_ZIP" -d "$STAGING_DIR"
elif command -v busybox >/dev/null 2>&1; then
  busybox unzip -q "$REMOTE_ZIP" -d "$STAGING_DIR"
else
  echo "设备未安装 unzip 或 busybox unzip" >&2
  exit 1
fi
if [ ! -d "$STAGING_DIR/dt" ]; then
  echo "离线地图包缺少 dt 目录" >&2
  exit 1
fi
chmod -R a+rX "$STAGING_DIR/dt" || true
if [ -d "$CURRENT_DT" ]; then
  $SUDO mv "$CURRENT_DT" "$BACKUP_DT"
fi
if ! $SUDO mv "$STAGING_DIR/dt" "$CURRENT_DT"; then
  if [ -d "$BACKUP_DT" ]; then
    $SUDO mv "$BACKUP_DT" "$CURRENT_DT" || true
  fi
  exit 1
fi
rm -rf "$(dirname "$REMOTE_ZIP")" "$STAGING_DIR"
if [ "$KEEP_BACKUP" != "1" ] && [ -d "$BACKUP_DT" ]; then
  rm -rf "$BACKUP_DT"
fi
echo "offline map switched"
`, shellQuote(installDir), shellQuote(remoteZip), shellQuote(taskID), shellQuote(keep))
}

type offlineMapLayout struct {
	StripPrefix string
}

func prepareOfflineMapPackage(sourcePath string) (string, int, func(), error) {
	reader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return "", 0, func() {}, fmt.Errorf("打开 ZIP 失败: %w", err)
	}
	defer reader.Close()

	var layout offlineMapLayout
	layoutSet := false
	for _, file := range reader.File {
		name := normalizeZipEntryName(file.Name)
		if name == "" || isIgnorableZipEntry(name) || file.FileInfo().IsDir() {
			continue
		}
		if err := validateOfflineMapEntry(file, name); err != nil {
			return "", 0, func() {}, err
		}
		if isXML(name) {
			continue
		}
		detected, ok := detectOfflineMapTileLayout(name)
		if !ok {
			return "", 0, func() {}, fmt.Errorf("不支持的离线地图瓦片路径: %s", name)
		}
		if !layoutSet {
			layout = detected
			layoutSet = true
		}
	}
	if !layoutSet {
		return "", 0, func() {}, fmt.Errorf("离线地图包中没有有效瓦片")
	}

	tmpDir, err := os.MkdirTemp("", "dr600ab-map-*")
	if err != nil {
		return "", 0, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	targetPath := filepath.Join(tmpDir, "offline-map-normalized.zip")
	out, err := os.Create(targetPath)
	if err != nil {
		cleanup()
		return "", 0, func() {}, err
	}
	zipWriter := zip.NewWriter(out)
	tileCount := 0
	success := false
	defer func() {
		_ = zipWriter.Close()
		_ = out.Close()
		if !success {
			cleanup()
		}
	}()

	seen := map[string]struct{}{}
	for _, file := range reader.File {
		name := normalizeZipEntryName(file.Name)
		if name == "" || isIgnorableZipEntry(name) || file.FileInfo().IsDir() || isXML(name) {
			continue
		}
		if err := validateOfflineMapEntry(file, name); err != nil {
			return "", 0, func() {}, err
		}
		rel, ok := layout.normalize(name)
		if !ok {
			return "", 0, func() {}, fmt.Errorf("离线地图包存在混合目录结构: %s", name)
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		if err := copyZipFile(zipWriter, file, rel); err != nil {
			return "", 0, func() {}, err
		}
		tileCount++
	}
	if tileCount == 0 {
		return "", 0, func() {}, fmt.Errorf("离线地图包中没有可提取瓦片")
	}
	if err := zipWriter.Close(); err != nil {
		return "", 0, func() {}, err
	}
	if err := out.Close(); err != nil {
		return "", 0, func() {}, err
	}
	success = true
	return targetPath, tileCount, cleanup, nil
}

func copyZipFile(writer *zip.Writer, file *zip.File, name string) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	target, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	_, err = io.Copy(target, source)
	return err
}

func validateOfflineMapEntry(file *zip.File, name string) error {
	raw := strings.TrimSpace(file.Name)
	rawSlash := filepath.ToSlash(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") || filepath.IsAbs(raw) || strings.Contains(raw, ":") || hasParentPathSegment(rawSlash) {
		return fmt.Errorf("ZIP 包含非法路径: %s", file.Name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("ZIP 包含非法路径: %s", name)
	}
	if file.FileInfo().Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ZIP 包含符号链接: %s", name)
	}
	if hasHiddenZipPart(name) {
		return fmt.Errorf("ZIP 包含隐藏或系统目录: %s", name)
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".dll", ".so", ".dylib", ".sh", ".bat", ".cmd", ".ps1", ".php", ".jsp", ".asp", ".aspx", ".sql", ".db", ".sqlite"} {
		if strings.HasSuffix(lower, ext) {
			return fmt.Errorf("ZIP 包含不允许的文件类型: %s", name)
		}
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".xml" || ext == ".jpg" || ext == ".jpeg" {
		return nil
	}
	return fmt.Errorf("离线地图只支持 JPG/JPEG 瓦片: %s", name)
}

func detectOfflineMapTileLayout(name string) (offlineMapLayout, bool) {
	parts := strings.Split(name, "/")
	if len(parts) >= 4 && parts[0] == "dt" && isValidTileXYZ(parts[1], parts[2], parts[3]) {
		return offlineMapLayout{StripPrefix: "dt"}, true
	}
	if len(parts) >= 5 && parts[1] == "dt" && isValidTileXYZ(parts[2], parts[3], parts[4]) {
		return offlineMapLayout{StripPrefix: parts[0] + "/dt"}, true
	}
	if len(parts) >= 3 && isValidTileXYZ(parts[0], parts[1], parts[2]) {
		return offlineMapLayout{}, true
	}
	if len(parts) >= 4 && isValidTileXYZ(parts[1], parts[2], parts[3]) {
		return offlineMapLayout{StripPrefix: parts[0]}, true
	}
	return offlineMapLayout{}, false
}

func (l offlineMapLayout) normalize(name string) (string, bool) {
	trimmed := name
	if l.StripPrefix != "" {
		prefix := strings.Trim(l.StripPrefix, "/") + "/"
		if !strings.HasPrefix(name, prefix) {
			return "", false
		}
		trimmed = strings.TrimPrefix(name, prefix)
	}
	if trimmed == "" || isXML(trimmed) {
		return "", false
	}
	return normalizeTileExtension("dt/" + trimmed), true
}

func isValidTileXYZ(zText, xText, yFile string) bool {
	z, err := strconv.Atoi(zText)
	if err != nil || z < 0 || z > 22 {
		return false
	}
	if _, err := strconv.Atoi(xText); err != nil {
		return false
	}
	yText := strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(yFile), ".jpeg"), ".jpg")
	if _, err := strconv.Atoi(yText); err != nil {
		return false
	}
	return true
}

func normalizeZipEntryName(name string) string {
	name = filepath.ToSlash(strings.TrimSpace(name))
	name = strings.TrimLeft(name, "/")
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func normalizeTileExtension(name string) string {
	ext := filepath.Ext(name)
	if strings.EqualFold(ext, ".jpeg") {
		return strings.TrimSuffix(name, ext) + ".jpg"
	}
	return name
}

func isXML(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".xml")
}

func isIgnorableZipEntry(name string) bool {
	if name == "" {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if part == "__MACOSX" || part == ".DS_Store" || strings.HasPrefix(part, "._") {
			return true
		}
	}
	return false
}

func hasHiddenZipPart(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return true
		}
	}
	return false
}

func hasParentPathSegment(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
