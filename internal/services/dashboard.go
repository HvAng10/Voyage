// Package services 数据同步服务层 - 仪表盘数据聚合
package services

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"voyage/internal/database"
)

// DashboardKPI 仪表盘 KPI 数据
type DashboardKPI struct {
	// 销售额
	TotalSales      float64 `json:"totalSales"`
	SalesPrev       float64 `json:"salesPrev"`       // 上期对比
	SalesTrend      float64 `json:"salesTrend"`      // 环比变化百分比

	// 订单数
	TotalOrders     int     `json:"totalOrders"`
	OrdersPrev      int     `json:"ordersPrev"`
	OrdersTrend     float64 `json:"ordersTrend"`

	// 广告花费
	AdSpend         float64 `json:"adSpend"`
	AdSpendPrev     float64 `json:"adSpendPrev"`
	AdSpendTrend    float64 `json:"adSpendTrend"`

	// ACoS（广告成本销售比）
	ACoS            float64 `json:"acos"`
	ACoSPrev        float64 `json:"acosPrev"`
	ACoSTrend       float64 `json:"acosTrend"`

	// 净利润（需要成本数据）
	NetProfit       float64 `json:"netProfit"`
	NetProfitTrend  float64 `json:"netProfitTrend"`

	// 货币单位
	Currency        string  `json:"currency"`

	// 数据日期范围
	DateStart       string  `json:"dateStart"`
	DateEnd         string  `json:"dateEnd"`

	// 数据延迟提示
	DataLatencyDays int     `json:"dataLatencyDays"` // Data Kiosk 延迟天数
}

// DailyDataPoint 每日数据点（用于折线图/柱状图）
type DailyDataPoint struct {
	Date           string  `json:"date"`
	Sales          float64 `json:"sales"`
	Orders         int     `json:"orders"`
	Units          int     `json:"units"`          // 与 orders 同值，兼容前端
	AdSpend        float64 `json:"adSpend"`
	PageViews      int     `json:"pageViews"`
	Sessions       int     `json:"sessions"`
	ConversionRate float64 `json:"conversionRate"` // 转化率 = orders/sessions * 100
	ACoS           float64 `json:"acos"`
}

// DashboardService 仪表盘数据查询服务
type DashboardService struct {
	db *database.DB
}

func NewDashboardService(db *database.DB) *DashboardService {
	return &DashboardService{db: db}
}

// GetKPI 获取指定账户和站点的 KPI 数据
// dateStart / dateEnd: YYYY-MM-DD 格式（店铺本地日期）
func (s *DashboardService) GetKPI(accountID int64, marketplaceID, dateStart, dateEnd string) (*DashboardKPI, error) {
	kpi := &DashboardKPI{Currency: "USD", DataLatencyDays: 2}

	// 查询当期货币单位
	s.db.QueryRow(
		"SELECT currency_code FROM marketplace WHERE marketplace_id = ?", marketplaceID,
	).Scan(&kpi.Currency)

	kpi.DateStart = dateStart
	kpi.DateEnd = dateEnd

	// ── 当期销售额和订单量（来自 sales_traffic_daily）
	err := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(ordered_product_sales), 0),
			COALESCE(SUM(units_ordered), 0)
		FROM sales_traffic_daily
		WHERE account_id = ? AND marketplace_id = ?
		  AND date >= ? AND date <= ?
	`, accountID, marketplaceID, dateStart, dateEnd).Scan(
		&kpi.TotalSales, &kpi.TotalOrders,
	)
	if err != nil {
		return nil, fmt.Errorf("查询销售数据失败: %w", err)
	}

	// ── 广告花费和 ACoS（来自 ad_performance_daily）
	s.db.QueryRow(`
		SELECT
			COALESCE(SUM(cost), 0),
			COALESCE(SUM(attributed_sales_7d), 0)
		FROM ad_performance_daily apd
		JOIN ad_campaigns ac ON apd.campaign_id = ac.campaign_id AND apd.account_id = ac.account_id
		WHERE apd.account_id = ? AND ac.marketplace_id = ?
		  AND apd.date >= ? AND apd.date <= ?
	`, accountID, marketplaceID, dateStart, dateEnd).Scan(
		&kpi.AdSpend, new(float64),
	)

	if kpi.TotalSales > 0 && kpi.AdSpend > 0 {
		kpi.ACoS = kpi.AdSpend / kpi.TotalSales * 100
	}

	// ── 净利润（销售额 - 广告费 - 平台费 - COGS）
	var totalFees float64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(marketplace_fee + fba_fee), 0)
		FROM financial_events
		WHERE account_id = ? AND marketplace_id = ?
		  AND posted_date >= ? AND posted_date <= ?
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalFees)

	// COGS 扣减（与 FinanceSummary 保持一致口径）
	var totalCOGS float64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
		FROM order_items oi
		JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
		JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
		WHERE o.account_id=? AND o.marketplace_id=?
		  AND o.purchase_date>=? AND o.purchase_date<=?
		  AND o.order_status NOT IN ('Canceled','Declined')
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalCOGS)

	kpi.NetProfit = kpi.TotalSales - kpi.AdSpend - totalFees - totalCOGS

	// ── 环比趋势计算：自动推算等长上期区间 ──
	prevStart, prevEnd := calcPrevPeriod(dateStart, dateEnd)
	if prevStart != "" && prevEnd != "" {
		// 上期销售额和订单量
		s.db.QueryRow(`
			SELECT COALESCE(SUM(ordered_product_sales), 0), COALESCE(SUM(units_ordered), 0)
			FROM sales_traffic_daily
			WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?
		`, accountID, marketplaceID, prevStart, prevEnd).Scan(&kpi.SalesPrev, &kpi.OrdersPrev)

		// 上期广告花费
		s.db.QueryRow(`
			SELECT COALESCE(SUM(cost), 0)
			FROM ad_performance_daily apd
			JOIN ad_campaigns ac ON apd.campaign_id=ac.campaign_id AND apd.account_id=ac.account_id
			WHERE apd.account_id=? AND ac.marketplace_id=? AND apd.date>=? AND apd.date<=?
		`, accountID, marketplaceID, prevStart, prevEnd).Scan(&kpi.AdSpendPrev)

		// 上期 ACoS
		if kpi.SalesPrev > 0 && kpi.AdSpendPrev > 0 {
			kpi.ACoSPrev = kpi.AdSpendPrev / kpi.SalesPrev * 100
		}

		// 计算各指标的环比变化百分比
		kpi.SalesTrend = calcTrendPct(kpi.TotalSales, kpi.SalesPrev)
		kpi.OrdersTrend = calcTrendPct(float64(kpi.TotalOrders), float64(kpi.OrdersPrev))
		kpi.AdSpendTrend = calcTrendPct(kpi.AdSpend, kpi.AdSpendPrev)
		kpi.ACoSTrend = calcTrendPct(kpi.ACoS, kpi.ACoSPrev)

		// 上期净利润
		var prevFees, prevCOGS float64
		s.db.QueryRow(`SELECT COALESCE(SUM(marketplace_fee + fba_fee), 0) FROM financial_events
			WHERE account_id=? AND marketplace_id=? AND posted_date>=? AND posted_date<=?`,
			accountID, marketplaceID, prevStart+" 00:00:00", prevEnd+" 23:59:59").Scan(&prevFees)
		s.db.QueryRow(`SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
			FROM order_items oi JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
			JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
			WHERE o.account_id=? AND o.marketplace_id=? AND o.purchase_date>=? AND o.purchase_date<=?
			AND o.order_status NOT IN ('Canceled','Declined')`,
			accountID, marketplaceID, prevStart+" 00:00:00", prevEnd+" 23:59:59").Scan(&prevCOGS)
		prevProfit := kpi.SalesPrev - kpi.AdSpendPrev - prevFees - prevCOGS
		kpi.NetProfitTrend = calcTrendPct(kpi.NetProfit, prevProfit)
	}

	return kpi, nil
}

// GetDailyTrend 获取每日销售趋势数据（用于仪表盘趋势图）
func (s *DashboardService) GetDailyTrend(accountID int64, marketplaceID, dateStart, dateEnd string) ([]DailyDataPoint, error) {
	rows, err := s.db.Query(`
		SELECT
			t.date,
			COALESCE(t.ordered_product_sales, 0),
			COALESCE(t.units_ordered, 0),
			COALESCE(t.page_views, 0),
			COALESCE(t.sessions, 0),
			COALESCE(a.day_spend, 0),
			COALESCE(a.day_acos, 0)
		FROM sales_traffic_daily t
		LEFT JOIN (
			SELECT
				apd.date,
				SUM(apd.cost) as day_spend,
				CASE WHEN SUM(apd.attributed_sales_7d) > 0
					THEN SUM(apd.cost) / SUM(apd.attributed_sales_7d) * 100
					ELSE 0
				END as day_acos
			FROM ad_performance_daily apd
			JOIN ad_campaigns ac ON apd.campaign_id = ac.campaign_id AND apd.account_id = ac.account_id
			WHERE apd.account_id = ? AND ac.marketplace_id = ?
			GROUP BY apd.date
		) a ON t.date = a.date
		WHERE t.account_id = ? AND t.marketplace_id = ?
		  AND t.date >= ? AND t.date <= ?
		ORDER BY t.date ASC
	`, accountID, marketplaceID, accountID, marketplaceID, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []DailyDataPoint
	for rows.Next() {
		var p DailyDataPoint
		if err := rows.Scan(
			&p.Date, &p.Sales, &p.Orders,
			&p.PageViews, &p.Sessions, &p.AdSpend, &p.ACoS,
		); err != nil {
			return nil, err
		}
		if p.Sessions > 0 {
			p.ConversionRate = float64(p.Orders) / float64(p.Sessions) * 100
		}
		p.Units = p.Orders // 兼容前端字段名
		points = append(points, p)
	}
	return points, nil
}

// AsinDailyPoint ASIN 每日数据点（商品趋势图用）
type AsinDailyPoint struct {
	Date     string  `json:"date"`
	Sales    float64 `json:"sales"`
	Units    int     `json:"units"`
	Sessions int     `json:"sessions"`
}

// GetAsinDailyTrend 获取指定 ASIN 的每日销售趋势，来源：sales_traffic_by_asin（Data Kiosk T+2）
func (s *DashboardService) GetAsinDailyTrend(accountID int64, marketplaceID, asin, dateStart, dateEnd string) ([]AsinDailyPoint, error) {
	rows, err := s.db.Query(`
		SELECT date,
			COALESCE(ordered_product_sales, 0),
			COALESCE(units_ordered, 0),
			COALESCE(sessions, 0)
		FROM sales_traffic_by_asin
		WHERE account_id = ? AND marketplace_id = ? AND asin = ?
		  AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, accountID, marketplaceID, asin, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []AsinDailyPoint
	for rows.Next() {
		var p AsinDailyPoint
		if err := rows.Scan(&p.Date, &p.Sales, &p.Units, &p.Sessions); err != nil {
			slog.Warn("扫描 ASIN 日趋势行失败", "error", err)
			continue
		}
		points = append(points, p)
	}
	return points, nil
}

// AsinFeeInfo ASIN 实际费率信息（来自财务事件，非估算）
type AsinFeeInfo struct {
	TotalRevenue     float64 `json:"totalRevenue"`     // 商品总销售额
	TotalFbaFee      float64 `json:"totalFbaFee"`      // 实际 FBA 费用
	TotalRefFee      float64 `json:"totalRefFee"`      // 实际平台佣金（Referral Fee）
	TotalAdSpend     float64 `json:"totalAdSpend"`     // 广告花费（账户级别按比例估算）
	ActualFeeRate    float64 `json:"actualFeeRate"`    // 实际综合费率 %
	DataNote         string  `json:"dataNote"`         // 数据说明
}

// GetAsinFeeInfo 获取 ASIN 基于真实财务事件计算的费率
// 注：财务事件是账户级别，无法精确拆分到单 ASIN；此处采用账户期间平均费率作为准确估算
func (s *DashboardService) GetAsinFeeInfo(accountID int64, marketplaceID, asin, dateStart, dateEnd string) (*AsinFeeInfo, error) {
	info := &AsinFeeInfo{}

	// ASIN 自身的销售额
	s.db.QueryRow(`
		SELECT COALESCE(SUM(ordered_product_sales), 0)
		FROM sales_traffic_by_asin
		WHERE account_id=? AND marketplace_id=? AND asin=?
		  AND date>=? AND date<=?
	`, accountID, marketplaceID, asin, dateStart, dateEnd).Scan(&info.TotalRevenue)

	// 账户期间真实费率（从 financial_events 汇总佣金和 FBA 费用）
	var accountSales, accountFbaFee, accountRefFee float64
	s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN event_type='Order' THEN principal_amount ELSE 0 END), 0),
			COALESCE(ABS(SUM(CASE WHEN event_type='Order' THEN fba_fee ELSE 0 END)), 0),
			COALESCE(ABS(SUM(CASE WHEN event_type='Order' THEN marketplace_fee ELSE 0 END)), 0)
		FROM financial_events
		WHERE account_id=? AND marketplace_id=?
		  AND posted_date>=? AND posted_date<=?
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59",
	).Scan(&accountSales, &accountFbaFee, &accountRefFee)

	// 按 ASIN 占账户销售额比例推算（最接近真实的无 ASIN 维度财务事件的方法）
	if accountSales > 0 && info.TotalRevenue > 0 {
		ratio := info.TotalRevenue / accountSales
		info.TotalFbaFee = accountFbaFee * ratio
		info.TotalRefFee = accountRefFee * ratio

		// 综合费率 = (FBA费 + 平台佣金) / 销售额
		info.ActualFeeRate = (info.TotalFbaFee + info.TotalRefFee) / info.TotalRevenue * 100
		info.DataNote = fmt.Sprintf("基于期间实际财务事件（FBA费率 %.1f%% + 佣金率 %.1f%%），按 ASIN 销售额占比推算",
			accountFbaFee/accountSales*100, accountRefFee/accountSales*100)
	} else {
		info.DataNote = "暂无财务事件数据，请先同步财务信息"
	}

	return info, nil
}

// ── 环比计算工具函数 ─────────────────────────────────────

// calcPrevPeriod 根据当期日期范围推算等长的上期区间
// 例如：当期 2026-03-01 ~ 2026-03-31（31天）→ 上期 2026-01-29 ~ 2026-02-28
func calcPrevPeriod(dateStart, dateEnd string) (string, string) {
	layout := "2006-01-02"
	startT, err1 := time.Parse(layout, dateStart)
	endT, err2 := time.Parse(layout, dateEnd)
	if err1 != nil || err2 != nil {
		return "", ""
	}
	duration := endT.Sub(startT)
	prevEnd := startT.AddDate(0, 0, -1)              // 当期开始前一天
	prevStart := prevEnd.Add(-duration)               // 等长上期开始
	return prevStart.Format(layout), prevEnd.Format(layout)
}

// calcTrendPct 计算环比变化百分比
// 当上期为 0 且当期 > 0 时返回 100（表示新增）；都为 0 时返回 0
func calcTrendPct(current, prev float64) float64 {
	if prev == 0 {
		if current > 0 {
			return 100 // 从无到有
		}
		return 0
	}
	return (current - prev) / prev * 100
}

// ── 利润日历 ────────────────────────────────────────────

// DailyProfitCell 每日利润格子（用于日历热力图）
type DailyProfitCell struct {
	Date      string  `json:"date"`      // "2026-04-01"
	Sales     float64 `json:"sales"`     // 当日销售额
	AdSpend   float64 `json:"adSpend"`   // 当日广告费
	Fees      float64 `json:"fees"`      // 当日平台费（月均摊）
	COGS      float64 `json:"cogs"`      // 当日商品成本
	NetProfit float64 `json:"netProfit"` // 当日净利润
	Level     string  `json:"level"`     // "profit" / "low" / "loss"
}

// GetDailyProfitCalendar 获取指定月份每日利润数据
// yearMonth 格式：YYYY-MM（如 "2026-04"）
func (s *DashboardService) GetDailyProfitCalendar(accountID int64, marketplaceID, yearMonth string) ([]DailyProfitCell, error) {
	// 推算月份起止日期
	t, err := time.Parse("2006-01", yearMonth)
	if err != nil {
		return nil, fmt.Errorf("日期格式错误，需 YYYY-MM: %w", err)
	}
	startDate := t.Format("2006-01-02")
	endDate := t.AddDate(0, 1, -1).Format("2006-01-02")

	// 1. 每日销售额
	salesMap := map[string]float64{}
	unitsMap := map[string]int{}
	rows, err := s.db.Query(`
		SELECT date, COALESCE(ordered_product_sales, 0), COALESCE(units_ordered, 0)
		FROM sales_traffic_daily
		WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?
		ORDER BY date
	`, accountID, marketplaceID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		var sales float64
		var units int
		if err := rows.Scan(&d, &sales, &units); err != nil { slog.Warn("扫描利润日历销售行失败", "error", err); continue }
		salesMap[d] = sales
		unitsMap[d] = units
		dates = append(dates, d)
	}

	if len(dates) == 0 {
		return []DailyProfitCell{}, nil
	}

	// 2. 每日广告费
	adMap := map[string]float64{}
	adRows, err := s.db.Query(`
		SELECT apd.date, COALESCE(SUM(apd.cost), 0)
		FROM ad_performance_daily apd
		JOIN ad_campaigns ac ON apd.campaign_id=ac.campaign_id AND apd.account_id=ac.account_id
		WHERE apd.account_id=? AND ac.marketplace_id=? AND apd.date>=? AND apd.date<=?
		GROUP BY apd.date
	`, accountID, marketplaceID, startDate, endDate)
	if err == nil {
		defer adRows.Close()
		for adRows.Next() {
			var d string
			var cost float64
			if err := adRows.Scan(&d, &cost); err != nil { slog.Warn("扫描利润日历广告行失败", "error", err); continue }
			adMap[d] = cost
		}
	}

	// 3. 月度平台费用（均摊到每天）
	var monthFees float64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(marketplace_fee + fba_fee), 0) FROM financial_events
		WHERE account_id=? AND marketplace_id=? AND posted_date>=? AND posted_date<=?
	`, accountID, marketplaceID, startDate+" 00:00:00", endDate+" 23:59:59").Scan(&monthFees)
	daysInMonth := float64(t.AddDate(0, 1, -1).Day())
	dailyFees := monthFees / daysInMonth

	// 4. 单位成本查询（SKU 级 COGS）
	cogsMap := map[string]float64{}
	cogsRows, err := s.db.Query(`
		SELECT date(o.purchase_date) as d, COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
		FROM order_items oi
		JOIN orders o ON oi.amazon_order_id=o.amazon_order_id
		JOIN product_costs pc ON oi.seller_sku=pc.seller_sku AND pc.account_id=o.account_id
		WHERE o.account_id=? AND o.marketplace_id=?
		  AND date(o.purchase_date)>=? AND date(o.purchase_date)<=?
		  AND o.order_status NOT IN ('Canceled','Declined')
		GROUP BY d
	`, accountID, marketplaceID, startDate, endDate)
	if err == nil {
		defer cogsRows.Close()
		for cogsRows.Next() {
			var d string
			var cogs float64
			if err := cogsRows.Scan(&d, &cogs); err != nil { slog.Warn("扫描利润日历 COGS 行失败", "error", err); continue }
			cogsMap[d] = cogs
		}
	}

	// 5. 组装每日利润
	var result []DailyProfitCell
	for _, d := range dates {
		sales := salesMap[d]
		adSpend := adMap[d]
		cogs := cogsMap[d]
		netProfit := sales - adSpend - dailyFees - cogs

		level := "profit"
		if netProfit < 0 {
			level = "loss"
		} else if sales > 0 && netProfit/sales < 0.05 {
			level = "low"
		}

		result = append(result, DailyProfitCell{
			Date:      d,
			Sales:     sales,
			AdSpend:   adSpend,
			Fees:      dailyFees,
			COGS:      cogs,
			NetProfit: netProfit,
			Level:     level,
		})
	}
	return result, nil
}

// ── 利润率趋势 ────────────────────────────────────────────

// ProfitMarginPoint 每日利润率数据点（用于趋势图）
type ProfitMarginPoint struct {
	Date        string  `json:"date"`
	Sales       float64 `json:"sales"`
	NetProfit   float64 `json:"netProfit"`
	Margin      float64 `json:"margin"`      // 利润率百分比（0~100）
	AdSpend     float64 `json:"adSpend"`
	Fees        float64 `json:"fees"`
	COGS        float64 `json:"cogs"`
}

// GetProfitMarginTrend 获取每日利润率趋势数据（日粒度）
// 利润率 = (销售额 - 广告费 - 平台费 - COGS) / 销售额 × 100
// 数据来源：sales_traffic_daily + ad_performance_daily + financial_events + product_costs
func (s *DashboardService) GetProfitMarginTrend(accountID int64, marketplaceID, dateStart, dateEnd string) ([]ProfitMarginPoint, error) {
	// 查询每日销售额
	salesRows, err := s.db.Query(`
		SELECT date, COALESCE(ordered_product_sales, 0)
		FROM sales_traffic_daily
		WHERE account_id = ? AND marketplace_id = ?
		  AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, accountID, marketplaceID, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer salesRows.Close()

	salesMap := map[string]float64{}
	var dates []string
	for salesRows.Next() {
		var d string
		var s float64
		if err := salesRows.Scan(&d, &s); err != nil {
			slog.Warn("扫描利润率趋势销售行失败", "error", err)
			continue
		}
		salesMap[d] = s
		dates = append(dates, d)
	}
	if len(dates) == 0 {
		return []ProfitMarginPoint{}, nil
	}

	// 查询每日广告花费（广告数据可能不存在）
	adRows, adErr := s.db.Query(`
		SELECT apd.date, COALESCE(SUM(apd.cost), 0)
		FROM ad_performance_daily apd
		JOIN ad_campaigns ac ON apd.campaign_id = ac.campaign_id AND apd.account_id = ac.account_id
		WHERE apd.account_id = ? AND ac.marketplace_id = ?
		  AND apd.date >= ? AND apd.date <= ?
		GROUP BY apd.date
	`, accountID, marketplaceID, dateStart, dateEnd)
	if adErr == nil {
		defer adRows.Close()
		for adRows.Next() {
			var d string
			var cost float64
			if err := adRows.Scan(&d, &cost); err != nil {
				slog.Warn("扫描利润率趋势广告行失败", "error", err)
				continue
			}
			salesMap[d+"_ad"] = cost
		}
		if err := adRows.Err(); err != nil {
			slog.Warn("利润率趋势广告游标错误", "error", err)
		}
	}

	// 查询每日平均 COGS（通过当期 product_costs 均摊到每天）
	var totalCOGS float64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(pc.unit_cost * oi.quantity_ordered), 0)
		FROM order_items oi
		JOIN orders o ON oi.amazon_order_id = o.amazon_order_id
		JOIN product_costs pc ON oi.seller_sku = pc.seller_sku AND pc.account_id = o.account_id
		WHERE o.account_id = ? AND o.marketplace_id = ?
		  AND o.purchase_date >= ? AND o.purchase_date <= ?
		  AND o.order_status NOT IN ('Canceled','Declined')
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalCOGS)

	// 查询每日平均平台费（均摊）
	var totalFees float64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(marketplace_fee + fba_fee), 0)
		FROM financial_events
		WHERE account_id = ? AND marketplace_id = ?
		  AND posted_date >= ? AND posted_date <= ?
	`, accountID, marketplaceID, dateStart+" 00:00:00", dateEnd+" 23:59:59").Scan(&totalFees)

	// 均摊到天（按销售额比例分配）
	totalSales := 0.0
	for _, d := range dates {
		totalSales += salesMap[d]
	}

	var result []ProfitMarginPoint
	for _, d := range dates {
		sales := salesMap[d]
		adSpend := salesMap[d+"_ad"]

		// 按当日销售额比例分配 COGS 和费用
		ratio := 0.0
		if totalSales > 0 {
			ratio = sales / totalSales
		}
		dayCOGS := totalCOGS * ratio
		dayFees := totalFees * ratio

		netProfit := sales - adSpend - dayFees - dayCOGS
		margin := 0.0
		if sales > 0 {
			margin = netProfit / sales * 100
		}

		result = append(result, ProfitMarginPoint{
			Date:      d,
			Sales:     sales,
			NetProfit: netProfit,
			Margin:    margin,
			AdSpend:   adSpend,
			Fees:      dayFees,
			COGS:      dayCOGS,
		})
	}
	return result, nil
}

// ── ASIN 利润率排行 ───────────────────────────────────────

// AsinProfitRankItem ASIN 利润率排行条目
type AsinProfitRankItem struct {
	ASIN        string  `json:"asin"`
	Title       string  `json:"title"`
	SKU         string  `json:"sku"`
	TotalSales  float64 `json:"totalSales"`
	TotalCOGS   float64 `json:"totalCogs"`
	NetProfit   float64 `json:"netProfit"`
	Margin      float64 `json:"margin"`      // 利润率 %
	UnitsSold   int     `json:"unitsSold"`
	UnitCost    float64 `json:"unitCost"`
	// 数据延迟提示
	DataLatencyNote string `json:"dataLatencyNote"`
}

// GetAsinProfitRank 获取 ASIN 利润率排行（Top N 最赚钱 + Top N 最亏钱）
// 数据来源：sales_traffic_by_asin + product_costs（不含平台费均摊，聚焦 COGS 毛利）
func (s *DashboardService) GetAsinProfitRank(accountID int64, marketplaceID, dateStart, dateEnd string, topN int) (map[string][]AsinProfitRankItem, error) {
	if topN <= 0 {
		topN = 5
	}

	rows, err := s.db.Query(`
		SELECT
			t.asin,
			COALESCE(p.title, t.asin)      AS title,
			COALESCE(pc.seller_sku, '')    AS sku,
			COALESCE(SUM(t.ordered_product_sales), 0) AS total_sales,
			COALESCE(SUM(t.units_ordered), 0)         AS units_sold,
			COALESCE(pc.unit_cost, 0)                 AS unit_cost
		FROM sales_traffic_by_asin t
		LEFT JOIN products p
			ON t.asin = p.asin AND p.account_id = t.account_id AND p.marketplace_id = t.marketplace_id
		LEFT JOIN product_costs pc
			ON pc.account_id = t.account_id
			AND pc.seller_sku = (
				SELECT seller_sku FROM products
				WHERE asin = t.asin AND account_id = t.account_id AND marketplace_id = t.marketplace_id
				LIMIT 1
			)
		WHERE t.account_id = ? AND t.marketplace_id = ?
		  AND t.date >= ? AND t.date <= ?
		GROUP BY t.asin
		HAVING total_sales > 0
		ORDER BY total_sales DESC
		LIMIT 50
	`, accountID, marketplaceID, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AsinProfitRankItem
	for rows.Next() {
		var item AsinProfitRankItem
		if err := rows.Scan(
			&item.ASIN, &item.Title, &item.SKU,
			&item.TotalSales, &item.UnitsSold, &item.UnitCost,
		); err != nil {
			slog.Warn("扫描 ASIN 利润排行行失败", "error", err)
			continue
		}
		item.TotalCOGS = item.UnitCost * float64(item.UnitsSold)
		item.NetProfit = item.TotalSales - item.TotalCOGS
		if item.TotalSales > 0 {
			item.Margin = item.NetProfit / item.TotalSales * 100
		}
		item.DataLatencyNote = "Data Kiosk T+2，仅含 COGS 毛利（不含平台费/广告费）"
		items = append(items, item)
	}

	// 按毛利率降序排列
	sort.Slice(items, func(i, j int) bool {
		return items[i].Margin > items[j].Margin
	})

	// 取最佳 Top N
	best := make([]AsinProfitRankItem, 0, topN)
	for i := 0; i < len(items) && len(best) < topN; i++ {
		best = append(best, items[i])
	}

	// 取最差 Top N（从尾部取，排除已出现在 best 中的 ASIN，防止重叠）
	bestSet := make(map[string]bool, len(best))
	for _, b := range best {
		bestSet[b.ASIN] = true
	}
	worst := make([]AsinProfitRankItem, 0, topN)
	for i := len(items) - 1; i >= 0 && len(worst) < topN; i-- {
		if !bestSet[items[i].ASIN] {
			worst = append(worst, items[i])
		}
	}

	return map[string][]AsinProfitRankItem{
		"best":  best,
		"worst": worst,
	}, nil
}

