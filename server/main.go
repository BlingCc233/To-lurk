package main

import (
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// --- 数据库模型 ---
// ClipboardEntry 用于存储剪贴板历史
type ClipboardEntry struct {
	ID        uint   `gorm:"primaryKey"`
	Content   string `gorm:"type:text"`
	CreatedAt time.Time
}

// ScreenshotEntry 用于存储截图信息
type ScreenshotEntry struct {
	ID        uint   `gorm:"primaryKey"`
	FilePath  string // 存储截图文件的路径
	CreatedAt time.Time
}

var db *gorm.DB

// --- 截图管理相关常量 ---
const MAX_SCREENSHOTS = 100

// --- 主函数和路由设置 ---
func main() {
	// 初始化数据库
	var err error
	db, err = gorm.Open(sqlite.Open("monitor.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}

	// 自动迁移数据库结构
	db.AutoMigrate(&ClipboardEntry{}, &ScreenshotEntry{})

	// 确保截图存储目录存在
	if err := os.MkdirAll("./screenshots", 0755); err != nil {
		log.Fatalf("无法创建截图存储目录: %v", err)
	}

	// 初始化 Gin 引擎
	r := gin.Default()

	// --- API 路由 ---
	api := r.Group("/api")
	{
		// 上传数据
		api.POST("/clipboard", handlePostClipboard)
		api.POST("/screenshot", handlePostScreenshot)

		// 获取最新数据
		api.GET("/latest/clipboard", handleGetLatestClipboard)
		api.GET("/latest/screenshot", handleGetLatestScreenshot)
	}

	log.Println("服务器正在启动，监听端口 :5656")
	// 监听在 0.0.0.0 上，以便从局域网内其他机器访问
	r.Run(":5656")
}

// --- 截图管理函数 ---
// 清理过量的截图文件
func cleanupOldScreenshots() error {
	var count int64

	// 获取当前截图总数
	if err := db.Model(&ScreenshotEntry{}).Count(&count).Error; err != nil {
		return err
	}

	// 如果超过最大数量，删除最早的截图
	if count > MAX_SCREENSHOTS {
		deleteCount := count - MAX_SCREENSHOTS

		// 获取最早的截图记录
		var oldEntries []ScreenshotEntry
		if err := db.Order("created_at asc").Limit(int(deleteCount)).Find(&oldEntries).Error; err != nil {
			return err
		}

		// 删除文件和数据库记录
		for _, entry := range oldEntries {
			// 删除物理文件
			if err := os.Remove(entry.FilePath); err != nil {
				log.Printf("警告：删除文件失败 %s: %v", entry.FilePath, err)
			}

			// 从数据库删除记录
			if err := db.Delete(&entry).Error; err != nil {
				log.Printf("警告：删除数据库记录失败 ID:%d: %v", entry.ID, err)
			}
		}

		log.Printf("已清理 %d 个旧截图文件", len(oldEntries))
	}

	return nil
}

// --- 处理器函数 (Handlers) ---
// 处理上传的剪贴板内容
func handlePostClipboard(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求内容"})
		return
	}

	content := string(body)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容不能为空"})
		return
	}

	entry := ClipboardEntry{Content: content}
	result := db.Create(&entry)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法保存到数据库"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "剪贴板内容已保存"})
}

// 处理上传的屏幕截图
func handlePostScreenshot(c *gin.Context) {
	file, err := c.FormFile("upload")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取上传文件: " + err.Error()})
		return
	}

	// 创建一个唯一的文件名
	filename := filepath.Join("screenshots", time.Now().Format("20060102150405.999")+".png")

	// 保存文件
	if err := c.SaveUploadedFile(file, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法保存文件: " + err.Error()})
		return
	}

	// 将文件路径存入数据库
	entry := ScreenshotEntry{FilePath: filename}
	result := db.Create(&entry)
	if result.Error != nil {
		// 如果数据库保存失败，最好把刚才存的文件也删掉，避免产生孤立文件
		os.Remove(filename)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法将文件信息保存到数据库"})
		return
	}

	// 保存成功后，检查并清理过量的截图
	if err := cleanupOldScreenshots(); err != nil {
		log.Printf("清理旧截图时出错: %v", err)
		// 不返回错误，因为当前截图已经成功保存
	}

	c.JSON(http.StatusOK, gin.H{"message": "截图已保存", "path": filename})
}

// 获取最新的剪贴板内容
func handleGetLatestClipboard(c *gin.Context) {
	var entry ClipboardEntry
	// 按创建时间倒序，获取第一条记录
	result := db.Order("created_at desc").First(&entry)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有找到剪贴板记录"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询数据库失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": entry.Content, "timestamp": entry.CreatedAt})
}

// 获取最新的屏幕截图
func handleGetLatestScreenshot(c *gin.Context) {
	var entry ScreenshotEntry
	result := db.Order("created_at desc").First(&entry)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有找到截图记录"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询数据库失败"})
		return
	}

	// 直接将图片文件作为响应返回
	c.File(entry.FilePath)
}
