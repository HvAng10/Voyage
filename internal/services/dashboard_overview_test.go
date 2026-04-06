package services

import (
	"testing"
)

func TestGetInventoryOverview(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ds := NewDashboardService(db)

	// 测试：空数据或尚未存在的聚合场景
	ov, err := ds.GetInventoryOverview(1, "US")
	if err != nil {
		t.Fatalf("GetInventoryOverview init error: %v", err)
	}
	if ov == nil || ov.TotalSKU != 0 {
		t.Errorf("Expected 0 TotalSKU, got %#v", ov)
	}

	// 插入真实情境结构数据（健康与断货SKU共存情况）
	_, _ = db.Exec(`INSERT INTO accounts (id, name, seller_id, region) VALUES (1, 'Test', '1', 'na')`)
	_, _ = db.Exec(`INSERT INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone) VALUES ('US', 'US', 'US', 'USD', 'NA', 'UTC')`)

	_, err = db.Exec(`INSERT INTO inventory_snapshots (account_id, marketplace_id, snapshot_date, seller_sku, fulfillable_qty) VALUES
		(1, 'US', '2026-04-01', 'sku-no-stock', 0),
		(1, 'US', '2026-04-01', 'sku-stock-good', 100)`)
	if err != nil { t.Fatal("Failed insert into inventory_snapshots:", err) }

	ov, err = ds.GetInventoryOverview(1, "US")
	if err != nil {
		t.Fatalf("GetInventoryOverview after insert error: %v", err)
	}
	
	if ov.TotalSKU != 2 {
		t.Errorf("Expected exactly 2 distinct SKUs tracked, got %d", ov.TotalSKU)
	}
	if ov.CriticalCount != 1 {
		t.Errorf("Expected 1 critical sku (0 stock), got %d", ov.CriticalCount)
	}
	if ov.TotalFulfillable != 100 {
		t.Errorf("Expected 100 total fulfillable volume, got %d", ov.TotalFulfillable)
	}
}

func TestGetAdOverview(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ds := NewDashboardService(db)

	ov, err := ds.GetAdOverview(1, "US", "2026-04-01", "2026-04-07")
	if err != nil {
		t.Fatalf("GetAdOverview error on empty structure: %v", err)
	}

	// 组合关联多表，填注广告开销流水
	_, _ = db.Exec(`INSERT INTO accounts (id, name, seller_id, region) VALUES (1, 'Test', '1', 'na')`)
	_, _ = db.Exec(`INSERT INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone) VALUES ('US', 'US', 'US', 'USD', 'NA', 'UTC')`)
	_, err = db.Exec(`INSERT INTO ad_campaigns (campaign_id, account_id, marketplace_id, ads_profile_id, name, campaign_type, state) VALUES 
		('c1', 1, 'US', 'p1', 'Camp1', 'sp', 'enabled')`)
	if err != nil { t.Fatal("Failed insert into ad_campaigns:", err) }
	
	_, err = db.Exec(`INSERT INTO ad_performance_daily (campaign_id, account_id, date, cost, attributed_sales_7d) VALUES 
		('c1', 1, '2026-04-05', 10.0, 100.0)`)
	if err != nil { t.Fatal("Failed insert into ad_performance_daily:", err) }

	ov, err = ds.GetAdOverview(1, "US", "2026-04-01", "2026-04-07")
	if err != nil {
		t.Fatalf("GetAdOverview after insert logic error: %v", err)
	}

	if ov.TotalCampaigns != 1 {
		t.Errorf("Expected 1 tracked campaign, got %d", ov.TotalCampaigns)
	}
	if ov.TotalSpend != 10.0 {
		t.Errorf("Expected ad spend 10.0, got %f", ov.TotalSpend)
	}
	if ov.TotalSales != 100.0 {
		t.Errorf("Expected ad sales 100.0, got %f", ov.TotalSales)
	}
	if ov.AvgACoS != 10.0 {
		t.Errorf("Expected total avg acos 10.0%%, got %f", ov.AvgACoS)
	}
	if ov.AvgROAS != 10.0 {
		t.Errorf("Expected total avg roas 10.0, got %f", ov.AvgROAS)
	}
}
