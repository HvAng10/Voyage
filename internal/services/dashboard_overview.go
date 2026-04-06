// Package services 仪表盘扩展：库存和广告快速概览（用于仪表盘底部卡片）
package services

// InventoryOverview 库存概览（仪表盘用）
type InventoryOverview struct {
	TotalSKU         int `json:"totalSku"`
	CriticalCount    int `json:"criticalCount"` // 断货/紧急
	WarningCount     int `json:"warningCount"`  // 库存不足
	OkCount          int `json:"okCount"`       // 库存充足
	TotalFulfillable int `json:"totalFulfillable"`
}

// GetInventoryOverview 获取库存健康概览（最新快照）
// 注：使用只读连接，避免阻塞写入操作
func (s *DashboardService) GetInventoryOverview(accountID int64, marketplaceID string) (*InventoryOverview, error) {
	row := s.db.ReadQueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE
				WHEN i.fulfillable_qty = 0 THEN 1
				ELSE 0
			END) as critical,
			SUM(CASE
				WHEN i.fulfillable_qty > 0
				  AND COALESCE(
					(SELECT SUM(t.units_ordered)/30.0 FROM sales_traffic_by_asin t
					 WHERE t.asin=i.asin AND t.account_id=i.account_id
					   AND t.marketplace_id=i.marketplace_id AND t.date>=date('now','-30 days')),
					0) > 0
				  AND i.fulfillable_qty / COALESCE(
					(SELECT SUM(t.units_ordered)/30.0 FROM sales_traffic_by_asin t
					 WHERE t.asin=i.asin AND t.account_id=i.account_id
					   AND t.marketplace_id=i.marketplace_id AND t.date>=date('now','-30 days')),
					1) < 14
				THEN 1 ELSE 0
			END) as warning,
			COALESCE(SUM(i.fulfillable_qty), 0) as total_fulfillable
		FROM inventory_snapshots i
		WHERE i.account_id=? AND i.marketplace_id=?
		  AND i.snapshot_date=(SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id=? AND marketplace_id=?)
	`, accountID, marketplaceID, accountID, marketplaceID)

	var ov InventoryOverview
	var critical, warning int
	if err := row.Scan(&ov.TotalSKU, &critical, &warning, &ov.TotalFulfillable); err != nil {
		return &InventoryOverview{}, nil // 无数据时返回空结构
	}
	ov.CriticalCount = critical
	ov.WarningCount = warning
	ov.OkCount = ov.TotalSKU - critical - warning
	return &ov, nil
}

// AdOverview 广告概览（仪表盘用）
type AdOverview struct {
	TotalCampaigns int     `json:"totalCampaigns"`
	TotalSpend     float64 `json:"totalSpend"`
	TotalSales     float64 `json:"totalSales"`
	AvgACoS        float64 `json:"avgAcos"`
	AvgROAS        float64 `json:"avgRoas"`
}

// GetAdOverview 获取广告快速概览
// 修复：ad_performance_daily 无 marketplace_id 字段，通过 JOIN ad_campaigns 过滤
// 注意：广告数据延迟 T+3，dateEnd 应为 today-3天
func (s *DashboardService) GetAdOverview(accountID int64, marketplaceID, dateStart, dateEnd string) (*AdOverview, error) {
	row := s.db.ReadQueryRow(`
		SELECT
			COUNT(DISTINCT apd.campaign_id) as campaigns,
			COALESCE(SUM(apd.cost), 0)                as spend,
			COALESCE(SUM(apd.attributed_sales_7d), 0) as sales
		FROM ad_performance_daily apd
		JOIN ad_campaigns ac
			ON apd.campaign_id = ac.campaign_id AND apd.account_id = ac.account_id
		WHERE apd.account_id = ?
		  AND ac.marketplace_id = ?
		  AND apd.date >= ? AND apd.date <= ?
	`, accountID, marketplaceID, dateStart, dateEnd)

	var ov AdOverview
	if err := row.Scan(&ov.TotalCampaigns, &ov.TotalSpend, &ov.TotalSales); err != nil {
		return &AdOverview{}, nil
	}
	if ov.TotalSales > 0 {
		ov.AvgACoS = (ov.TotalSpend / ov.TotalSales) * 100
	}
	if ov.TotalSpend > 0 {
		ov.AvgROAS = ov.TotalSales / ov.TotalSpend
	}
	return &ov, nil
}

