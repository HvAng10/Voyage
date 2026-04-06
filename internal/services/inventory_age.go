// Package services - FBA 库龄数据同步与长期仓储费预警
// 数据延迟：每月 15 日更新，T+1；长期仓储费于每年 2 月和 8 月征收
package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
)

// FBA 库龄报告类型
const ReportTypeFBAInventoryAge = "GET_FBA_INVENTORY_AGED_DATA"

// InventoryAgeService 库龄数据同步服务
type InventoryAgeService struct {
	db     *database.DB
	client *spapi.Client
}

func NewInventoryAgeService(db *database.DB, client *spapi.Client) *InventoryAgeService {
	return &InventoryAgeService{db: db, client: client}
}

// SyncInventoryAge 同步 FBA 库龄数据
// 注意：此报告每月更新一次（约 15 日刷新），建议每月触发一次同步
// 数据延迟 T+1
func (s *InventoryAgeService) SyncInventoryAge(ctx context.Context, accountID int64, marketplaceID string) (int, error) {
	slog.Info("开始同步 FBA 库龄数据", "account", accountID, "marketplace", marketplaceID)

	logID := logSyncStart(s.db, accountID, marketplaceID, "fba_inventory_age")

	// 库龄报告为当前时间点快照，不传时间范围
	createResp, err := s.client.CreateReport(ctx, spapi.CreateReportRequest{
		ReportType:     ReportTypeFBAInventoryAge,
		MarketplaceIds: []string{marketplaceID},
	})
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("创建库龄报告失败: %w", err)
	}

	docID, err := s.client.WaitForReport(ctx, createResp.ReportId, 3*time.Minute, 40*time.Minute)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	docResp, err := s.client.GetReportDocument(ctx, docID)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	snapshotDate := time.Now().UTC().Format("2006-01-02")
	count, err := s.parseInventoryAgeReport(ctx, accountID, marketplaceID, snapshotDate, docResp.Url)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}

	logSyncEnd(s.db, logID, "success", count, "")
	slog.Info("FBA 库龄同步完成", "account", accountID, "records", count)

	// 检查长期仓储费预警
	s.checkLTSFAlerts(accountID, marketplaceID, snapshotDate)

	return count, nil
}

// parseInventoryAgeReport 解析 FBA 库龄 TSV 报告
// 列参考：https://developer-docs.amazon.com/sp-api/docs/report-type-values-fba#fba-inventory-reports
func (s *InventoryAgeService) parseInventoryAgeReport(ctx context.Context, accountID int64, marketplaceID, snapshotDate, url string) (int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载库龄报告失败: %w", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.Comma = '\t'
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("读取库龄报告表头失败: %w", err)
	}

	colMap := make(map[string]int)
	for i, h := range headers {
		colMap[strings.TrimSpace(h)] = i
	}

	get := func(row []string, key string) string {
		i, ok := colMap[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	getInt := func(row []string, key string) int {
		v, _ := strconv.Atoi(get(row, key))
		return v
	}
	getFloat := func(row []string, key string) float64 {
		v, _ := strconv.ParseFloat(get(row, key), 64)
		return v
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO fba_inventory_age (
			account_id, marketplace_id, snapshot_date,
			sku, asin, product_name, condition,
			qty_0_90_days, qty_91_180_days, qty_181_270_days, qty_271_365_days, qty_over_365_days,
			est_ltsf, currency, fnsku
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id, marketplace_id, snapshot_date, sku) DO UPDATE SET
			qty_0_90_days = excluded.qty_0_90_days,
			qty_91_180_days = excluded.qty_91_180_days,
			qty_181_270_days = excluded.qty_181_270_days,
			qty_271_365_days = excluded.qty_271_365_days,
			qty_over_365_days = excluded.qty_over_365_days,
			est_ltsf = excluded.est_ltsf
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("解析库龄报告行失败", "error", err)
			continue
		}

		sku := get(row, "sku")
		if sku == "" {
			continue
		}

		_, err = stmt.ExecContext(ctx,
			accountID, marketplaceID, snapshotDate,
			sku,
			get(row, "asin"),
			get(row, "product-name"),
			get(row, "condition"),
			getInt(row, "inv-age-0-to-90-days"),
			getInt(row, "inv-age-91-to-180-days"),
			getInt(row, "inv-age-181-to-270-days"),
			getInt(row, "inv-age-271-to-365-days"),
			getInt(row, "inv-age-365-plus-days"),
			getFloat(row, "estimated-storage-cost-next-month"),
			"USD",
			get(row, "fnsku"),
		)
		if err == nil {
			count++
		}
	}

	return count, tx.Commit()
}

// checkLTSFAlerts 检查长期仓储费风险预警
// 365 天以上库存会被征收高额长期仓储费（≥$6.90/立方英尺）
func (s *InventoryAgeService) checkLTSFAlerts(accountID int64, marketplaceID, snapshotDate string) {
	rows, err := s.db.Query(`
		SELECT sku, asin, qty_over_365_days, qty_181_270_days + qty_271_365_days as at_risk, est_ltsf
		FROM fba_inventory_age
		WHERE account_id = ? AND marketplace_id = ? AND snapshot_date = ?
		  AND (qty_over_365_days > 0 OR qty_181_270_days + qty_271_365_days > 30)
		ORDER BY est_ltsf DESC
	`, accountID, marketplaceID, snapshotDate)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var sku, asin string
		var over365, atRisk int
		var estLTSF float64
		if err := rows.Scan(&sku, &asin, &over365, &atRisk, &estLTSF); err != nil { slog.Warn("扫描库龄预警行失败", "error", err); continue }

		var severity, title, msg string
		if over365 > 0 {
			severity = "critical"
			title = fmt.Sprintf("长期仓储费警告：%s", sku)
			msg = fmt.Sprintf("SKU %s 有 %d 件库存超过 365 天，预计本月长期仓储费 $%.2f，建议立即促销或移仓", sku, over365, estLTSF)
		} else {
			severity = "warning"
			title = fmt.Sprintf("库存滞龄预警：%s", sku)
			msg = fmt.Sprintf("SKU %s 有 %d 件库存超过 180 天，若无法在 90-180 天内清货，将产生长期仓储费", sku, atRisk)
		}

		// 每 7 天触发一次（库龄变化缓慢）
		var existing int
		s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='ltsf_risk'
			AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-7 days')`,
			accountID, sku).Scan(&existing)
		if existing > 0 {
			continue
		}

		s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
			VALUES (?,?,?,?,?,?,?)`,
			accountID, "ltsf_risk", severity, title, msg, "product", sku,
		)
	}
}

// ── 库龄数据查询 ─────────────────────────────────────────

// InventoryAgeItem 库龄分析数据项
type InventoryAgeItem struct {
	SKU            string  `json:"sku"`
	ASIN           string  `json:"asin"`
	ProductName    string  `json:"productName"`
	Qty0to90       int     `json:"qty0to90"`
	Qty91to180     int     `json:"qty91to180"`
	Qty181to270    int     `json:"qty181to270"`
	Qty271to365    int     `json:"qty271to365"`
	QtyOver365     int     `json:"qtyOver365"`
	TotalQty       int     `json:"totalQty"`
	EstLTSF        float64 `json:"estLtsf"`
	// 计算字段
	AgingRatio     float64 `json:"agingRatio"` // >180 天占比
	RiskLevel      string  `json:"riskLevel"`  // ok / warning / critical
	// 数据延迟标注
	DataLatencyNote string  `json:"dataLatencyNote"`
}

// InventoryAgeSummary 库龄汇总
type InventoryAgeSummary struct {
	SnapshotDate    string  `json:"snapshotDate"`
	TotalSKUs       int     `json:"totalSkus"`
	TotalQty        int     `json:"totalQty"`
	QtyAtRisk       int     `json:"qtyAtRisk"`       // >180 天
	QtyOver365      int     `json:"qtyOver365"`      // >365 天
	EstTotalLTSF    float64 `json:"estTotalLtsf"`
	DataLatencyNote string  `json:"dataLatencyNote"`
}

// GetInventoryAge 查询最新一批库龄数据
func GetInventoryAge(db *database.DB, accountID int64, marketplaceID string) ([]InventoryAgeItem, *InventoryAgeSummary, error) {
	// 获取最新快照日期
	var snapshotDate string
	db.QueryRow(`SELECT MAX(snapshot_date) FROM fba_inventory_age WHERE account_id=? AND marketplace_id=?`,
		accountID, marketplaceID).Scan(&snapshotDate)

	if snapshotDate == "" {
		return nil, nil, nil
	}

	rows, err := db.Query(`
		SELECT sku, asin, product_name,
			qty_0_90_days, qty_91_180_days, qty_181_270_days, qty_271_365_days, qty_over_365_days,
			est_ltsf
		FROM fba_inventory_age
		WHERE account_id=? AND marketplace_id=? AND snapshot_date=?
		ORDER BY est_ltsf DESC, qty_over_365_days DESC
	`, accountID, marketplaceID, snapshotDate)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	latencyNote := "库龄数据约每月 15 日更新（T+1），请以最新快照日期为准"

	var items []InventoryAgeItem
	summary := &InventoryAgeSummary{
		SnapshotDate:    snapshotDate,
		DataLatencyNote: latencyNote,
	}

	for rows.Next() {
		var item InventoryAgeItem
		if err := rows.Scan(&item.SKU, &item.ASIN, &item.ProductName,
			&item.Qty0to90, &item.Qty91to180, &item.Qty181to270, &item.Qty271to365, &item.QtyOver365,
			&item.EstLTSF); err != nil {
			slog.Warn("扫描库龄数据行失败", "error", err)
			continue
		}

		item.TotalQty = item.Qty0to90 + item.Qty91to180 + item.Qty181to270 + item.Qty271to365 + item.QtyOver365
		item.DataLatencyNote = latencyNote

		// 计算风险系数
		atRiskQty := item.Qty181to270 + item.Qty271to365 + item.QtyOver365
		if item.TotalQty > 0 {
			item.AgingRatio = float64(atRiskQty) / float64(item.TotalQty) * 100
		}

		switch {
		case item.QtyOver365 > 0:
			item.RiskLevel = "critical"
		case item.AgingRatio > 40:
			item.RiskLevel = "warning"
		default:
			item.RiskLevel = "ok"
		}

		// 汇总
		summary.TotalSKUs++
		summary.TotalQty += item.TotalQty
		summary.QtyAtRisk += atRiskQty
		summary.QtyOver365 += item.QtyOver365
		summary.EstTotalLTSF += item.EstLTSF

		items = append(items, item)
	}

	return items, summary, nil
}
