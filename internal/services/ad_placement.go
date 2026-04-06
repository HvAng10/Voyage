// Package services - 广告 Placement 报告同步与查询
// 数据来源：Advertising API v3 spCampaigns 报告（增加 placement 维度）
// 延迟：T+3
package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"voyage/internal/amazon/advertising"
	"voyage/internal/database"
)

// PlacementRow 版位效果单行
type PlacementRow struct {
	CampaignName string  `json:"campaignName"`
	CampaignID   string  `json:"campaignId"`
	Placement    string  `json:"placement"`     // Top of Search / Product Pages / Rest of Search
	Impressions  int     `json:"impressions"`
	Clicks       int     `json:"clicks"`
	Cost         float64 `json:"cost"`
	Sales7d      float64 `json:"sales7d"`
	Orders7d     int     `json:"orders7d"`
	ACoS         float64 `json:"acos"`
	CTR          float64 `json:"ctr"`
	CPC          float64 `json:"cpc"`
	CVR          float64 `json:"cvr"` // 转化率 = orders / clicks
}

// placementReportRecord 广告 Placement 报告原始记录
type placementReportRecord struct {
	Date         string  `json:"date"`
	CampaignID   string  `json:"campaignId"`
	CampaignName string  `json:"campaignName"`
	Placement    string  `json:"placement"`
	Impressions  int     `json:"impressions"`
	Clicks       int     `json:"clicks"`
	Cost         float64 `json:"cost"`
	Sales7d      float64 `json:"sales7d"`
	Orders7d     int     `json:"purchases7d"`
}

// SPPlacementReportConfig 创建 SP 版位日报配置
func SPPlacementReportConfig(startDate, endDate string) advertising.CreateReportRequest {
	return advertising.CreateReportRequest{
		Name:      "SP版位日报-" + startDate + "-" + endDate,
		StartDate: startDate,
		EndDate:   endDate,
		Configuration: advertising.ReportConfiguration{
			AdProduct:    "SPONSORED_PRODUCTS",
			GroupBy:      []string{"campaign", "date", "placement"},
			ReportTypeId: "spCampaigns",
			TimeUnit:     "DAILY",
			Format:       "GZIP_JSON",
			Columns: []string{
				"date", "campaignId", "campaignName", "placement",
				"impressions", "clicks", "cost",
				"sales7d", "purchases7d",
			},
		},
	}
}

// SyncAdPlacementReport 同步广告版位数据
// 流程：创建报告 → 轮询状态 → 下载解析 → 写入 ad_placement_daily
func SyncAdPlacementReport(ctx context.Context, db *database.DB, adsClient *advertising.Client,
	accountID int64, startDate, endDate string) (int, error) {

	slog.Info("开始同步广告版位报告", "account", accountID, "range", startDate+"~"+endDate)

	cfg := SPPlacementReportConfig(startDate, endDate)
	reportResp, err := adsClient.CreateReport(ctx, cfg)
	if err != nil {
		return 0, err
	}

	// 轮询报告完成
	downloadURL, err := adsClient.WaitForReport(ctx, reportResp.ReportId, 1*time.Minute, 60*time.Minute)
	if err != nil {
		return 0, err
	}

	// 通过 HTTP GET 下载报告（与 SyncAdPerformance 相同模式）
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// 解析 JSON 格式
	var records []placementReportRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return 0, err
	}

	// 写入数据库
	count := 0
	for _, r := range records {
		if r.CampaignID == "" || r.Placement == "" {
			continue
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO ad_placement_daily (
				campaign_id, account_id, date, placement,
				impressions, clicks, cost, sales_7d, orders_7d
			) VALUES (?,?,?,?,?,?,?,?,?)
			ON CONFLICT(campaign_id, account_id, date, placement) DO UPDATE SET
				impressions = excluded.impressions,
				clicks = excluded.clicks,
				cost = excluded.cost,
				sales_7d = excluded.sales_7d,
				orders_7d = excluded.orders_7d
		`, r.CampaignID, accountID, r.Date, r.Placement,
			r.Impressions, r.Clicks, r.Cost, r.Sales7d, r.Orders7d)
		if err != nil {
			slog.Warn("版位数据写入失败", "campaignId", r.CampaignID, "error", err)
			continue
		}
		count++
	}

	slog.Info("广告版位报告同步完成", "account", accountID, "records", count)
	return count, nil
}


// GetAdPlacementStats 查询版位效果汇总
// 返回按 placement 分组的聚合数据
func GetAdPlacementStats(db *database.DB, accountID int64, dateStart, dateEnd string) ([]PlacementRow, error) {
	rows, err := db.Query(`
		SELECT
			COALESCE(ac.name, p.campaign_id) AS campaign_name,
			p.campaign_id,
			p.placement,
			COALESCE(SUM(p.impressions), 0),
			COALESCE(SUM(p.clicks), 0),
			COALESCE(SUM(p.cost), 0),
			COALESCE(SUM(p.sales_7d), 0),
			COALESCE(SUM(p.orders_7d), 0)
		FROM ad_placement_daily p
		LEFT JOIN ad_campaigns ac ON p.campaign_id = ac.campaign_id AND p.account_id = ac.account_id
		WHERE p.account_id = ? AND p.date >= ? AND p.date <= ?
		GROUP BY p.campaign_id, p.placement
		ORDER BY SUM(p.cost) DESC
	`, accountID, dateStart, dateEnd)
	if err != nil {
		return []PlacementRow{}, nil
	}
	defer rows.Close()

	var result []PlacementRow
	for rows.Next() {
		var r PlacementRow
		if err := rows.Scan(
			&r.CampaignName, &r.CampaignID, &r.Placement,
			&r.Impressions, &r.Clicks, &r.Cost, &r.Sales7d, &r.Orders7d,
		); err != nil {
			slog.Warn("版位数据行解析失败", "error", err)
			continue
		}
		// 计算衍生指标
		if r.Sales7d > 0 {
			r.ACoS = r.Cost / r.Sales7d * 100
		}
		if r.Impressions > 0 {
			r.CTR = float64(r.Clicks) / float64(r.Impressions) * 100
		}
		if r.Clicks > 0 {
			r.CPC = r.Cost / float64(r.Clicks)
			r.CVR = float64(r.Orders7d) / float64(r.Clicks) * 100
		}
		result = append(result, r)
	}
	if result == nil {
		result = []PlacementRow{}
	}
	return result, nil
}

// GetPlacementSummary 按版位类型汇总（不区分活动）
func GetPlacementSummary(db *database.DB, accountID int64, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT
			p.placement,
			COALESCE(SUM(p.impressions), 0),
			COALESCE(SUM(p.clicks), 0),
			COALESCE(SUM(p.cost), 0),
			COALESCE(SUM(p.sales_7d), 0),
			COALESCE(SUM(p.orders_7d), 0)
		FROM ad_placement_daily p
		WHERE p.account_id = ? AND p.date >= ? AND p.date <= ?
		GROUP BY p.placement
		ORDER BY SUM(p.cost) DESC
	`, accountID, dateStart, dateEnd)
	if err != nil {
		return []map[string]interface{}{}, nil
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var placement string
		var imps, clicks, orders int
		var cost, sales float64
		if err := rows.Scan(&placement, &imps, &clicks, &cost, &sales, &orders); err != nil {
			continue
		}
		item := map[string]interface{}{
			"placement":   placement,
			"impressions": imps,
			"clicks":      clicks,
			"cost":        cost,
			"sales":       sales,
			"orders":      orders,
		}
		if sales > 0 {
			item["acos"] = cost / sales * 100
		}
		if imps > 0 {
			item["ctr"] = float64(clicks) / float64(imps) * 100
		}
		if clicks > 0 {
			item["cpc"] = cost / float64(clicks)
			item["cvr"] = float64(orders) / float64(clicks) * 100
		}
		result = append(result, item)
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return result, nil
}
