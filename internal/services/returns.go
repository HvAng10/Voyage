// Package services - FBA 退货报告同步与分析
// 数据延迟：T+1（FBA 退货数据通常在退货发生后第二天可用）
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

// FBA 退货报告类型
const ReportTypeFBAReturns = "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA"

// ReturnsService FBA 退货数据同步服务
type ReturnsService struct {
	db     *database.DB
	client *spapi.Client
}

func NewReturnsService(db *database.DB, client *spapi.Client) *ReturnsService {
	return &ReturnsService{db: db, client: client}
}

// SyncFBAReturns 同步 FBA 客户退货数据
// 注意：数据延迟约 T+1，始终以 endDate = 昨天 为上限
func (s *ReturnsService) SyncFBAReturns(ctx context.Context, accountID int64, marketplaceID string, days int) (int, error) {
	slog.Info("开始同步 FBA 退货数据", "account", accountID, "marketplace", marketplaceID)

	logID := logSyncStart(s.db, accountID, marketplaceID, "fba_returns")

	// T+1 延迟：结束日期为昨天
	endDate := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + "T00:00:00Z"
	startDate := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02") + "T00:00:00Z"

	createResp, err := s.client.CreateReport(ctx, spapi.CreateReportRequest{
		ReportType:     ReportTypeFBAReturns,
		MarketplaceIds: []string{marketplaceID},
		DataStartTime:  &startDate,
		DataEndTime:    &endDate,
	})
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("创建 FBA 退货报告失败: %w", err)
	}

	docID, err := s.client.WaitForReport(ctx, createResp.ReportId, 2*time.Minute, 30*time.Minute)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	docResp, err := s.client.GetReportDocument(ctx, docID)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	count, err := s.parseReturnsReport(ctx, accountID, marketplaceID, docResp.Url)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}

	logSyncEnd(s.db, logID, "success", count, "")
	slog.Info("FBA 退货同步完成", "account", accountID, "records", count)

	// 退货率超警戒线的预警检查
	s.checkReturnRateAlerts(accountID, marketplaceID)

	return count, nil
}

// parseReturnsReport 解析 FBA 退货 TSV 报告
// 列参考：https://developer-docs.amazon.com/sp-api/docs/report-type-values-fba#fba-sales-reports
func (s *ReturnsService) parseReturnsReport(ctx context.Context, accountID int64, marketplaceID, url string) (int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载退货报告失败: %w", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.Comma = '\t'
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("读取退货报告表头失败: %w", err)
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

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO fba_returns (
			account_id, marketplace_id, return_date, order_id,
			sku, asin, fnsku, product_name, quantity,
			fulfillment_center, detailed_disposition, reason, status,
			license_plate_number, customer_comments
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT DO NOTHING
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
			slog.Warn("解析退货报告行失败", "error", err)
			continue
		}

		sku := get(row, "sku")
		if sku == "" {
			continue
		}

		_, err = stmt.ExecContext(ctx,
			accountID, marketplaceID,
			get(row, "return-date"),
			get(row, "order-id"),
			sku,
			get(row, "asin"),
			get(row, "fnsku"),
			get(row, "product-name"),
			getInt(row, "quantity"),
			get(row, "fulfillment-center-id"),
			get(row, "detailed-disposition"),
			get(row, "reason"),
			get(row, "status"),
			get(row, "license-plate-number"),
			get(row, "customer-comments"),
		)
		if err == nil {
			count++
		}
	}

	return count, tx.Commit()
}

// checkReturnRateAlerts 检查退货率超警戒线（近 30 天 ASIN 退货率 > 10%）
func (s *ReturnsService) checkReturnRateAlerts(accountID int64, marketplaceID string) {
	rows, err := s.db.Query(`
		SELECT
			r.sku, r.asin,
			SUM(r.quantity) as returned,
			COALESCE(
				(SELECT SUM(t.units_ordered)
				 FROM sales_traffic_by_asin t
				 WHERE t.asin = r.asin AND t.account_id = r.account_id
				   AND t.date >= date('now','-30 days')
				), 0
			) as sold
		FROM fba_returns r
		WHERE r.account_id = ? AND r.marketplace_id = ?
		  AND r.return_date >= date('now','-30 days')
		GROUP BY r.sku, r.asin
		HAVING sold > 0
	`, accountID, marketplaceID)
	if err != nil {
		return
	}

	// ⚠️ 两阶段模式：先读后写，避免 MaxOpenConns=1 死锁
	type returnRow struct { sku, asin string; returned, sold int }
	var returnItems []returnRow
	for rows.Next() {
		var r returnRow
		if err := rows.Scan(&r.sku, &r.asin, &r.returned, &r.sold); err != nil { slog.Warn("扫描退货率行失败", "error", err); continue }
		returnItems = append(returnItems, r)
	}
	rows.Close() // 释放连接

	for _, r := range returnItems {
		returnRate := float64(r.returned) / float64(r.sold) * 100

		var severity string
		if returnRate > 20 {
			severity = "critical"
		} else if returnRate > 10 {
			severity = "warning"
		} else {
			continue
		}

		// 去重：24 小时内不重复
		var existing int
		s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='high_return_rate'
			AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-24 hours')`,
			accountID, r.sku).Scan(&existing)
		if existing > 0 {
			continue
		}

		s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
			VALUES (?,?,?,?,?,?,?)`,
			accountID, "high_return_rate", severity,
			fmt.Sprintf("退货率预警：%s", r.sku),
			fmt.Sprintf("SKU %s 近 30 天退货率为 %.1f%%（退货 %d / 销售 %d 件），请检查产品质量或描述准确性", r.sku, returnRate, r.returned, r.sold),
			"product", r.sku,
		)
	}
}

// ── 退货数据查询 ─────────────────────────────────────────

// ReturnRateByASIN ASIN 退货率汇总
type ReturnRateByASIN struct {
	ASIN        string  `json:"asin"`
	SKU         string  `json:"sku"`
	Title       string  `json:"title"`
	TotalReturns int    `json:"totalReturns"`
	TotalSold   int     `json:"totalSold"`
	ReturnRate  float64 `json:"returnRate"`
	TopReason   string  `json:"topReason"`
	// 数据延迟标注
	DataLatencyNote string `json:"dataLatencyNote"`
}

// GetReturnRateByASIN 查询 ASIN 退货率统计（近 N 天）
func GetReturnRateByASIN(db *database.DB, accountID int64, marketplaceID string, days int) ([]ReturnRateByASIN, error) {
	rows, err := db.Query(`
		SELECT
			r.asin,
			r.sku,
			COALESCE(p.title, r.asin) as title,
			SUM(r.quantity) as total_returns,
			COALESCE(
				(SELECT SUM(t.units_ordered)
				 FROM sales_traffic_by_asin t
				 WHERE t.asin = r.asin AND t.account_id = r.account_id
				   AND t.date >= date('now', '-' || ? || ' days')
				), 0
			) as total_sold,
			(SELECT r2.reason FROM fba_returns r2
			 WHERE r2.account_id = r.account_id AND r2.asin = r.asin
			   AND r2.return_date >= date('now', '-' || ? || ' days')
			 GROUP BY r2.reason ORDER BY COUNT(*) DESC LIMIT 1
			) as top_reason
		FROM fba_returns r
		LEFT JOIN products p ON r.asin = p.asin
			AND p.account_id = r.account_id AND p.marketplace_id = r.marketplace_id
		WHERE r.account_id = ? AND r.marketplace_id = ?
		  AND r.return_date >= date('now', '-' || ? || ' days')
		GROUP BY r.asin, r.sku
		ORDER BY SUM(r.quantity) DESC
		LIMIT 50
	`, days, days, accountID, marketplaceID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ReturnRateByASIN
	for rows.Next() {
		var item ReturnRateByASIN
		var reason *string
		if err := rows.Scan(&item.ASIN, &item.SKU, &item.Title, &item.TotalReturns, &item.TotalSold, &reason); err != nil { slog.Warn("扫描 ASIN 退货行失败", "error", err); continue }
		if reason != nil {
			item.TopReason = *reason
		}
		if item.TotalSold > 0 {
			item.ReturnRate = float64(item.TotalReturns) / float64(item.TotalSold) * 100
		}
		item.DataLatencyNote = "数据延迟约 T+1（FBA 退货数据通常在退货发生后次日可查）"
		result = append(result, item)
	}
	return result, nil
}

// ReturnDetail 退货明细记录
type ReturnDetail struct {
	ReturnDate         string `json:"returnDate"`
	SKU                string `json:"sku"`
	ASIN               string `json:"asin"`
	Quantity           int    `json:"quantity"`
	Reason             string `json:"reason"`
	DetailedDis        string `json:"detailedDisposition"`
	Status             string `json:"status"`
	FulfillmentCenter  string `json:"fulfillmentCenter"`
}

// GetReturnDetails 查询退货明细
func GetReturnDetails(db *database.DB, accountID int64, marketplaceID string, days int) ([]ReturnDetail, error) {
	rows, err := db.Query(`
		SELECT return_date, sku, asin, quantity, reason, detailed_disposition, status, fulfillment_center
		FROM fba_returns
		WHERE account_id = ? AND marketplace_id = ?
		  AND return_date >= date('now', '-' || ? || ' days')
		ORDER BY return_date DESC
		LIMIT 200
	`, accountID, marketplaceID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ReturnDetail
	for rows.Next() {
		var d ReturnDetail
		if err := rows.Scan(&d.ReturnDate, &d.SKU, &d.ASIN, &d.Quantity, &d.Reason, &d.DetailedDis, &d.Status, &d.FulfillmentCenter); err != nil { slog.Warn("扫描退货明细行失败", "error", err); continue }
		result = append(result, d)
	}
	return result, nil
}

// ReturnReasonStat 退货原因分布统计
type ReturnReasonStat struct {
	Reason     string  `json:"reason"`
	ReasonDesc string  `json:"reasonDesc"` // 中文翻译
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// reasonDescMap 亚马逊标准退货原因中文翻译
var reasonDescMap = map[string]string{
	"DEFECTIVE":                "商品缺陷",
	"NOT_AS_DESCRIBED":         "与描述不符",
	"WRONG_ITEM":               "发错商品",
	"UNWANTED_ITEM":            "不想要了",
	"NO_REASON":                "未提供原因",
	"SWITCHEROO":               "调换退货（欺诈）",
	"QUALITY_UNACCEPTABLE":     "质量不可接受",
	"UNAUTHORIZED_PURCHASE":    "未授权购买",
	"FOUND_BETTER_PRICE":       "找到更低价格",
	"MISSED_ESTIMATED_DELIVERY":"超出预计送达时间",
	"DAMAGED_BY_CARRIER":       "物流损坏",
	"CUSTOMER_CHANGED_MIND":    "客户改变主意",
	"APPAREL_TOO_SMALL":        "尺码偏小",
	"APPAREL_TOO_LARGE":        "尺码偏大",
	"APPAREL_STYLE":            "款式不满意",
}

// GetReturnReasonDistribution 获取退货原因分布统计
func GetReturnReasonDistribution(db *database.DB, accountID int64, marketplaceID string, days int) ([]ReturnReasonStat, error) {
	if days <= 0 { days = 30 }
	rows, err := db.Query(`
		SELECT COALESCE(reason, 'UNKNOWN') as r, SUM(quantity) as cnt
		FROM fba_returns
		WHERE account_id=? AND marketplace_id=? AND return_date >= date('now', '-' || ? || ' days')
		GROUP BY r
		ORDER BY cnt DESC
	`, accountID, marketplaceID, days)
	if err != nil { return nil, err }
	defer rows.Close()

	var items []ReturnReasonStat
	var total int
	for rows.Next() {
		var s ReturnReasonStat
		if err := rows.Scan(&s.Reason, &s.Count); err != nil { slog.Warn("扫描退货原因行失败", "error", err); continue }
		total += s.Count
		items = append(items, s)
	}

	// 计算百分比并翻译
	for i := range items {
		if total > 0 {
			items[i].Percentage = float64(items[i].Count) / float64(total) * 100
		}
		if desc, ok := reasonDescMap[items[i].Reason]; ok {
			items[i].ReasonDesc = desc
		} else {
			items[i].ReasonDesc = items[i].Reason
		}
	}
	return items, nil
}
