package services

import (
	"testing"
)

func TestMockDataGenerateAndClear(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// 1. 生成 Mock 模拟数据
	resGen := GenerateMockData(db)
	if success, ok := resGen["success"].(bool); !ok || !success {
		t.Fatalf("GenerateMockData failed: %v", resGen)
	}

	// 验证模拟数据已经正确写入到各个系统级表内
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM accounts WHERE name LIKE 'MockStore%'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query accounts: %v", err)
	}
	if count == 0 {
		t.Errorf("Expected mock accounts to be created (MockStoreUS/UK), but count=0")
	}

	// 2. 清理 Mock 模拟数据 (安全退出不留痕迹验证)
	resClear := ClearMockData(db)
	if success, ok := resClear["success"].(bool); !ok || !success {
		t.Fatalf("ClearMockData failed: %v", resClear)
	}

	// 重新校验表内残留情况
	err = db.QueryRow("SELECT COUNT(*) FROM accounts WHERE name LIKE 'MockStore%'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query accounts after purge: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected mock accounts to be securely wiped, but count=%d remains", count)
	}
}
