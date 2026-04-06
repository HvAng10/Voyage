package services

import (
	"context"
	"testing"
)

func TestParseSalesTrafficByDate(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// 模拟清洗服务 (网络请求 client 可留 nil 因为此处只验证 parse 清洗引擎)
	ds := NewDataKioskService(db, nil)
	
	// 放发前置的外键实体来接纳数据
	_, _ = db.Exec(`INSERT INTO accounts (id, name, seller_id, region) VALUES (1, 'Test', '1', 'na')`)
	_, _ = db.Exec(`INSERT INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone) VALUES ('US-MARKET', 'US', 'US', 'USD', 'NA', 'UTC')`)

	// 1. 使用极其典型的 Amazon Analytics SP-API 返回的 JSONL 块作为负载注入
	rawJSON := []byte(`{
		"analytics_salesAndTraffic_2023_11_15": {
			"salesAndTrafficByDate": [
				{
					"startDate": "2026-04-05",
					"marketplaceId": "US-MARKET",
					"orderedProductSales": {"amount": 185.0, "currencyCode": "USD"},
					"unitsOrdered": 12,
					"pageViews": 200,
					"sessions": 85,
					"unitSessionPercentage": 14.11
				}
			]
		}
	}`)

	count := 0
	err := ds.parseSalesTrafficByDate(context.Background(), 1, "US-MARKET", rawJSON, &count)
	if err != nil {
		t.Fatalf("Parser strictly failed to decode standard amazon structure: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 payload integrated over JSONL block wrapper, got %d", count)
	}

	// 验证 SQLite 底层是否被安稳摄入
	var sales float64
	var units, views, sessions int
	err = db.QueryRow("SELECT ordered_product_sales, units_ordered, page_views, sessions FROM sales_traffic_daily WHERE account_id=1").
		Scan(&sales, &units, &views, &sessions)
	
	if err != nil {
		t.Fatalf("Failed to fetch verified metrics record: %v", err)
	}

	if sales != 185.0 || units != 12 || views != 200 || sessions != 85 {
		t.Errorf("Data anomaly! Parse skew: Sales %f Units %d", sales, units)
	}
}

func TestParseSalesTrafficByAsin(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ds := NewDataKioskService(db, nil)

	// 补充外键表以接受插入，否则将遇到 constraint 阻挡
	_, _ = db.Exec(`INSERT INTO accounts (id, name, seller_id, region) VALUES (1, 'Test', '1', 'na')`)
	_, _ = db.Exec(`INSERT INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone) VALUES ('US-MARKET', 'US', 'US', 'USD', 'NA', 'UTC')`)

	rawJSON := []byte(`{
		"analytics_salesAndTraffic_2023_11_15": {
			"salesAndTrafficByAsin": [
				{
					"childAsin": "B00XXXXXXY",
					"parentAsin": "B00XXXXXXX",
					"orderedProductSales": {"amount": 55.5},
					"unitsOrdered": 2,
					"pageViews": 100,
					"featuredOfferBuyBoxPercentage": 98.5
				}
			]
		}
	}`)

	err := ds.parseSalesTrafficByAsin(context.Background(), 1, "US-MARKET", rawJSON)
	if err != nil {
		t.Fatalf("ASIN Parser strictly failed to decode: %v", err)
	}

	var asin string
	var bb float64
	err = db.QueryRow("SELECT asin, buy_box_percentage FROM sales_traffic_by_asin WHERE account_id=1").
		Scan(&asin, &bb)

	if err != nil {
		t.Fatalf("Failed to fetch verified ASIN metrics: %v", err)
	}

	if asin != "B00XXXXXXY" {
		t.Errorf("Excepted distinct child asin extracted, got %v", asin)
	}
	if bb != 98.5 {
		t.Errorf("Buy box dropped during cleaning map. Expected 98.5, got %f", bb)
	}
}
