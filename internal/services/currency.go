// Package services - 汇率服务 + 多账户合并 KPI
// 汇率数据来源：免费公共 API（exchangerate-api.com 免费版 / frankfurter.app）
// 基准货币：CNY（人民币）
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"voyage/internal/database"
)

// CurrencyService 汇率管理服务
type CurrencyService struct {
	db *database.DB
}

func NewCurrencyService(db *database.DB) *CurrencyService {
	return &CurrencyService{db: db}
}

// CurrencyRate 汇率数据
type CurrencyRate struct {
	CurrencyCode string  `json:"currencyCode"`
	RateToCNY    float64 `json:"rateToCny"`
	UpdatedAt    string  `json:"updatedAt"`
}

// frankfurterResponse frankfurter.app API 响应（完全免费，无需注册）
type frankfurterResponse struct {
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

// SyncExchangeRates 从免费 API 获取最新汇率，以 CNY 为基准
// 数据源：https://api.frankfurter.app/latest?base=CNY
// 完全免费，无需 API Key，每日限流约 1000 次
func (s *CurrencyService) SyncExchangeRates(ctx context.Context) error {
	slog.Info("开始同步汇率数据")

	// frankfurter.app 免费汇率 API（欧央行数据源）
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.frankfurter.app/latest?base=CNY", nil)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// 回退到 exchangerate-api
		slog.Warn("frankfurter API 失败，尝试备用源", "error", err)
		return s.syncFromBackupAPI(ctx)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("frankfurter API HTTP 错误", "status", resp.StatusCode)
		return s.syncFromBackupAPI(ctx)
	}

	body, _ := io.ReadAll(resp.Body)
	var result frankfurterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析汇率数据失败: %w", err)
	}

	// frankfurter 返回的是 1 CNY = ? 外币，我们需要的是 1 外币 = ? CNY
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for code, rate := range result.Rates {
		if rate <= 0 {
			continue
		}
		rateToCNY := 1.0 / rate // 转换：1 外币 = 1/rate CNY
		s.db.Exec(`
			INSERT INTO currency_rates (currency_code, rate_to_cny, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(currency_code) DO UPDATE SET rate_to_cny=excluded.rate_to_cny, updated_at=excluded.updated_at
		`, code, rateToCNY, now)
	}

	// CNY 本身
	s.db.Exec(`INSERT INTO currency_rates (currency_code, rate_to_cny, updated_at)
		VALUES ('CNY', 1.0, ?)
		ON CONFLICT(currency_code) DO UPDATE SET rate_to_cny=1.0, updated_at=excluded.updated_at`, now)

	slog.Info("汇率同步完成", "currencies", len(result.Rates), "date", result.Date)
	return nil
}

// syncFromBackupAPI 备用汇率 API
func (s *CurrencyService) syncFromBackupAPI(ctx context.Context) error {
	// 使用 open.er-api.com（完全免费，无需注册）
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://open.er-api.com/v6/latest/CNY", nil)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("备用汇率 API 也失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析备用汇率数据失败: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for code, rate := range result.Rates {
		if rate <= 0 {
			continue
		}
		rateToCNY := 1.0 / rate
		s.db.Exec(`INSERT INTO currency_rates (currency_code, rate_to_cny, updated_at)
			VALUES (?, ?, ?) ON CONFLICT(currency_code) DO UPDATE SET rate_to_cny=excluded.rate_to_cny, updated_at=excluded.updated_at`,
			code, rateToCNY, now)
	}
	slog.Info("备用汇率同步完成", "currencies", len(result.Rates))
	return nil
}

// GetAllRates 获取所有汇率
func (s *CurrencyService) GetAllRates() ([]CurrencyRate, error) {
	// 使用只读连接，避免阻塞写操作
	rows, err := s.db.ReadQuery(`SELECT currency_code, rate_to_cny, updated_at FROM currency_rates ORDER BY currency_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rates []CurrencyRate
	for rows.Next() {
		var r CurrencyRate
		if err := rows.Scan(&r.CurrencyCode, &r.RateToCNY, &r.UpdatedAt); err != nil {
			continue
		}
		rates = append(rates, r)
	}
	return rates, nil
}

// ConvertToCNY 将金额转换为 CNY——使用只读连接，避免在循环中阻塞写操作
func (s *CurrencyService) ConvertToCNY(amount float64, fromCurrency string) float64 {
	if fromCurrency == "CNY" {
		return amount
	}
	var rate float64
	s.db.ReadQueryRow(`SELECT rate_to_cny FROM currency_rates WHERE currency_code=?`, fromCurrency).Scan(&rate)
	if rate <= 0 {
		return amount // 找不到汇率则原样返回
	}
	return amount * rate
}

// ── 多账户合并 KPI ─────────────────────────────────────────

// CrossAccountKPI 跨账户汇总 KPI（以 CNY 为基准）
type CrossAccountKPI struct {
	TotalSalesCNY     float64              `json:"totalSalesCny"`
	TotalOrderCount   int                  `json:"totalOrderCount"`
	TotalAdSpendCNY   float64              `json:"totalAdSpendCny"`
	TotalFeesCNY      float64              `json:"totalFeesCny"`
	TotalCogsCNY      float64              `json:"totalCogsCny"`
	TotalNetProfitCNY float64              `json:"totalNetProfitCny"`
	TotalProfitMargin float64              `json:"totalProfitMargin"` // %
	AccountBreakdown  []AccountKPISummary  `json:"accountBreakdown"`
	BaseCurrency      string               `json:"baseCurrency"`
	RateUpdatedAt     string               `json:"rateUpdatedAt"`
}

// AccountKPISummary 单账户 KPI 摘要
type AccountKPISummary struct {
	AccountID        int64   `json:"accountId"`
	AccountName      string  `json:"accountName"`
	MarketplaceID    string  `json:"marketplaceId"`
	MarketplaceName  string  `json:"marketplaceName"`
	OriginalCurrency string  `json:"originalCurrency"`
	Sales            float64 `json:"sales"`
	SalesCNY         float64 `json:"salesCny"`
	Orders           int     `json:"orders"`
	AdSpend          float64 `json:"adSpend"`
	AdSpendCNY       float64 `json:"adSpendCny"`
	Fees             float64 `json:"fees"`
	FeesCNY          float64 `json:"feesCny"`
	Cogs             float64 `json:"cogs"`
	CogsCNY          float64 `json:"cogsCny"`
	NetProfit        float64 `json:"netProfit"`
	NetProfitCNY     float64 `json:"netProfitCny"`
	ProfitMargin     float64 `json:"profitMargin"` // %
	ProfitSharePct   float64 `json:"profitSharePct"` // 利润贡献占比 %
}

// GetCrossAccountKPI 获取多账户合并 KPI（含完整利润数据）
func GetCrossAccountKPI(db *database.DB, dateStart, dateEnd string) (*CrossAccountKPI, error) {
	currSvc := NewCurrencyService(db)

	// 使用只读连接查询所有活跃账户的所有站点数据
	rows, err := db.ReadQuery(`
		SELECT
			a.id, a.name,
			am.marketplace_id,
			m.name as mp_name,
			m.currency_code,
			COALESCE(SUM(s.ordered_product_sales), 0) as sales,
			COALESCE(SUM(s.units_ordered), 0) as orders
		FROM accounts a
		JOIN account_marketplaces am ON a.id = am.account_id
		JOIN marketplace m ON am.marketplace_id = m.marketplace_id
		LEFT JOIN sales_traffic_daily s ON s.account_id = a.id
			AND s.marketplace_id = am.marketplace_id
			AND s.date >= ? AND s.date <= ?
		WHERE a.is_active = 1
		GROUP BY a.id, am.marketplace_id
		ORDER BY sales DESC
	`, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &CrossAccountKPI{BaseCurrency: "CNY"}

	// ⚠️ 两阶段模式：先读到内存，再逐行查询广告/费用/COGS
	type kpiRow struct {
		accountID      int64
		accountName    string
		marketplaceID  string
		mpName         string
		currency       string
		sales          float64
		orders         int
	}
	var kpiItems []kpiRow
	for rows.Next() {
		var r kpiRow
		if err := rows.Scan(&r.accountID, &r.accountName, &r.marketplaceID,
			&r.mpName, &r.currency, &r.sales, &r.orders); err != nil {
			continue
		}
		kpiItems = append(kpiItems, r)
	}
	rows.Close() // 释放连接

	for _, r := range kpiItems {
		item := AccountKPISummary{
			AccountID:        r.accountID,
			AccountName:      r.accountName,
			MarketplaceID:    r.marketplaceID,
			MarketplaceName:  r.mpName,
			OriginalCurrency: r.currency,
			Sales:            r.sales,
			Orders:           r.orders,
		}

		// 转换为 CNY
		item.SalesCNY = currSvc.ConvertToCNY(item.Sales, item.OriginalCurrency)

		// 查询广告花费
		db.ReadQueryRow(`
			SELECT COALESCE(SUM(apd.cost), 0)
			FROM ad_performance_daily apd
			JOIN ad_campaigns ac ON apd.campaign_id=ac.campaign_id AND apd.account_id=ac.account_id
			WHERE apd.account_id=? AND ac.marketplace_id=? AND apd.date>=? AND apd.date<=?
		`, item.AccountID, item.MarketplaceID, dateStart, dateEnd).Scan(&item.AdSpend)
		item.AdSpendCNY = currSvc.ConvertToCNY(item.AdSpend, item.OriginalCurrency)

		// 查询平台费用（佣金 + FBA）
		db.ReadQueryRow(`SELECT COALESCE(SUM(marketplace_fee + fba_fee), 0) FROM financial_events
			WHERE account_id=? AND marketplace_id=? AND posted_date>=? AND posted_date<=?`,
			item.AccountID, item.MarketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&item.Fees)
		item.FeesCNY = currSvc.ConvertToCNY(item.Fees, item.OriginalCurrency)

		// 查询 COGS（采购成本）
		db.ReadQueryRow(`SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
			FROM order_items oi
			JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
			JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
			WHERE o.account_id=? AND o.marketplace_id=?
			  AND o.purchase_date>=? AND o.purchase_date<=?
			  AND o.order_status NOT IN ('Canceled','Declined')`,
			item.AccountID, item.MarketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&item.Cogs)
		item.CogsCNY = currSvc.ConvertToCNY(item.Cogs, item.OriginalCurrency)

		// 计算净利润（原始货币）
		item.NetProfit = item.Sales - item.AdSpend - absFloat(item.Fees) - item.Cogs
		// 直接用已转换的 CNY 分量计算，避免额外调用 ConvertToCNY + 消除精度损失
		item.NetProfitCNY = item.SalesCNY - item.AdSpendCNY - absFloat(item.FeesCNY) - item.CogsCNY
		if item.Sales > 0 {
			item.ProfitMargin = item.NetProfit / item.Sales * 100
		}

		result.TotalSalesCNY += item.SalesCNY
		result.TotalOrderCount += item.Orders
		result.TotalAdSpendCNY += item.AdSpendCNY
		result.TotalFeesCNY += absFloat(item.FeesCNY)
		result.TotalCogsCNY += item.CogsCNY
		result.TotalNetProfitCNY += item.NetProfitCNY
		result.AccountBreakdown = append(result.AccountBreakdown, item)
	}

	// 计算总利润率 + 各账户利润贡献占比
	if result.TotalSalesCNY > 0 {
		result.TotalProfitMargin = result.TotalNetProfitCNY / result.TotalSalesCNY * 100
	}
	for i := range result.AccountBreakdown {
		if result.TotalNetProfitCNY != 0 {
			result.AccountBreakdown[i].ProfitSharePct = result.AccountBreakdown[i].NetProfitCNY / absFloat(result.TotalNetProfitCNY) * 100
		}
	}

	// 汇率更新时间
	db.ReadQueryRow(`SELECT MIN(updated_at) FROM currency_rates`).Scan(&result.RateUpdatedAt)

	return result, nil
}

// absFloat 取绝对值
func absFloat(v float64) float64 {
	if v < 0 { return -v }
	return v
}
