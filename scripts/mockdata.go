// 独立模拟数据生成脚本（命令行版）
// 用法：go run scripts/mockdata.go
// 功能与 internal/services/mockdata.go 中的 GenerateMockData 完全一致
// 生成双账户(US+UK)的 90 天完整演示数据
package main

import (
	"fmt"
	"log"

	"voyage/internal/config"
	"voyage/internal/database"
	"voyage/internal/services"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("无法获取配置: %v", err)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库打开失败: %v", err)
	}
	defer db.Close()

	log.Printf("📂 数据库路径: %s", cfg.DBPath)
	log.Println("🚀 开始生成模拟数据...")

	result := services.GenerateMockData(db)

	if result["success"] == true {
		log.Println("")
		log.Println("═══════════════════════════════════════════════")
		log.Println("🎉 模拟数据生成完成！")
		log.Println("═══════════════════════════════════════════════")
		log.Printf("  %s", result["message"])
		log.Printf("  📂 数据库: %s", cfg.DBPath)
		fmt.Println("\n  ✅ 现在可以打开 Voyage 客户端查看所有页面和图表了！")
	} else {
		log.Fatalf("❌ 生成失败: %s", result["message"])
	}
}
