package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings" // 导入 strings 包用于处理字符串
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// CookieEntry 用于存储单个 cookie 信息
type CookieEntry struct {
	ID        uint   `gorm:"primaryKey"`
	Domain    string `gorm:"uniqueIndex:idx_domain_name"` // 与 Name 组合成唯一索引
	Name      string `gorm:"uniqueIndex:idx_domain_name"` // 与 Domain 组合成唯一索引
	Value     string `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// --- 新增: 用于最终 JSON 输出的 Cookie 结构 ---
// OutputCookie 定义了 GET /api/cookies 接口返回的子项格式
type OutputCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

var db *gorm.DB

// --- 截图管理相关常量 ---
const MAX_SCREENSHOTS = 100
const CookiePWD = "cc233"

// --- 主函数和路由设置 ---
func main() {
	// 初始化数据库
	var err error
	db, err = gorm.Open(sqlite.Open("monitor.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}

	// 自动迁移数据库结构
	db.AutoMigrate(&ClipboardEntry{}, &ScreenshotEntry{}, &CookieEntry{})

	// 确保截图存储目录存在
	if err := os.MkdirAll("./screenshots", 0755); err != nil {
		log.Panicf("无法创建截图存储目录: %v", err)
	}

	// 初始化 Gin 引擎
	r := gin.Default()

	// --- API 路由 ---
	api := r.Group("/api")
	{
		// 上传数据
		api.POST("/clipboard", handlePostClipboard)
		api.POST("/screenshot", handlePostScreenshot)
		api.POST("/cookies", handlePostCookies)

		// 获取数据
		api.GET("/latest/clipboard", handleGetLatestClipboard)
		api.GET("/latest/screenshot", handleGetLatestScreenshot)
		api.GET("/cookies", handleGetCookies) // 使用新的 handleGetCookies 函数
	}

	log.Println("服务器正在启动，监听端口 :5656")
	r.Run(":5656")
}

// --- 截图管理函数 ---
func cleanupOldScreenshots() error {
	var count int64
	if err := db.Model(&ScreenshotEntry{}).Count(&count).Error; err != nil {
		return err
	}

	if count > MAX_SCREENSHOTS {
		deleteCount := count - MAX_SCREENSHOTS
		var oldEntries []ScreenshotEntry
		if err := db.Order("created_at asc").Limit(int(deleteCount)).Find(&oldEntries).Error; err != nil {
			return err
		}

		for _, entry := range oldEntries {
			if err := os.Remove(entry.FilePath); err != nil {
				log.Printf("警告：删除文件失败 %s: %v", entry.FilePath, err)
			}
			if err := db.Delete(&entry).Error; err != nil {
				log.Printf("警告：删除数据库记录失败 ID:%d: %v", entry.ID, err)
			}
		}
		log.Printf("已清理 %d 个旧截图文件", len(oldEntries))
	}
	return nil
}

// --- 处理器函数 (Handlers) ---

// getGroupingKey 根据广义域名规则计算分组键
func getGroupingKey(domain string) string {
	if strings.HasPrefix(domain, "www.") {
		return strings.TrimPrefix(domain, "www.")
	}
	if strings.HasPrefix(domain, ".") {
		return strings.TrimPrefix(domain, ".")
	}
	return domain
}

// handleGetCookies 检索、分组并按要求格式化所有 cookie
func handleGetCookies(c *gin.Context) {
	pwd := c.Query("pwd")
	if pwd != CookiePWD {
		c.String(http.StatusBadRequest, "invalid")
		return
	}

	var allCookies []CookieEntry

	// 1. 从数据库中获取所有 cookie 记录
	if err := db.Find(&allCookies).Error; err != nil {
		log.Printf("数据库查询 cookies 失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询数据库失败"})
		return
	}

	// 2. 使用 map 按广义域名对 cookie 进行分组
	// 键是分组键 (例如 "bilibili.com")
	// 值是符合该分组的 cookie 列表 (OutputCookie 格式)
	groupedCookies := make(map[string][]OutputCookie)

	for _, cookie := range allCookies {
		key := getGroupingKey(cookie.Domain)

		// 创建仅包含 domain, name, value 的简化对象
		outputCookie := OutputCookie{
			Domain: cookie.Domain, // 在输出中保留原始域名
			Name:   cookie.Name,
			Value:  cookie.Value,
		}

		// 将简化后的 cookie 对象添加到对应的分组中
		groupedCookies[key] = append(groupedCookies[key], outputCookie)
	}

	// 3. 将 map 中的所有分组（值）转换为一个大的列表（列表的列表）
	finalResult := make([][]OutputCookie, 0, len(groupedCookies))
	for _, group := range groupedCookies {
		finalResult = append(finalResult, group)
	}

	// 4. 返回最终的 JSON 结构
	c.JSON(http.StatusOK, finalResult)
}

func handlePostCookies(c *gin.Context) {
	var cookies []CookieEntry
	if err := c.ShouldBindJSON(&cookies); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 JSON 格式: " + err.Error()})
		return
	}
	if len(cookies) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cookie 列表不能为空"})
		return
	}
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&cookies)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法将 cookie 保存到数据库: " + result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功处理 %d 条 cookie 记录", len(cookies))})
}

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

func handlePostScreenshot(c *gin.Context) {
	file, err := c.FormFile("upload")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取上传文件: " + err.Error()})
		return
	}
	filename := filepath.Join("screenshots", time.Now().Format("20060102150405.999")+".png")
	if err := c.SaveUploadedFile(file, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法保存文件: " + err.Error()})
		return
	}
	entry := ScreenshotEntry{FilePath: filename}
	result := db.Create(&entry)
	if result.Error != nil {
		os.Remove(filename)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法将文件信息保存到数据库"})
		return
	}
	if err := cleanupOldScreenshots(); err != nil {
		log.Printf("清理旧截图时出错: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "截图已保存", "path": filename})
}

func handleGetLatestClipboard(c *gin.Context) {
	var entry ClipboardEntry
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
	c.File(entry.FilePath)
}
