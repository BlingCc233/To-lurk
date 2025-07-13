package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- 我们将在这里添加核心功能 ---

// 定义服务器地址。请根据你的实际情况修改。
const serverAddress = "http://localhost:5656"

// ClipboardResponse 用于解析从服务器返回的剪贴板JSON数据
type ClipboardResponse struct {
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// GetLatestClipboard 从服务器获取最新的剪贴板内容
// GetLatestClipboard 从服务器获取最新的剪贴板内容
func (a *App) GetLatestClipboard() (ClipboardResponse, error) {
	// 这是我们将返回给前端的最终数据结构
	var frontendResponse ClipboardResponse

	// 定义一个临时的内部结构体，用于解析从服务器收到的原始JSON。
	// 这个结构体的Timestamp字段是time.Time，与服务器API的定义匹配。
	var serverResponse struct {
		Content   string    `json:"content"`
		Timestamp time.Time `json:"timestamp"`
	}

	// 构造请求URL
	url := fmt.Sprintf("%s/api/latest/clipboard", serverAddress)

	// 发送HTTP GET请求
	resp, err := http.Get(url)
	if err != nil {
		return frontendResponse, fmt.Errorf("请求服务器失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return frontendResponse, fmt.Errorf("服务器返回错误状态: %s", resp.Status)
	}

	// 读取响应体
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return frontendResponse, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 将服务器返回的JSON解析到我们的临时结构体中
	err = json.Unmarshal(body, &serverResponse)
	if err != nil {
		return frontendResponse, fmt.Errorf("解析JSON失败: %w", err)
	}

	// --- 核心转换逻辑 ---
	// 填充我们将要返回给前端的结构体
	frontendResponse.Content = serverResponse.Content
	// 将time.Time对象格式化为ISO 8601格式的字符串 (例如 "2025-07-12T21:30:00Z")
	// 这种格式可以被JavaScript的 new Date() 轻松解析
	frontendResponse.Timestamp = serverResponse.Timestamp.Format(time.RFC3339)

	return frontendResponse, nil
}

// GetLatestScreenshot 获取最新的屏幕截图并返回Base64编码的字符串
// 返回Base64字符串是为了方便前端直接在 <img> 标签中使用
func (a *App) GetLatestScreenshot() (string, error) {
	// 构造请求URL
	url := fmt.Sprintf("%s/api/latest/screenshot", serverAddress)

	// 发送HTTP GET请求
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("请求服务器失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("服务器返回错误状态: %s", resp.Status)
	}

	// 读取图片二进制数据
	imageBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取图片数据失败: %w", err)
	}

	// 将图片数据编码为Base64
	base64String := base64.StdEncoding.EncodeToString(imageBytes)

	// 获取图片MIME类型，这里我们假设是PNG，你也可以从http header `Content-Type` 获取
	mimeType := "image/png"
	// http.DetectContentType(imageBytes) // 更可靠的方式

	// 构造可以在HTML中直接使用的Data URL
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64String)

	return dataURL, nil
}
