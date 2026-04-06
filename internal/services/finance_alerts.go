// Package services - 财务数据查询 + 预警服务
package services

import (
	"fmt"
	"log/slog"
	"time"

	"voyage/internal/database"
)

// ── 财务查询 ─────────────────────────────────────────────

// FinanceService 财务数据查询服务
type FinanceService struct {
	db *database.DB
}

func NewFinanceService(db *database.DB) *FinanceService {
	return &FinanceService{db: db}
}

// FinanceSummary 财务摘要
type FinanceSummary struct {
	TotalRevenue    float64 `json:"totalRevenue"`
	TotalRefunds    float64 `json:"totalRefunds"`
	TotalFees       float64 `json:"totalFees"`    // 平台佣金 + FBA费用
	TotalAdSpend    float64 `json:"totalAdSpend"`
	TotalCOGS       float64 `json:"totalCogs"`    // 用户录入成本
	GrossProfit     float64 `json:"grossProfit"`  // 毛利润（含费用前）
	NetProfit       float64 `json:"netProfit"`    // 净利润
	ProfitMargin    float64 `json:"profitMargin"` // 净利率
	Currency        string  `json:"currency"`
	DateStart       string  `json:"dateStart"`
	DateEnd         string  `json:"dateEnd"`
}

// GetFinanceSummary 获取财务汇总数据
func (s *FinanceService) GetFinanceSummary(accountID int64, marketplaceID, dateStart, dateEnd string) (*FinanceSummary, error) {
	summary := &FinanceSummary{
		Currency:  "USD",
		DateStart: dateStart,
		DateEnd:   dateEnd,
	}

	s.db.QueryRow("SELECT currency_code FROM marketplace WHERE marketplace_id = ?",
		marketplaceID).Scan(&summary.Currency)

	// 从 Data Kiosk 销售数据获取收入
	s.db.QueryRow(`
		SELECT COALESCE(SUM(ordered_product_sales), 0)
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?
	`, accountID, marketplaceID, dateStart, dateEnd).Scan(&summary.TotalRevenue)

	// 从财务事件获取退款和费用
	// ⚠️ 必须在后续 QueryRow 之前手动关闭 rows，避免 MaxOpenConns=1 死锁
	rows, err := s.db.Query(`
		SELECT event_type, COALESCE(SUM(total_amount), 0)
		FROM financial_events
		WHERE account_id=? AND marketplace_id=?
		  AND posted_date>=? AND posted_date<=?
		GROUP BY event_type
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59")
	if err == nil {
		for rows.Next() {
			var evType string
			var amount float64
			if err := rows.Scan(&evType, &amount); err != nil { slog.Warn("扫描财务事件行失败", "error", err); continue }
			switch evType {
			case "Refund":
				summary.TotalRefunds += amount
			case "ServiceFee", "FBAFee":
				summary.TotalFees += amount
			}
		}
		rows.Close() // 提前关闭，释放连接给后续 QueryRow
	}

	// 广告花费
	s.db.QueryRow(`
		SELECT COALESCE(SUM(apd.cost), 0)
		FROM ad_performance_daily apd
		JOIN ad_campaigns ac ON apd.campaign_id=ac.campaign_id AND apd.account_id=ac.account_id
		WHERE apd.account_id=? AND ac.marketplace_id=? AND apd.date>=? AND apd.date<=?
	`, accountID, marketplaceID, dateStart, dateEnd).Scan(&summary.TotalAdSpend)

	// COGS（用户录入成本，联 product_costs 表）
	s.db.QueryRow(`
		SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
		FROM order_items oi
		JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
		JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
		WHERE o.account_id=? AND o.marketplace_id=?
		  AND o.purchase_date>=? AND o.purchase_date<=?
		  AND o.order_status NOT IN ('Canceled','Declined')
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&summary.TotalCOGS)

	// 计算利润
	summary.GrossProfit = summary.TotalRevenue - summary.TotalRefunds - summary.TotalFees
	summary.NetProfit = summary.GrossProfit - summary.TotalAdSpend - summary.TotalCOGS
	if summary.TotalRevenue > 0 {
		summary.ProfitMargin = summary.NetProfit / summary.TotalRevenue * 100
	}

	return summary, nil
}

// SettlementSummary 结算报告摘要
type SettlementSummary struct {
	SettlementID   string  `json:"settlementId"`
	StartDate      string  `json:"startDate"`
	EndDate        string  `json:"endDate"`
	DepositDate    string  `json:"depositDate"`
	TotalAmount    float64 `json:"totalAmount"`
	Currency       string  `json:"currency"`
}

// GetSettlements 获取结算报告列表
func (s *FinanceService) GetSettlements(accountID int64, limit int) ([]SettlementSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT settlement_id, settlement_start_date, settlement_end_date,
			deposit_date, total_amount, currency_code
		FROM settlement_reports
		WHERE account_id=?
		ORDER BY deposit_date DESC LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SettlementSummary
	for rows.Next() {
		var s SettlementSummary
		if err := rows.Scan(&s.SettlementID, &s.StartDate, &s.EndDate, &s.DepositDate, &s.TotalAmount, &s.Currency); err != nil { slog.Warn("扫描结算报告行失败", "error", err); continue }
		results = append(results, s)
	}
	return results, nil
}

// ── 预警服务 ─────────────────────────────────────────────

// AlertsService 智能预警服务
type AlertsService struct {
	db *database.DB
}

func NewAlertsService(db *database.DB) *AlertsService {
	return &AlertsService{db: db}
}

// Alert 预警记录
type Alert struct {
	ID                  int64  `json:"id"`
	AlertType           string `json:"alertType"`
	Severity            string `json:"severity"`
	Title               string `json:"title"`
	Message             string `json:"message"`
	RelatedEntityType   string `json:"relatedEntityType"`
	RelatedEntityID     string `json:"relatedEntityId"`
	IsRead              bool   `json:"isRead"`
	IsDismissed         bool   `json:"isDismissed"`
	CreatedAt           string `json:"createdAt"`
}

// GetAlerts 获取未处理的预警列表
func (s *AlertsService) GetAlerts(accountID int64, unreadOnly bool) ([]Alert, error) {
	query := `
		SELECT id, alert_type, severity, title, message,
			COALESCE(related_entity_type,''), COALESCE(related_entity_id,''),
			is_read, is_dismissed, created_at
		FROM alerts
		WHERE account_id=? AND is_dismissed=0
	`
	if unreadOnly {
		query += " AND is_read=0"
	}
	query += " ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, created_at DESC LIMIT 100"

	rows, err := s.db.Query(query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var isRead, isDismissed int
		if err := rows.Scan(&a.ID, &a.AlertType, &a.Severity, &a.Title, &a.Message,
			&a.RelatedEntityType, &a.RelatedEntityID,
			&isRead, &isDismissed, &a.CreatedAt); err != nil {
			continue
		}
		a.IsRead = isRead == 1
		a.IsDismissed = isDismissed == 1
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// MarkAlertRead 标记预警为已读
func (s *AlertsService) MarkAlertRead(alertID int64) error {
	_, err := s.db.Exec("UPDATE alerts SET is_read=1 WHERE id=?", alertID)
	return err
}

// DismissAlert 忽略预警
func (s *AlertsService) DismissAlert(alertID int64) error {
	_, err := s.db.Exec("UPDATE alerts SET is_dismissed=1 WHERE id=?", alertID)
	return err
}

// CountUnreadAlerts 统计未读预警数量
func (s *AlertsService) CountUnreadAlerts(accountID int64) int {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM alerts WHERE account_id=? AND is_read=0 AND is_dismissed=0", accountID).Scan(&count)
	return count
}

// RunAlertChecks 执行预警规则检查（在每次数据同步完成后调用）
func (s *AlertsService) RunAlertChecks(accountID int64, marketplaceID string) {
	// 1. 低库存预警（可售天数 < 14 天）
	// ⚠️ 两阶段模式：先读数据到内存，关闭 rows 释放连接，再执行预警写入
	rows, err := s.db.Query(`
		SELECT i.seller_sku, i.asin, i.fulfillable_qty,
			COALESCE(
				(SELECT SUM(t.units_ordered)/30.0
				 FROM sales_traffic_by_asin t
				 WHERE t.asin=i.asin AND t.account_id=i.account_id AND t.marketplace_id=i.marketplace_id
				   AND t.date>=date('now','-30 days')
				), 0
			) as daily_sales
		FROM inventory_snapshots i
		WHERE i.account_id=? AND i.marketplace_id=?
		  AND i.snapshot_date=(SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id=? AND marketplace_id=?)
		  AND i.fulfillable_qty > 0
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err == nil {
		type invRow struct { sku, asin string; qty int; dailySales float64 }
		var invItems []invRow
		for rows.Next() {
			var r invRow
			if err := rows.Scan(&r.sku, &r.asin, &r.qty, &r.dailySales); err != nil { slog.Warn("扫描库存预警行失败", "error", err); continue }
			invItems = append(invItems, r)
		}
		rows.Close() // 释放连接

		for _, r := range invItems {
			if r.dailySales <= 0 {
				continue
			}
			daysLeft := float64(r.qty) / r.dailySales

			var severity, msg string
			if r.qty == 0 {
				severity = "critical"
				msg = fmt.Sprintf("SKU %s 已断货！当前可售库存为 0", r.sku)
			} else if daysLeft < 7 {
				severity = "critical"
				msg = fmt.Sprintf("SKU %s 库存严重不足，预计 %.0f 天后断货", r.sku, daysLeft)
			} else if daysLeft < 14 {
				severity = "warning"
				msg = fmt.Sprintf("SKU %s 库存偏低，预计 %.0f 天后断货", r.sku, daysLeft)
			} else {
				continue
			}

			var existing int
			s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='low_inventory'
				AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-24 hours')`,
				accountID, r.sku).Scan(&existing)
			if existing > 0 {
				continue
			}

			s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
				VALUES (?,?,?,?,?,?,?)`,
				accountID, "low_inventory", severity,
				"库存预警："+r.sku, msg, "product", r.sku,
			)
		}
	}

	// 2. 高 ACoS 预警（ACoS > 30%，基于原始 cost 和 attributed_sales_7d 计算）
	acoRows, err := s.db.Query(`
		SELECT campaign_id,
			CASE WHEN SUM(attributed_sales_7d) > 0
				THEN SUM(cost) / SUM(attributed_sales_7d) * 100
				ELSE 0
			END as calc_acos
		FROM ad_performance_daily
		WHERE account_id=? AND date>=date('now','-7 days') AND cost > 0
		GROUP BY campaign_id
		HAVING calc_acos > 30
	`, accountID)
	if err == nil {
		type acosRow struct { campaignID string; avgAcos float64 }
		var acosItems []acosRow
		for acoRows.Next() {
			var r acosRow
			if err := acoRows.Scan(&r.campaignID, &r.avgAcos); err != nil { slog.Warn("扫描 ACoS 预警行失败", "error", err); continue }
			acosItems = append(acosItems, r)
		}
		acoRows.Close() // 释放连接

		for _, r := range acosItems {
			var existing int
			s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='high_acos'
				AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-24 hours')`,
				accountID, r.campaignID).Scan(&existing)
			if existing > 0 {
				continue
			}

			s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
				VALUES (?,?,?,?,?,?,?)`,
				accountID, "high_acos", "warning",
				"广告 ACoS 偏高",
				fmt.Sprintf("活动 %s 近 7 天平均 ACoS 为 %.1f%%，建议优化竞价", r.campaignID, r.avgAcos),
				"campaign", r.campaignID,
			)
		}
	}

	slog.Info("预警规则检查完成", "account", accountID)

	// 3. 销售下滑预警（周销售环比下滑 >25%）
	s.checkSalesDropAlert(accountID, marketplaceID)

	// 4. Listing 不可售预警
	s.checkListingInactiveAlert(accountID, marketplaceID)
}

// checkSalesDropAlert 周销售环比下滑 >25% 预警
// 数据来源：sales_traffic_daily（Data Kiosk T+2）
func (s *AlertsService) checkSalesDropAlert(accountID int64, marketplaceID string) {
	var thisWeek, lastWeek float64
	// 本周（T-9 到 T-2，排除 T+2 延迟数据）
	s.db.QueryRow(`
		SELECT COALESCE(SUM(ordered_product_sales), 0)
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=?
		  AND date >= date('now', '-9 days') AND date <= date('now', '-2 days')
	`, accountID, marketplaceID).Scan(&thisWeek)
	// 上周（T-16 到 T-9）
	s.db.QueryRow(`
		SELECT COALESCE(SUM(ordered_product_sales), 0)
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=?
		  AND date >= date('now', '-16 days') AND date < date('now', '-9 days')
	`, accountID, marketplaceID).Scan(&lastWeek)

	if lastWeek <= 0 {
		return // 无上周数据，跳过
	}
	dropRate := (lastWeek - thisWeek) / lastWeek * 100
	if dropRate < 25 {
		return // 下滑不足 25%，跳过
	}

	// 7 天去重
	var existing int
	s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='sales_drop'
		AND is_dismissed=0 AND created_at>datetime('now','-7 days')`, accountID).Scan(&existing)
	if existing > 0 {
		return
	}

	severity := "warning"
	if dropRate > 50 {
		severity = "critical"
	}
	s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
		VALUES (?,?,?,?,?,?,?)`,
		accountID, "sales_drop", severity,
		"📉 销售下滑预警",
		fmt.Sprintf("本周全站销售额 %.0f，环比上周 %.0f 下滑 %.1f%%，请检查流量和竞价", thisWeek, lastWeek, dropRate),
		"marketplace", marketplaceID,
	)
}

// checkListingInactiveAlert 检测有历史销售但库存清零的 ASIN（疑似下架/不可售）
// 逻辑：库存最新快照中 fulfillable=0 且 inbound=0，但近 90 天有过销售的 ASIN
func (s *AlertsService) checkListingInactiveAlert(accountID int64, marketplaceID string) {
	rows, err := s.db.Query(`
		SELECT i.seller_sku, i.asin
		FROM inventory_snapshots i
		WHERE i.account_id=? AND i.marketplace_id=?
		  AND i.snapshot_date = (SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id=? AND marketplace_id=?)
		  AND i.fulfillable_qty = 0 AND COALESCE(i.inbound_qty, 0) = 0
		  AND EXISTS (
			SELECT 1 FROM sales_traffic_by_asin t
			WHERE t.asin = i.asin AND t.account_id = i.account_id
			  AND t.marketplace_id = i.marketplace_id
			  AND t.date >= date('now', '-90 days') AND t.units_ordered > 0
		  )
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		return
	}

	type listingRow struct { sku, asin string }
	var listingItems []listingRow
	for rows.Next() {
		var r listingRow
		if err := rows.Scan(&r.sku, &r.asin); err != nil {
			continue
		}
		listingItems = append(listingItems, r)
	}
	rows.Close() // 释放连接

	for _, r := range listingItems {
		// 7 天去重
		var existing int
		s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='listing_inactive'
			AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-7 days')`,
			accountID, r.sku).Scan(&existing)
		if existing > 0 {
			continue
		}

		s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
			VALUES (?,?,?,?,?,?,?)`,
			accountID, "listing_inactive", "warning",
			"🚫 Listing 疑似不可售",
			fmt.Sprintf("SKU %s (ASIN: %s) 当前 FBA 可售库存为 0 且无在途，但近 90 天有过销售记录，请检查 Listing 状态", r.sku, r.asin),
			"product", r.sku,
		)
	}
}

// CleanStaleData 清理过期数据（由调度器每月调用）
func (s *AlertsService) CleanStaleData() {
	// sync_log: 保留 90 天
	s.db.Exec(`DELETE FROM sync_log WHERE started_at < datetime('now', '-90 days')`)
	// alerts: 已忽略的保留 30 天
	s.db.Exec(`DELETE FROM alerts WHERE is_dismissed=1 AND created_at < datetime('now', '-30 days')`)
	// ad_search_terms: 保留 180 天
	s.db.Exec(`DELETE FROM ad_search_terms WHERE date < date('now', '-180 days')`)
	// fba_returns: 保留 365 天
	s.db.Exec(`DELETE FROM fba_returns WHERE return_date < date('now', '-365 days')`)
	slog.Info("过期数据清理完成")
}

// 统一 logSyncStart/End（复用 InventoryService 的方法签名避免重复）
func logSyncStart(db *database.DB, accountID int64, mktID, syncType string) int64 {
	r, err := db.Exec(`INSERT INTO sync_log(account_id,marketplace_id,sync_type,status,started_at)
		VALUES(?,?,?,'running',datetime('now'))`, accountID, mktID, syncType)
	if err != nil {
		return 0
	}
	id, _ := r.LastInsertId()
	return id
}

func logSyncEnd(db *database.DB, logID int64, status string, records int, errMsg string) {
	if logID == 0 {
		return
	}
	db.Exec(`UPDATE sync_log SET status=?,completed_at=datetime('now'),records_synced=?,error_message=? WHERE id=?`,
		status, records, errMsg, logID)
}

// GetSyncHistory 获取同步历史（使用只读连接）
func (s *AlertsService) GetSyncHistory(accountID int64, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.ReadQuery(`
		SELECT sync_type, status, started_at, completed_at, records_synced, COALESCE(error_message,'')
		FROM sync_log WHERE account_id=?
		ORDER BY started_at DESC LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var syncType, status, startedAt, completedAt, errMsg string
		var records int
		if err := rows.Scan(&syncType, &status, &startedAt, &completedAt, &records, &errMsg); err != nil { slog.Warn("扫描同步日志行失败", "error", err); continue }
		item := map[string]interface{}{
			"syncType":    syncType,
			"status":      status,
			"startedAt":   startedAt,
			"completedAt": completedAt,
			"records":     records,
			"error":       errMsg,
			"duration":    calcDuration(startedAt, completedAt),
		}
		result = append(result, item)
	}
	return result, nil
}

func calcDuration(start, end string) string {
	if start == "" || end == "" {
		return "-"
	}
	layout := "2006-01-02 15:04:05"
	s, err1 := time.Parse(layout, start)
	e, err2 := time.Parse(layout, end)
	if err1 != nil || err2 != nil {
		return "-"
	}
	d := e.Sub(s)
	if d < time.Minute {
		return fmt.Sprintf("%.0f 秒", d.Seconds())
	}
	return fmt.Sprintf("%.1f 分钟", d.Minutes())
}
