// Package config 负责应用配置和 API 凭证的管理
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppConfig 应用全局配置
type AppConfig struct {
	// 应用数据根目录
	DataDir string
	// 数据库文件路径
	DBPath string
	// 分类子目录
	DBDir      string // 数据库文件目录
	ReportsDir string // PDF 报告输出
	BackupsDir string // 数据库备份
	ExportsDir string // CSV 导出文件
	LogsDir    string // 日志文件
	// 加密器实例
	Encryptor *Encryptor
}

// New 创建应用配置，自动确定数据目录并创建分类子目录
func New() (*AppConfig, error) {
	dataDir, err := getDataDir()
	if err != nil {
		return nil, err
	}

	// 定义分类子目录
	dbDir      := filepath.Join(dataDir, "data")
	reportsDir := filepath.Join(dataDir, "reports")
	backupsDir := filepath.Join(dataDir, "backups")
	exportsDir := filepath.Join(dataDir, "exports")
	logsDir    := filepath.Join(dataDir, "logs")

	// 确保所有目录存在
	for _, dir := range []string{dataDir, dbDir, reportsDir, backupsDir, exportsDir, logsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}

	// 兼容旧版：自动迁移旧路径文件到新子目录
	newDBPath := filepath.Join(dbDir, "voyage.db")
	oldDBPath := filepath.Join(dataDir, "voyage.db")
	if _, err := os.Stat(oldDBPath); err == nil {
		// 旧数据库文件存在，检查新路径是否已有
		if _, err := os.Stat(newDBPath); os.IsNotExist(err) {
			// 迁移旧数据库到新目录
			os.Rename(oldDBPath, newDBPath)
			// 同时迁移 WAL 和 SHM 文件
			os.Rename(oldDBPath+"-wal", newDBPath+"-wal")
			os.Rename(oldDBPath+"-shm", newDBPath+"-shm")
		}
	}

	// 迁移旧版散落在根目录的备份文件
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() && len(name) > 14 && name[:14] == "voyage_backup_" {
				oldPath := filepath.Join(dataDir, name)
				newPath := filepath.Join(backupsDir, name)
				os.Rename(oldPath, newPath)
			}
		}
	}

	machineID := getMachineID()
	enc := NewEncryptor(machineID, "")

	return &AppConfig{
		DataDir:    dataDir,
		DBPath:     filepath.Join(dbDir, "voyage.db"),
		DBDir:      dbDir,
		ReportsDir: reportsDir,
		BackupsDir: backupsDir,
		ExportsDir: exportsDir,
		LogsDir:    logsDir,
		Encryptor:  enc,
	}, nil
}

// getDataDir 获取平台对应的应用数据目录
func getDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = os.Getenv("USERPROFILE")
		}
		return filepath.Join(appData, "Voyage"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Voyage"), nil
	default: // Linux
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configDir = filepath.Join(home, ".config")
		}
		return filepath.Join(configDir, "voyage"), nil
	}
}

// getMachineID 获取机器唯一标识，用于派生加密密钥
func getMachineID() string {
	// 读取系统机器 ID 文件（Linux/macOS）
	paths := []string{
		"/etc/machine-id",                     // Linux
		"/var/lib/dbus/machine-id",            // Linux 备用
		"/Library/Preferences/SystemConfiguration/preferences.plist", // macOS（近似）
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return string(data)
		}
	}

	// Windows：使用 ComputerName + USERNAME
	computerName := os.Getenv("COMPUTERNAME")
	userName := os.Getenv("USERNAME")
	if computerName != "" {
		return computerName + "::" + userName
	}

	// 兜底：使用主机名
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "voyage-default-machine-id"
}
