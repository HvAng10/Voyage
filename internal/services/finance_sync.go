// Package services - 财务事件同步 + CSV 成本批量导入
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
)

// FinanceSyncService 财务事件同步服务
type FinanceSyncService struct {
	db     *database.DB
	client *spapi.Client
}

func NewFinanceSyncService(db *database.DB, client *spapi.Client) *FinanceSyncService {
	return &FinanceSyncService{db: db, client: client}
}

// spAmount 货币金额类型
type spAmount struct {
	CurrencyCode   string  `json:"CurrencyCode"`
	CurrencyAmount float64 `json:"CurrencyAmount"`
}

// financialEventsResp SP-API 财务事件响应
type financialEventsResp struct {
	Payload struct {
		NextToken       string `json:"NextToken"`
		FinancialEvents struct {
			ShipmentEventList []struct {
				AmazonOrderId   string `json:"AmazonOrderId"`
				PostedDate      string `json:"PostedDate"`
				MarketplaceName string `json:"MarketplaceName"`
				ShipmentItemList []struct {
					ItemChargeList []struct {
						ChargeType   string   `json:"ChargeType"`
						ChargeAmount spAmount `json:"ChargeAmount"`
					} `json:"ItemChargeList"`
					ItemFeeList []struct {
						FeeType   string   `json:"FeeType"`
						FeeAmount spAmount `json:"FeeAmount"`
					} `json:"ItemFeeList"`
					PromotionList []struct {
						PromotionType   string   `json:"PromotionType"`
						PromotionAmount spAmount `json:"PromotionAmount"`
					} `json:"PromotionList"`
				} `json:"ShipmentItemList"`
			} `json:"ShipmentEventList"`
			RefundEventList []struct {
				AmazonOrderId string `json:"AmazonOrderId"`
				PostedDate    string `json:"PostedDate"`
				ShipmentItemAdjustmentList []struct {
					ItemChargeAdjustmentList []struct {
						ChargeType       string   `json:"ChargeType"`
						ChargeAmount     spAmount `json:"ChargeAmount"`
					} `json:"ItemChargeAdjustmentList"`
				} `json:"ShipmentItemAdjustmentList"`
			} `json:"RefundEventList"`
			ServiceFeeEventList []struct {
				AmazonOrderId string `json:"AmazonOrderId"`
				PostedDate    string `json:"PostedDate"`
				FeeList []struct {
					FeeType   string   `json:"FeeType"`
					FeeAmount spAmount `json:"FeeAmount"`
				} `json:"FeeList"`
			} `json:"ServiceFeeEventList"`
		} `json:"FinancialEvents"`
	} `json:"payload"`
}

// SyncFinancialEvents 同步财务事件（Finances API v2024-06-19）
func (s *FinanceSyncService) SyncFinancialEvents(ctx context.Context, accountID int64, marketplaceID, dateStart, dateEnd string) (int, error) {
	logID := logSyncStart(s.db, accountID, marketplaceID, "financial_events")
	records := 0

	nextToken := ""
	for {
		params := url.Values{}
		params.Set("PostedAfter", dateStart+"T00:00:00Z")
		params.Set("PostedBefore", dateEnd+"T23:59:59Z")
		if nextToken != "" {
			params.Set("NextToken", nextToken)
		}

		var resp financialEventsResp
		if err := s.client.Get(ctx, "/finances/2024-06-19/financial-events", params, &resp); err != nil {
			logSyncEnd(s.db, logID, "failed", records, err.Error())
			return records, fmt.Errorf("获取财务事件失败: %w", err)
		}

		// 处理发货事件（销售收入）
		for _, e := range resp.Payload.FinancialEvents.ShipmentEventList {
			var principal, tax, fee float64
			currency := "USD"
			for _, item := range e.ShipmentItemList {
				for _, charge := range item.ItemChargeList {
					currency = charge.ChargeAmount.CurrencyCode
					switch charge.ChargeType {
					case "Principal":
						principal += charge.ChargeAmount.CurrencyAmount
					case "Tax":
						tax += charge.ChargeAmount.CurrencyAmount
					}
				}
				for _, f := range item.ItemFeeList {
					fee += f.FeeAmount.CurrencyAmount
					if currency == "USD" && f.FeeAmount.CurrencyCode != "" {
						currency = f.FeeAmount.CurrencyCode
					}
				}
			}
			n, _ := s.upsertFinancialEvent(accountID, marketplaceID, e.AmazonOrderId, "Order", e.PostedDate, principal, tax, fee, currency)
			records += n
		}

		// 处理退款事件
		for _, e := range resp.Payload.FinancialEvents.RefundEventList {
			var refundAmt float64
			currency := "USD"
			for _, item := range e.ShipmentItemAdjustmentList {
				for _, charge := range item.ItemChargeAdjustmentList {
					refundAmt += charge.ChargeAmount.CurrencyAmount
					if charge.ChargeAmount.CurrencyCode != "" {
						currency = charge.ChargeAmount.CurrencyCode
					}
				}
			}
			n, _ := s.upsertFinancialEvent(accountID, marketplaceID, e.AmazonOrderId, "Refund", e.PostedDate, refundAmt, 0, 0, currency)
			records += n
		}

		// 处理服务费事件
		for _, e := range resp.Payload.FinancialEvents.ServiceFeeEventList {
			var feeAmt float64
			currency := "USD"
			for _, f := range e.FeeList {
				feeAmt += f.FeeAmount.CurrencyAmount
				if f.FeeAmount.CurrencyCode != "" {
					currency = f.FeeAmount.CurrencyCode
				}
			}
			n, _ := s.upsertFinancialEvent(accountID, marketplaceID, e.AmazonOrderId, "ServiceFee", e.PostedDate, 0, 0, feeAmt, currency)
			records += n
		}

		nextToken = resp.Payload.NextToken
		if nextToken == "" {
			break
		}

		select {
		case <-ctx.Done():
			logSyncEnd(s.db, logID, "failed", records, "context canceled")
			return records, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}

	logSyncEnd(s.db, logID, "success", records, "")
	slog.Info("财务事件同步完成", "account", accountID, "records", records)
	return records, nil
}

// upsertFinancialEvent 写入单条财务事件
func (s *FinanceSyncService) upsertFinancialEvent(
	accountID int64, marketplaceID, orderID, eventType, postedDate string,
	principal, tax, fee float64, currency string,
) (int, error) {
	_, err := s.db.Exec(`
		INSERT INTO financial_events
			(account_id, marketplace_id, amazon_order_id, event_type, posted_date,
			 principal_amount, tax_amount, marketplace_fee, total_amount, currency_code)
		VALUES (?,?,?,?,?,?,?,?,?,?)
	`, accountID, marketplaceID, orderID, eventType, postedDate,
		principal, tax, fee, principal+tax+fee, currency)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// SyncSettlementReports 同步结算报告（创建报告 → 轮询 → 下载 → TSV 解析）
func (s *FinanceSyncService) SyncSettlementReports(ctx context.Context, accountID int64) (int, error) {
	logID := logSyncStart(s.db, accountID, "", "settlement_reports")

	// 创建报告请求
	type createReportReq struct {
		ReportType string `json:"reportType"`
	}
	var createResult struct {
		ReportID string `json:"reportId"`
	}
	if err := s.client.Post(ctx, "/reports/2021-06-30/reports",
		createReportReq{ReportType: "GET_V2_SETTLEMENT_REPORT_DATA_FLAT_FILE_V2"}, &createResult); err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("创建结算报告失败: %w", err)
	}
	if createResult.ReportID == "" {
		logSyncEnd(s.db, logID, "failed", 0, "报告ID为空")
		return 0, fmt.Errorf("报告 ID 为空")
	}

	// 轮询报告状态（最多等 10 分钟）
	var reportDocID string
	for i := 0; i < 20; i++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(30 * time.Second):
		}
		var status struct {
			ProcessingStatus string `json:"processingStatus"`
			ReportDocumentID string `json:"reportDocumentId"`
		}
		if err := s.client.Get(ctx, "/reports/2021-06-30/reports/"+createResult.ReportID, nil, &status); err != nil {
			continue
		}
		if status.ProcessingStatus == "DONE" {
			reportDocID = status.ReportDocumentID
			break
		}
		if status.ProcessingStatus == "FATAL" {
			logSyncEnd(s.db, logID, "failed", 0, "报告处理失败")
			return 0, fmt.Errorf("结算报告处理失败")
		}
	}
	if reportDocID == "" {
		logSyncEnd(s.db, logID, "failed", 0, "报告超时")
		return 0, fmt.Errorf("结算报告生成超时")
	}

	// 获取下载地址
	var docResult struct {
		URL                  string `json:"url"`
		CompressionAlgorithm string `json:"compressionAlgorithm"`
	}
	if err := s.client.Get(ctx, "/reports/2021-06-30/documents/"+reportDocID, nil, &docResult); err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}
	slog.Info("结算报告下载链接已获取", "docId", reportDocID)

	// 下载 TSV 文件内容
	tsvContent, err := s.client.DownloadReportDocument(ctx, docResult.URL, docResult.CompressionAlgorithm)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("下载结算报告失败: %w", err)
	}

	// 解析 TSV 并写入数据库
	count, err := parseAndSaveSettlementTSV(s.db, accountID, tsvContent)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}
	logSyncEnd(s.db, logID, "success", count, "")
	slog.Info("结算报告同步完成", "account", accountID, "settlements", count)
	return count, nil
}

// parseAndSaveSettlementTSV 解析亚马逊结算报告 TSV 并写入数据库
// TSV 关键列：settlement-id, settlement-start-date, settlement-end-date, deposit-date,
//           total-amount, currency, transaction-type, order-id, amount-type, amount-description, amount
func parseAndSaveSettlementTSV(db *database.DB, accountID int64, content string) (int, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return 0, nil
	}

	// 解析表头，建立列名→索引映射
	headers := strings.Split(lines[0], "\t")
	colIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		colIdx[strings.TrimSpace(strings.ToLower(h))] = i
	}

	// 辅助函数：安全获取列属性
	getCol := func(row []string, name string) string {
		i, ok := colIdx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	// 按 settlement-id 分组汇总
	type settlementAgg struct {
		SettlementID string
		StartDate    string
		EndDate      string
		DepositDate  string
		TotalAmount  float64
		Currency     string
	}
	settlements := make(map[string]*settlementAgg)

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		row := strings.Split(line, "\t")
		sid := getCol(row, "settlement-id")
		if sid == "" {
			continue
		}
		if _, ok := settlements[sid]; !ok {
			settlements[sid] = &settlementAgg{
				SettlementID: sid,
				StartDate:    normalizeSettlementDate(getCol(row, "settlement-start-date")),
				EndDate:      normalizeSettlementDate(getCol(row, "settlement-end-date")),
				DepositDate:  normalizeSettlementDate(getCol(row, "deposit-date")),
				Currency:     getCol(row, "currency"),
			}
		}
		// 加总 total-amount 列（仅计汇总行，按 transaction-type=Transfer或空行）
		txType := strings.ToLower(getCol(row, "transaction-type"))
		if txType == "transfer" || txType == "" {
			amtStr := getCol(row, "total-amount")
			if amtStr == "" {
				amtStr = getCol(row, "amount")
			}
			if amt, err := strconv.ParseFloat(strings.ReplaceAll(amtStr, ",", ""), 64); err == nil {
				settlements[sid].TotalAmount += amt
			}
		}
	}

	// 写入数据库
	count := 0
	for _, agg := range settlements {
		_, err := db.Exec(`
			INSERT INTO settlement_reports
				(account_id, settlement_id, settlement_start_date, settlement_end_date, deposit_date, total_amount, currency_code)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(account_id, settlement_id) DO UPDATE SET
				total_amount=excluded.total_amount,
				deposit_date=excluded.deposit_date
		`, accountID, agg.SettlementID,
			agg.StartDate, agg.EndDate, agg.DepositDate, agg.TotalAmount, agg.Currency)
		if err != nil {
			slog.Warn("写入结算记录失败", "sid", agg.SettlementID, "error", err)
			continue
		}
		count++
	}
	return count, nil
}

// normalizeSettlementDate 将 Amazon 返回的日期（含时区/TSV 空格）转为 YYYY-MM-DD
func normalizeSettlementDate(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	s = parts[0] // 取第一个空格分隔部分
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}


// ── CSV 成本批量导入 ──────────────────────────────────────

// CostImportResult CSV 导入结果
type CostImportResult struct {
	Imported int      `json:"imported"`
	Errors   []string `json:"errors"`
}

// ImportCostCSV 从 CSV 字符串批量导入 COGS（格式：sku,cost[,currency]）
func ImportCostCSV(db *database.DB, accountID int64, csvContent string, defaultCurrency string) CostImportResult {
	lines := strings.Split(strings.ReplaceAll(csvContent, "\r\n", "\n"), "\n")
	result := CostImportResult{Errors: []string{}}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 跳过表头行
		lower := strings.ToLower(line)
		if i == 0 && (strings.HasPrefix(lower, "sku") || strings.HasPrefix(lower, "seller") || strings.HasPrefix(lower, "\"sku")) {
			continue
		}

		fields := parseCsvLine(line)
		if len(fields) < 2 {
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行格式错误（需至少 2 列）: %s", i+1, line))
			continue
		}

		sku := strings.TrimSpace(fields[0])
		costStr := strings.TrimSpace(fields[1])
		currency := defaultCurrency
		if len(fields) >= 3 {
			if c := strings.TrimSpace(fields[2]); c != "" {
				currency = c
			}
		}

		if sku == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行 SKU 为空", i+1))
			continue
		}
		cost, err := strconv.ParseFloat(costStr, 64)
		if err != nil || cost < 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行成本格式错误: %q", i+1, costStr))
			continue
		}

		_, err = db.Exec(`
			INSERT INTO product_costs(account_id, seller_sku, cost_currency, unit_cost, effective_from)
			VALUES (?,?,?,?,date('now'))
			ON CONFLICT(account_id, seller_sku, effective_from) DO UPDATE SET
				unit_cost=excluded.unit_cost, cost_currency=excluded.cost_currency
		`, accountID, sku, currency, cost)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行保存失败: %s", i+1, err.Error()))
			continue
		}
		result.Imported++
	}

	return result
}

// parseCsvLine 解析单行 CSV（支持引号包裹）
func parseCsvLine(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '"' {
			inQuote = !inQuote
		} else if ch == ',' && !inQuote {
			fields = append(fields, cur.String())
			cur.Reset()
		} else {
			cur.WriteByte(ch)
		}
	}
	fields = append(fields, cur.String())
	return fields
}

// GetProductCosts 获取当前成本列表
func GetProductCosts(db *database.DB, accountID int64) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT seller_sku, cost_currency, unit_cost, effective_from
		FROM product_costs
		WHERE account_id=?
		ORDER BY seller_sku
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var sku, currency, date string
		var cost float64
		if err := rows.Scan(&sku, &currency, &cost, &date); err != nil { slog.Warn("扫描成本行失败", "error", err); continue }
		result = append(result, map[string]interface{}{
			"sku":      sku,
			"currency": currency,
			"cost":     cost,
			"date":     date,
		})
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return result, nil
}

// ExportSalesCSV 导出销售数据为 CSV 字符串
func ExportSalesCSV(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	rows, err := db.Query(`
		SELECT date, ordered_product_sales, units_ordered, page_views, sessions,
			unit_session_percentage, buy_box_percentage
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?
		ORDER BY date
	`, accountID, marketplaceID, dateStart, dateEnd)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("日期,销售额,销量(件),页面浏览,访客数,转化率(%),Buy Box占比(%)\n")
	for rows.Next() {
		var date string
		var sales float64
		var units, pv, sessions int
		var convRate, buyBox float64
		if err := rows.Scan(&date, &sales, &units, &pv, &sessions, &convRate, &buyBox); err != nil { slog.Warn("扫描销售 CSV 行失败", "error", err); continue }
		sb.WriteString(fmt.Sprintf("%s,%.2f,%d,%d,%d,%.2f,%.2f\n",
			date, sales, units, pv, sessions, convRate*100, buyBox*100))
	}
	return sb.String(), nil
}

// ExportInventoryCSV 导出库存快照为 CSV
func ExportInventoryCSV(db *database.DB, accountID int64, marketplaceID string) (string, error) {
	rows, err := db.Query(`
		SELECT seller_sku, asin, fulfillable_qty, inbound_qty, unsellable_qty, snapshot_date
		FROM inventory_snapshots
		WHERE account_id=? AND marketplace_id=?
		  AND snapshot_date=(SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id=? AND marketplace_id=?)
		ORDER BY fulfillable_qty ASC
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("SKU,ASIN,可售库存,在途库存,不可售库存,快照日期\n")
	for rows.Next() {
		var sku, asin, date string
		var fulfillable, inbound, unsellable int
		if err := rows.Scan(&sku, &asin, &fulfillable, &inbound, &unsellable, &date); err != nil { slog.Warn("扫描库存 CSV 行失败", "error", err); continue }
		sb.WriteString(fmt.Sprintf("%q,%q,%d,%d,%d,%s\n", sku, asin, fulfillable, inbound, unsellable, date))
	}
	return sb.String(), nil
}

// ExportAdvertisingCSV 导出广告活动汇总为 CSV
func ExportAdvertisingCSV(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	rows, err := db.Query(`
		SELECT c.campaign_id, c.name, c.state,
			COALESCE(SUM(p.impressions),0), COALESCE(SUM(p.clicks),0),
			COALESCE(SUM(p.cost),0), COALESCE(SUM(p.attributed_sales_7d),0),
			COALESCE(SUM(p.attributed_conversions_7d),0),
			CASE WHEN SUM(p.attributed_sales_7d)>0 THEN SUM(p.cost)/SUM(p.attributed_sales_7d)*100 ELSE 0 END
		FROM ad_campaigns c
		LEFT JOIN ad_performance_daily p ON c.campaign_id=p.campaign_id AND c.account_id=p.account_id
			AND p.date>=? AND p.date<=?
		WHERE c.account_id=? AND c.marketplace_id=?
		GROUP BY c.campaign_id
		ORDER BY SUM(p.cost) DESC
	`, dateStart, dateEnd, accountID, marketplaceID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("活动ID,活动名称,状态,展示量,点击量,花费,归因销售额,转化量,ACoS(%)\n")
	for rows.Next() {
		var cid, name, state string
		var impressions, clicks, conversions int
		var cost, sales, acos float64
		if err := rows.Scan(&cid, &name, &state, &impressions, &clicks, &cost, &sales, &conversions, &acos); err != nil {
			slog.Warn("扫描广告 CSV 行失败", "error", err)
			continue
		}
		sb.WriteString(fmt.Sprintf("%q,%q,%s,%d,%d,%.2f,%.2f,%d,%.2f\n",
			cid, name, state, impressions, clicks, cost, sales, conversions, acos))
	}
	return sb.String(), nil
}

// ExportFinanceCSV 导出财务利润明细为 CSV
func ExportFinanceCSV(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	// 汇总 AdSpend / Fees / COGS 作为一次性查询
	// ⚠️ 修复严重 BUG：必须放在 db.Query 之前执行！因为 SQLite 的最大连接数为 1 (MaxOpenConns=1)
	// 如果在 rows 未关闭的情况下执行 QueryRow，会陷入死锁等待，导致全站所有需要数据库请求的功能卡死！
	var totalAdSpend, totalFees, totalCOGS float64
	db.QueryRow(`SELECT COALESCE(SUM(cost),0) FROM ad_performance_daily apd
		JOIN ad_campaigns ac ON apd.campaign_id=ac.campaign_id AND apd.account_id=ac.account_id
		WHERE apd.account_id=? AND ac.marketplace_id=? AND apd.date>=? AND apd.date<=?`,
		accountID, marketplaceID, dateStart, dateEnd).Scan(&totalAdSpend)
	db.QueryRow(`SELECT COALESCE(SUM(marketplace_fee + fba_fee),0) FROM financial_events
		WHERE account_id=? AND marketplace_id=? AND posted_date>=? AND posted_date<=?`,
		accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalFees)
	db.QueryRow(`SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered),0)
		FROM order_items oi JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
		JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
		WHERE o.account_id=? AND o.marketplace_id=? AND o.purchase_date>=? AND o.purchase_date<=?
		AND o.order_status NOT IN ('Canceled','Declined')`,
		accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalCOGS)

	rows, err := db.Query(`
		SELECT date, ordered_product_sales, units_ordered
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?
		ORDER BY date
	`, accountID, marketplaceID, dateStart, dateEnd)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("日期,销售额,销量(件)\n")
	var grandSales float64
	var grandUnits int
	for rows.Next() {
		var date string
		var sales float64
		var units int
		if err := rows.Scan(&date, &sales, &units); err != nil { slog.Warn("扫描财务 CSV 行失败", "error", err); continue }
		sb.WriteString(fmt.Sprintf("%s,%.2f,%d\n", date, sales, units))
		grandSales += sales
		grandUnits += units
	}
	// 追加汇总行
	netProfit := grandSales - totalAdSpend - totalFees - totalCOGS
	margin := 0.0
	if grandSales > 0 {
		margin = netProfit / grandSales * 100
	}
	sb.WriteString(fmt.Sprintf("\n汇总,%.2f,%d\n", grandSales, grandUnits))
	sb.WriteString(fmt.Sprintf("广告花费,%.2f,\n", totalAdSpend))
	sb.WriteString(fmt.Sprintf("平台费用,%.2f,\n", totalFees))
	sb.WriteString(fmt.Sprintf("采购成本(COGS),%.2f,\n", totalCOGS))
	sb.WriteString(fmt.Sprintf("净利润,%.2f,\n", netProfit))
	sb.WriteString(fmt.Sprintf("净利率,%.2f%%,\n", margin))
	return sb.String(), nil
}

// parseSettlementJSON helper（保留供未来使用）
func parseSettlementJSON(data []byte) (map[string]interface{}, error) {
	var result map[string]interface{}
	return result, json.Unmarshal(data, &result)
}

// minInt 工具函数
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
