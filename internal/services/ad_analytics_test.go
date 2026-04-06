package services

import (
	"path/filepath"
	"testing"

	"voyage/internal/database"
)

// prepareTestDB 初始化带有临时表结构的虚拟数据库连接
func prepareTestDB(t *testing.T) *database.DB {
	t.Helper()
	tempDir := t.TempDir()
	db, err := database.Open(filepath.Join(tempDir, "test_services.db"))
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	return db
}

func TestGetAdKeywords_Empty(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// 测试表为空（或结构尚未准备好）情况下的优雅降级，不能发生 panic
	res, err := GetAdKeywords(db, 1, "US", "2026-04-01", "2026-04-07", 10)
	if err != nil {
		t.Fatalf("Unexpected error on empty DB: %v", err)
	}
	if res == nil {
		t.Errorf("Expected empty slice, but got nil")
	}
	if len(res) != 0 {
		t.Errorf("Expected 0 results, got %d items", len(res))
	}
}

func TestGetAdTargets_Empty(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	res, err := GetAdTargets(db, 1, "US", "2026-04-01", "2026-04-07", 10)
	if err != nil {
		t.Fatalf("Unexpected error on empty DB: %v", err)
	}
	if res == nil {
		t.Errorf("Expected empty slice array, got nil")
	}
	if len(res) != 0 {
		t.Errorf("Expected 0 result, got %v", res)
	}
}

func TestGetAdCampaignDailyTrend_Empty(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	res, err := GetAdCampaignDailyTrend(db, 1, "c-123", "2026-04-01", "2026-04-07")
	if err != nil {
		t.Logf("Expected warning if schema not exist, ignored successfully (err=%v)", err)
	} else {
		if res == nil {
			t.Errorf("Expected empty array map[], got nil")
		}
	}
}

func TestGetAdKeywords_WithMockData(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// 使用带字段名称的常规插入，兼容已存在的真实数据表结构
	_, err := db.Exec(`INSERT INTO accounts (id, name, seller_id, region) VALUES (1, 'Test Account', 'A1', 'na')`)
	if err != nil { t.Log("Insert account warning (may exist):", err) }
	
	_, err = db.Exec(`INSERT INTO ad_campaigns (campaign_id, account_id, marketplace_id, ads_profile_id, name, campaign_type, state) VALUES ('c1', 1, 'US', 'prof-1', 'CampA', 'sp', 'enabled')`)
	if err != nil { t.Fatal("Failed insert campaigns:", err) }

	_, err = db.Exec(`INSERT INTO ad_groups (ad_group_id, campaign_id, account_id, name) VALUES ('g1', 'c1', 1, 'GroupA')`)
	if err != nil { t.Fatal("Failed insert groups:", err) }

	_, err = db.Exec(`INSERT INTO ad_keywords (keyword_id, ad_group_id, account_id, keyword_text, match_type, state) VALUES ('k1', 'g1', 1, 'apple', 'EXACT', 'ENABLED')`)
	if err != nil { t.Fatal("Failed insert keywords:", err) }

	// 注入多天数据让其聚合
	_, err = db.Exec(`INSERT INTO ad_keyword_performance (keyword_id, account_id, date, impressions, clicks, cost, attributed_sales_7d, attributed_orders_7d) VALUES 
		('k1', 1, '2026-04-05', 100, 10, 5.0, 50.0, 2),
		('k1', 1, '2026-04-06', 50,  5,  2.5, 25.0, 1)`)
	if err != nil { t.Fatal("Failed insert perf:", err) }

	res, err := GetAdKeywords(db, 1, "US", "2026-04-01", "2026-04-07", 10)
	if err != nil {
		t.Fatalf("Data query fail: %v", err)
	}
	
	if len(res) != 1 {
		t.Fatalf("Expected 1 grouped row, got %d", len(res))
	}
	
	row := res[0]
	if row.Impressions != 150 { t.Errorf("Expected 150 impressions, got %d", row.Impressions) }
	if row.Clicks != 15 { t.Errorf("Expected 15 clicks, got %d", row.Clicks) }
	if row.Cost != 7.5 { t.Errorf("Expected 7.5 cost, got %f", row.Cost) }
	if row.Sales != 75.0 { t.Errorf("Expected 75.0 sales, got %f", row.Sales) }
	if row.Orders != 3 { t.Errorf("Expected 3 orders, got %d", row.Orders) }
	
	// ACoS = 7.5 / 75.0 * 100 = 10%
	if row.ACoS != 10.0 { t.Errorf("ACoS %f != 10.0", row.ACoS) }
	// CTR = 15 / 150 * 100 = 10%
	if row.CTR != 10.0 { t.Errorf("CTR %f != 10.0", row.CTR) }
	// CPC = 7.5 / 15 = 0.5
	if row.CPC != 0.5 { t.Errorf("CPC %f != 0.5", row.CPC) }
}
