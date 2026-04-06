// Package services - 广告关键词数据查询
// 扩展 app.go 中的广告分析，支持关键词/广告组钻取
package services

import (
	"log/slog"

	"voyage/internal/database"
)

// AdKeywordRow 关键词广告绩效
type AdKeywordRow struct {
	CampaignName string  `json:"campaignName"`
	AdGroupName  string  `json:"adGroupName"`
	KeywordText  string  `json:"keywordText"`
	MatchType    string  `json:"matchType"`
	State        string  `json:"state"`
	Impressions  int     `json:"impressions"`
	Clicks       int     `json:"clicks"`
	Cost         float64 `json:"cost"`
	Sales        float64 `json:"sales"`
	Orders       int     `json:"orders"`
	ACoS         float64 `json:"acos"`
	CTR          float64 `json:"ctr"`
	CPC          float64 `json:"cpc"`
}

// GetAdKeywords 查询关键词级别广告绩效
// dateStart / dateEnd: YYYY-MM-DD
func GetAdKeywords(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]AdKeywordRow, error) {
	rows, err := db.Query(`
		SELECT
			ac.name        AS campaign_name,
			ag.name        AS ad_group_name,
			ak.keyword_text,
			ak.match_type,
			ak.state,
			COALESCE(SUM(kp.impressions), 0),
			COALESCE(SUM(kp.clicks), 0),
			COALESCE(SUM(kp.cost), 0),
			COALESCE(SUM(kp.attributed_sales_7d), 0),
			COALESCE(SUM(kp.attributed_orders_7d), 0)
		FROM ad_keywords ak
		JOIN ad_groups ag ON ak.ad_group_id = ag.ad_group_id AND ak.account_id = ag.account_id
		JOIN ad_campaigns ac ON ag.campaign_id = ac.campaign_id AND ag.account_id = ac.account_id
		LEFT JOIN ad_keyword_performance kp ON ak.keyword_id = kp.keyword_id
			AND kp.account_id = ak.account_id
			AND kp.date >= ? AND kp.date <= ?
		WHERE ak.account_id = ? AND ac.marketplace_id = ?
		GROUP BY ak.keyword_id
		ORDER BY SUM(kp.cost) DESC
		LIMIT ?
	`, dateStart, dateEnd, accountID, marketplaceID, limit)
	if err != nil {
		// 表可能不存在或尚无数据，返回空数组而非错误
		return []AdKeywordRow{}, nil
	}
	defer rows.Close()

	var result []AdKeywordRow
	for rows.Next() {
		var r AdKeywordRow
		if err := rows.Scan(
			&r.CampaignName, &r.AdGroupName, &r.KeywordText, &r.MatchType, &r.State,
			&r.Impressions, &r.Clicks, &r.Cost, &r.Sales, &r.Orders,
		); err != nil {
			return nil, err
		}
		// 计算衍生指标
		if r.Sales > 0 { r.ACoS = r.Cost / r.Sales * 100 }
		if r.Impressions > 0 { r.CTR = float64(r.Clicks) / float64(r.Impressions) * 100 }
		if r.Clicks > 0 { r.CPC = r.Cost / float64(r.Clicks) }
		result = append(result, r)
	}
	if result == nil {
		result = []AdKeywordRow{}
	}
	return result, nil
}

// AdTargetRow ASIN 定向广告绩效
type AdTargetRow struct {
	CampaignName string  `json:"campaignName"`
	TargetType   string  `json:"targetType"` // ASIN / Category
	TargetValue  string  `json:"targetValue"`
	State        string  `json:"state"`
	Impressions  int     `json:"impressions"`
	Clicks       int     `json:"clicks"`
	Cost         float64 `json:"cost"`
	Sales        float64 `json:"sales"`
	ACoS         float64 `json:"acos"`
}

// GetAdTargets 查询 ASIN 商品投放定向数据
func GetAdTargets(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]AdTargetRow, error) {
	rows, err := db.Query(`
		SELECT
			ac.name,
			at.target_type,
			at.target_value,
			at.state,
			COALESCE(SUM(tp.impressions), 0),
			COALESCE(SUM(tp.clicks), 0),
			COALESCE(SUM(tp.cost), 0),
			COALESCE(SUM(tp.attributed_sales_7d), 0)
		FROM ad_targets at
		JOIN ad_groups ag ON at.ad_group_id = ag.ad_group_id AND at.account_id = ag.account_id
		JOIN ad_campaigns ac ON ag.campaign_id = ac.campaign_id AND ag.account_id = ac.account_id
		LEFT JOIN ad_target_performance tp ON at.target_id = tp.target_id
			AND tp.account_id = at.account_id
			AND tp.date >= ? AND tp.date <= ?
		WHERE at.account_id = ? AND ac.marketplace_id = ?
		GROUP BY at.target_id
		ORDER BY SUM(tp.cost) DESC
		LIMIT ?
	`, dateStart, dateEnd, accountID, marketplaceID, limit)
	if err != nil {
		// 表可能不存在，返回空数据而不是错误
		return []AdTargetRow{}, nil
	}
	defer rows.Close()

	var result []AdTargetRow
	for rows.Next() {
		var r AdTargetRow
		if err := rows.Scan(&r.CampaignName, &r.TargetType, &r.TargetValue, &r.State,
			&r.Impressions, &r.Clicks, &r.Cost, &r.Sales); err != nil {
			slog.Warn("扫描广告定向行失败", "error", err)
			continue
		}
		if r.Sales > 0 { r.ACoS = r.Cost / r.Sales * 100 }
		result = append(result, r)
	}
	if result == nil {
		result = []AdTargetRow{}
	}
	return result, nil
}

// GetAdCampaignDailyTrend 单个活动的每日花费/ACoS 趋势（用于图表钻取）
func GetAdCampaignDailyTrend(db *database.DB, accountID int64, campaignID, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT date,
			COALESCE(SUM(cost), 0),
			COALESCE(SUM(impressions), 0),
			COALESCE(SUM(clicks), 0),
			COALESCE(SUM(attributed_sales_7d), 0),
			CASE WHEN SUM(attributed_sales_7d) > 0
				THEN SUM(cost) / SUM(attributed_sales_7d) * 100
				ELSE 0
			END
		FROM ad_performance_daily
		WHERE account_id = ? AND campaign_id = ? AND date >= ? AND date <= ?
		GROUP BY date ORDER BY date
	`, accountID, campaignID, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var date string
		var cost, sales, acos float64
		var impressions, clicks int
		if err := rows.Scan(&date, &cost, &impressions, &clicks, &sales, &acos); err != nil { slog.Warn("扫描广告趋势行失败", "error", err); continue }
		result = append(result, map[string]interface{}{
			"date": date, "cost": cost, "impressions": impressions,
			"clicks": clicks, "sales": sales, "acos": acos,
		})
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return result, nil
}
