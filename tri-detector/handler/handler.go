package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"tri-detector/client"
	"tri-detector/parser"
)

// ReadLoop 持续从客户端读取行数据并解析输出
func ReadLoop(c *client.SerialClient) {
	c.ReadLoop(func(line string) {
		msg, err := parser.ParseLine(line)
		if err != nil {
			return
		}

		payload, err := json.Marshal(msg)
		if err != nil {
			return
		}

		fmt.Printf("[PARSED] %s\n", payload)
	})
}

// InputLoop 从 stdin 读取用户输入并通过客户端发送，返回 true 表示用户请求退出
func InputLoop(inputScanner *bufio.Scanner, c client.Client) bool {
	if !inputScanner.Scan() {
		if err := inputScanner.Err(); err != nil {
			log.Printf("读取输入失败: %v", err)
		}
		return true
	}

	line := strings.TrimSpace(inputScanner.Text())
	if strings.EqualFold(line, "exit") {
		fmt.Println("退出。")
		return true
	}
	if line == "" {
		return false
	}

	if err := c.Send(line); err != nil {
		log.Printf("发送失败: %v", err)
		return false
	}
	fmt.Printf("[TX] %q\n", line)
	return false
}

// NewInputScanner 创建标准输入扫描器
func NewInputScanner() *bufio.Scanner {
	return bufio.NewScanner(os.Stdin)
}
