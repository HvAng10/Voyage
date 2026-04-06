package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewAppConfig(t *testing.T) {
	// 创建临时目录环境防止污染真实环境
	tempDir := t.TempDir()

	// 备份并替换环境变量
	origAppData := os.Getenv("APPDATA")
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	
	defer func() {
		os.Setenv("APPDATA", origAppData)
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()

	if runtime.GOOS == "windows" {
		os.Setenv("APPDATA", tempDir)
		os.Setenv("USERPROFILE", tempDir)
	} else {
		os.Setenv("HOME", tempDir)
		os.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	cfg, err := New()
	if err != nil {
		t.Fatalf("Failed to create new config: %v", err)
	}

	if cfg.DataDir == "" {
		t.Error("DataDir should not be empty")
	}

	// 校验所有子目录是否真的被创建成功
	dirsToCheck := []string{
		cfg.DataDir,
		cfg.DBDir,
		cfg.ReportsDir,
		cfg.BackupsDir,
		cfg.ExportsDir,
		cfg.LogsDir,
	}

	for _, dir := range dirsToCheck {
		stat, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("预期创建目录 %s，但测试表示不存在", dir)
		} else if err != nil {
			t.Errorf("访问目录 %s 时出错: %v", dir, err)
		} else if !stat.IsDir() {
			t.Errorf("路径 %s 应该是一个目录", dir)
		}
	}

	// 验证路径拼接合理性
	if filepath.Base(cfg.DBDir) != "data" {
		t.Errorf("预期 DBDir 的基础目录名为 data, 得到 %s", filepath.Base(cfg.DBDir))
	}
	if filepath.Base(cfg.DBPath) != "voyage.db" {
		t.Errorf("预期 DBPath 的文件名为 voyage.db, 得到 %s", filepath.Base(cfg.DBPath))
	}

	// 验证 Encryptor 被成功初始化
	if cfg.Encryptor == nil {
		t.Error("预期 Encryptor 会被初始化，但它是 nil")
	}
}

func TestGetDataDir(t *testing.T) {
	// 防止影响真实数据
	tempDir := t.TempDir()
	
	// 测试 Windows 下的环境变量优先级
	if runtime.GOOS == "windows" {
		origAppData := os.Getenv("APPDATA")
		defer os.Setenv("APPDATA", origAppData)
		
		os.Setenv("APPDATA", tempDir)
		dir, err := getDataDir()
		if err != nil {
			t.Fatalf("getDataDir() windows error: %v", err)
		}
		expected := filepath.Join(tempDir, "Voyage")
		if dir != expected {
			t.Errorf("Windows APPDATA expected %s, got %s", expected, dir)
		}
	}
}
