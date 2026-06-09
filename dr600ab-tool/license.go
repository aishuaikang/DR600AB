package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LicenseUploadRequest struct {
	InstallDir  string `json:"installDir"`
	LicensePath string `json:"licensePath"`
	APIHost     string `json:"apiHost,omitempty"`
	APIPort     int    `json:"apiPort,omitempty"`
}

type LicenseUploadResult struct {
	Message string `json:"message"`
}

type licenseUploadResponse struct {
	Message string `json:"message"`
}

type licenseAPIError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (a *App) UploadLicense(req LicenseUploadRequest) (LicenseUploadResult, error) {
	req.InstallDir = a.getInstallDir(req.InstallDir)
	req.LicensePath = strings.TrimSpace(req.LicensePath)
	if req.LicensePath == "" {
		return LicenseUploadResult{}, fmt.Errorf("请选择授权文件")
	}
	if _, err := os.Stat(req.LicensePath); err != nil {
		return LicenseUploadResult{}, fmt.Errorf("读取授权文件失败: %w", err)
	}
	if _, err := a.getSSHClient(); err != nil {
		return LicenseUploadResult{}, err
	}

	a.updateConfig(func(cfg *AppConfig) {
		cfg.InstallDir = req.InstallDir
		cfg.LicensePath = req.LicensePath
	})

	apiURL := a.licenseUploadURL(req)
	a.emitProgress("license-progress", 0, "校验授权文件", "正在读取授权文件", "running", 0, nil)
	if err := validateLicensePath(req.LicensePath); err != nil {
		a.emitProgress("license-progress", 0, "校验授权文件", "授权文件校验失败", "error", 0, err)
		return LicenseUploadResult{}, err
	}
	a.emitProgress("license-progress", 0, "校验授权文件", "授权文件已就绪", "success", 100, nil)

	a.emitProgress("license-progress", 1, "上传授权文件", "正在上传并激活授权", "running", 20, nil)
	message, err := postLicenseFile(apiURL, req.LicensePath)
	if err != nil {
		a.emitProgress("license-progress", 1, "上传授权文件", "上传授权失败", "error", 20, err)
		return LicenseUploadResult{}, err
	}
	a.emitProgress("license-progress", 1, "上传授权文件", "授权文件已上传并激活", "success", 100, nil)
	a.emitProgress("license-progress", 2, "完成", "授权已启用", "success", 100, nil)
	if strings.TrimSpace(message) == "" {
		message = "授权文件已上传并激活"
	}
	return LicenseUploadResult{Message: message}, nil
}

func validateLicensePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("读取授权文件失败: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("授权文件不能是目录")
	}
	if info.Size() == 0 {
		return fmt.Errorf("授权文件不能为空")
	}
	if !strings.EqualFold(filepath.Ext(path), ".lic") {
		return fmt.Errorf("请选择 .lic 授权文件")
	}
	return nil
}

func (a *App) licenseUploadURL(req LicenseUploadRequest) string {
	host := strings.TrimSpace(req.APIHost)
	if host == "" {
		host = a.currentSSHHost()
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port := req.APIPort
	if port == 0 {
		port = 18080
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/api/v1/license/upload"
}

func (a *App) currentSSHHost() string {
	a.sshMu.Lock()
	defer a.sshMu.Unlock()
	if a.conn == nil {
		return ""
	}
	return strings.TrimSpace(a.conn.config.Host)
}

func postLicenseFile(url string, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开授权文件失败: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("创建上传表单失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("读取授权文件失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("完成上传表单失败: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("创建上传请求失败: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Locale", "zh-CN")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接设备授权接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取授权接口响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload licenseAPIError
		if err := json.Unmarshal(respBody, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			if payload.Code != "" {
				return "", fmt.Errorf("%s（%s）", payload.Message, payload.Code)
			}
			return "", fmt.Errorf("%s", payload.Message)
		}
		return "", fmt.Errorf("授权接口返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var payload licenseUploadResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("解析授权接口响应失败: %w", err)
	}
	return payload.Message, nil
}
