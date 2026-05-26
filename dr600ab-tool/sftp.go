package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

type progressReader struct {
	reader  io.Reader
	total   int64
	read    int64
	onBytes func(read, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.onBytes != nil {
			r.onBytes(r.read, r.total)
		}
	}
	return n, err
}

func (a *App) uploadFile(localPath, remotePath string, onProgress func(read, total int64)) error {
	client, err := a.getSSHClient()
	if err != nil {
		return err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("创建 SFTP 客户端失败: %w", err)
	}
	defer sftpClient.Close()

	local, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer local.Close()
	stat, err := local.Stat()
	if err != nil {
		return fmt.Errorf("读取本地文件信息失败: %w", err)
	}
	dir := remoteDir(remotePath)
	if dir != "" {
		if err := sftpClient.MkdirAll(dir); err != nil {
			return fmt.Errorf("创建远程目录失败: %w", err)
		}
	}
	tmpPath := remotePath + ".tmp"
	remote, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建远程文件失败: %w", err)
	}
	reader := &progressReader{reader: local, total: stat.Size(), onBytes: onProgress}
	_, copyErr := io.Copy(remote, reader)
	closeErr := remote.Close()
	if copyErr != nil {
		_ = sftpClient.Remove(tmpPath)
		return fmt.Errorf("上传文件失败: %w", copyErr)
	}
	if closeErr != nil {
		_ = sftpClient.Remove(tmpPath)
		return fmt.Errorf("关闭远程文件失败: %w", closeErr)
	}
	if err := sftpClient.Rename(tmpPath, remotePath); err != nil {
		_ = sftpClient.Remove(tmpPath)
		return fmt.Errorf("提交远程文件失败: %w", err)
	}
	return nil
}

func (a *App) downloadIfExists(remotePath, localPath string) (bool, error) {
	client, err := a.getSSHClient()
	if err != nil {
		return false, err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return false, fmt.Errorf("创建 SFTP 客户端失败: %w", err)
	}
	defer sftpClient.Close()
	if _, err := sftpClient.Stat(remotePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
		return false, err
	}
	remote, err := sftpClient.Open(remotePath)
	if err != nil {
		return false, err
	}
	defer remote.Close()
	local, err := os.OpenFile(localPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer local.Close()
	if _, err := io.Copy(local, remote); err != nil {
		return false, err
	}
	return true, nil
}

func remoteDir(remotePath string) string {
	idx := strings.LastIndex(remotePath, "/")
	if idx <= 0 {
		return ""
	}
	return remotePath[:idx]
}
