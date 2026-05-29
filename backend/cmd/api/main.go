// Command api 启动无人机智能管控系统后端服务。
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"dr600ab-api/internal/app"
	"dr600ab-api/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	server, err := app.New(cfg)
	if err != nil {
		log.Fatalf("初始化后端服务失败: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("无人机智能管控系统后端已启动: %s", cfg.Addr)
		errCh <- server.Listen(cfg.Addr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("收到退出信号 %s，正在关闭服务", sig)
	case err := <-errCh:
		if err != nil {
			log.Fatalf("后端服务退出: %v", err)
		}
	}

	if err := server.Shutdown(); err != nil {
		log.Printf("关闭服务时发生错误: %v", err)
	}
}
