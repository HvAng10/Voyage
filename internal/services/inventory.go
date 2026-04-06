// Package services - 库存同步服务
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

// InventoryService 库存同步服务（FBA 库存报告）
type InventoryService struct {
	db     *database.DB
	client *spapi.Client
}

func NewInventoryService(db *database.DB, client *spapi.Client) *InventoryService {
	return &InventoryService{db: db, client: client}
}

// SyncFBAInventory 同步 FBA 库存快照
// 注：FBA 库存报告两次请求间隔至少 30 分钟（Amazon API 限制）
func (s *InventoryService) SyncFBAInventory(ctx context.Context, accountID int64, marketplaceID string) (int, error) {
	slog.Info("开始同步 FBA 库存", "account", accountID, "marketplace", marketplaceID)

	// 检查是否距上次同步已过 30 分钟
	var lastSync string
	s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(started_at), '')
		FROM sync_log WHERE account_id = ? AND sync_type = 'inventory' AND status = 'success'
	`, accountID).Scan(&lastSync)

	if lastSync != "" {
		t, err := time.Parse("2006-01-02 15:04:05", lastSync)
		if err == nil && time.Since(t) < 31*time.Minute {
			return 0, fmt.Errorf("FBA 库存报告两次请求间隔至少 30 分钟（上次：%s）", lastSync)
		}
	}

	// 记录同步开始
	logID := s.logSyncStart(accountID, marketplaceID, "inventory")

	// 创建报告
	createResp, err := s.client.CreateReport(ctx, spapi.CreateReportRequest{
		ReportType:     spapi.ReportTypeFBAInventory,
		MarketplaceIds: []string{marketplaceID},
	})
	if err != nil {
		s.logSyncEnd(logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("创建 FBA 库存报告失败: %w", err)
	}

	slog.Info("FBA 库存报告已创建", "reportId", createResp.ReportId)

	// 等待报告完成（最多 30 分钟轮询）
	docID, err := s.client.WaitForReport(ctx, createResp.ReportId, 2*time.Minute, 30*time.Minute)
	if err != nil {
		s.logSyncEnd(logID, "failed", 0, err.Error())
		return 0, err
	}

	// 获取下载 URL
	docResp, err := s.client.GetReportDocument(ctx, docID)
	if err != nil {
		s.logSyncEnd(logID, "failed", 0, err.Error())
		return 0, err
	}

	// 下载并解析 TSV 报告
	count, err := s.downloadAndParseInventoryReport(ctx, accountID, marketplaceID, docResp.Url)
	if err != nil {
		s.logSyncEnd(logID, "failed", count, err.Error())
		return count, err
	}

	s.logSyncEnd(logID, "success", count, "")
	slog.Info("FBA 库存同步完成", "account", accountID, "records", count)
	return count, nil
}

// downloadAndParseInventoryReport 下载并解析 FBA 库存 TSV 报告
func (s *InventoryService) downloadAndParseInventoryReport(ctx context.Context, accountID int64, marketplaceID, downloadURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载库存报告失败: %w", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.Comma = '\t' // FBA 库存报告使用 Tab 分隔

	// 读取并解析表头行
	headers, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("读取报告表头失败: %w", err)
	}

	// 建立列名到索引的映射（FBA 库存报告列名参考官方文档）
	// 来源：https://developer-docs.amazon.com/sp-api/docs/report-type-values-fba#fba-inventory-reports
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

	snapshotDate := time.Now().UTC().Format("2006-01-02")
	count := 0

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 先删除今天的旧快照（避免重复）
	tx.Exec(
		"DELETE FROM inventory_snapshots WHERE account_id = ? AND marketplace_id = ? AND snapshot_date = ?",
		accountID, marketplaceID, snapshotDate,
	)

	stmt, err := tx.Prepare(`
		INSERT INTO inventory_snapshots (
			account_id, marketplace_id, seller_sku, asin, fnsku,
			condition_type, fulfillable_qty, unsellable_qty,
			reserved_qty, inbound_qty, researching_qty,
			unfulfillable_qty, snapshot_date
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("解析库存报告行失败", "error", err)
			continue
		}

		sku := get(row, "seller-sku")
		if sku == "" {
			continue
		}

		if _, err := stmt.Exec(
			accountID, marketplaceID,
			sku,
			get(row, "asin"),
			get(row, "fnsku"),
			get(row, "condition-type"),
			getInt(row, "your-fulfillable-quantity"),
			getInt(row, "unsellable-quantity"),
			getInt(row, "reserved-quantity"),
			getInt(row, "inbound-quantity"),
			getInt(row, "researching-quantity"),
			getInt(row, "unfulfillable-quantity"),
			snapshotDate,
		); err != nil {
			slog.Warn("写入库存快照失败", "sku", sku, "error", err)
			continue
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

// ── 库存查询 ────────────────────────────────────────────

// InventoryItem 库存明细（用于前端表格展示）
type InventoryItem struct {
	SellerSKU       string  `json:"sellerSku"`
	ASIN            string  `json:"asin"`
	Title           string  `json:"title"`
	FulfillableQty  int     `json:"fulfillableQty"`
	InboundQty      int     `json:"inboundQty"`
	UnsellableQty   int     `json:"unsellableQty"`
	ReservedQty     int     `json:"reservedQty"`
	TotalQty        int     `json:"totalQty"`
	DailySalesAvg   float64 `json:"dailySalesAvg"`   // 近 30 天日均销量
	EstDaysLeft     float64 `json:"estDaysLeft"`     // 预计库存天数
	AlertLevel      string  `json:"alertLevel"`      // ok / warning / critical
	SnapshotDate    string  `json:"snapshotDate"`
}

// GetInventoryItems 查询库存明细（独立函数，避免在非本地类型上定义方法）
func GetInventoryItems(db *database.DB, accountID int64, marketplaceID string) ([]InventoryItem, error) {
	rows, err := db.Query(`
		SELECT
			i.seller_sku,
			i.asin,
			COALESCE(p.title, i.asin) as title,
			i.fulfillable_qty,
			i.inbound_qty,
			i.unsellable_qty,
			i.reserved_qty,
			(i.fulfillable_qty + i.inbound_qty) as total_qty,
			i.snapshot_date,
			-- 近 30 天日均销量（来自 sales_traffic_by_asin）
			COALESCE(
				(SELECT SUM(t.units_ordered) / 30.0
				 FROM sales_traffic_by_asin t
				 WHERE t.asin = i.asin
				   AND t.account_id = i.account_id
				   AND t.marketplace_id = i.marketplace_id
				   AND t.date >= date('now', '-30 days')
				), 0
			) as daily_sales_avg
		FROM inventory_snapshots i
		LEFT JOIN products p ON i.asin = p.asin
			AND p.account_id = i.account_id AND p.marketplace_id = i.marketplace_id
		WHERE i.account_id = ? AND i.marketplace_id = ?
		  AND i.snapshot_date = (
			  SELECT MAX(snapshot_date) FROM inventory_snapshots
			  WHERE account_id = ? AND marketplace_id = ?
		  )
		ORDER BY fulfillable_qty ASC
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(
			&item.SellerSKU, &item.ASIN, &item.Title,
			&item.FulfillableQty, &item.InboundQty, &item.UnsellableQty,
			&item.ReservedQty, &item.TotalQty,
			&item.SnapshotDate, &item.DailySalesAvg,
		); err != nil {
			return nil, err
		}

		// 计算预计库存天数
		if item.DailySalesAvg > 0 {
			item.EstDaysLeft = float64(item.FulfillableQty) / item.DailySalesAvg
		} else {
			item.EstDaysLeft = 999
		}

		// 预警等级
		switch {
		case item.FulfillableQty == 0:
			item.AlertLevel = "critical"
		case item.EstDaysLeft < 14:
			item.AlertLevel = "warning"
		default:
			item.AlertLevel = "ok"
		}

		items = append(items, item)
	}
	return items, nil
}

// logSyncStart 记录同步开始
func (s *InventoryService) logSyncStart(accountID int64, marketplaceID, syncType string) int64 {
	r, err := s.db.Exec(`
		INSERT INTO sync_log(account_id, marketplace_id, sync_type, status, started_at)
		VALUES(?, ?, ?, 'running', datetime('now'))
	`, accountID, marketplaceID, syncType)
	if err != nil {
		return 0
	}
	id, _ := r.LastInsertId()
	return id
}

// logSyncEnd 更新同步结束状态
func (s *InventoryService) logSyncEnd(logID int64, status string, records int, errMsg string) {
	if logID == 0 {
		return
	}
	s.db.Exec(`
		UPDATE sync_log SET status=?, completed_at=datetime('now'), records_synced=?, error_message=?
		WHERE id=?
	`, status, records, errMsg, logID)
}
