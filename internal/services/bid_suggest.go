// Package services - 广告竞价建议引擎
// 基于历史广告投放数据计算最优竞价建议
// 算法：目标 ACoS × 转化率 × ASP = 建议 CPC 上限
package services

import (
	"fmt"
	"math"

	"voyage/internal/database"
)

// BidSuggestion 竞价建议结果
type BidSuggestion struct {
	CampaignName string  `json:"campaignName"`
	AdGroupName  string  `json:"adGroupName"`
	KeywordText  string  `json:"keywordText"`
	MatchType    string  `json:"matchType"`

	// 历史表现
	HistImpressions int     `json:"histImpressions"`
	HistClicks      int     `json:"histClicks"`
	HistCost        float64 `json:"histCost"`
	HistSales       float64 `json:"histSales"`
	HistOrders      int     `json:"histOrders"`
	HistACoS        float64 `json:"histAcos"`   // 实际 ACoS（%）
	HistCTR         float64 `json:"histCtr"`    // 实际 CTR（%）
	HistCPC         float64 `json:"histCpc"`    // 实际 CPC
	HistCVR         float64 `json:"histCvr"`    // 转化率（%）

	// 建议竞价
	SuggestedBid   float64 `json:"suggestedBid"`   // 建议竞价（$）
	CurrentBid     float64 `json:"currentBid"`      // 当前出价
	BidDelta       float64 `json:"bidDelta"`         // 差值 = suggested - current
	Confidence     string  `json:"confidence"`       // high / medium / low
	Reason         string  `json:"reason"`           // 建议原因

	// 参考指标
	TargetACoS     float64 `json:"targetAcos"`       // 目标 ACoS（%）
	AvgSalePrice   float64 `json:"avgSalePrice"`     // 平均客单价
}

// GetBidSuggestions 获取广告出价建议
// targetACoS: 目标 ACoS（如 25 表示 25%），0 表示使用默认值 25%
func GetBidSuggestions(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string, targetACoS float64) ([]BidSuggestion, error) {
	if targetACoS <= 0 { targetACoS = 25.0 }

	// 查询有一定数据量的关键词（至少 10 次展示）
	rows, err := db.Query(`
		SELECT
			ac.name AS campaign_name,
			ag.name AS ad_group_name,
			ak.keyword_text,
			ak.match_type,
			ak.bid,
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
		WHERE ak.account_id = ? AND ac.marketplace_id = ? AND ak.state = 'enabled'
		GROUP BY ak.keyword_id
		HAVING SUM(kp.impressions) >= 10
		ORDER BY SUM(kp.cost) DESC
		LIMIT 100
	`, dateStart, dateEnd, accountID, marketplaceID)
	if err != nil {
		return []BidSuggestion{}, nil
	}
	defer rows.Close()

	var results []BidSuggestion
	for rows.Next() {
		var s BidSuggestion
		if err := rows.Scan(
			&s.CampaignName, &s.AdGroupName, &s.KeywordText, &s.MatchType,
			&s.CurrentBid, &s.HistImpressions, &s.HistClicks, &s.HistCost,
			&s.HistSales, &s.HistOrders,
		); err != nil {
			continue
		}

		s.TargetACoS = targetACoS

		// 计算历史指标
		if s.HistClicks > 0 {
			s.HistCPC = s.HistCost / float64(s.HistClicks)
		}
		if s.HistImpressions > 0 {
			s.HistCTR = float64(s.HistClicks) / float64(s.HistImpressions) * 100
		}
		if s.HistSales > 0 {
			s.HistACoS = s.HistCost / s.HistSales * 100
		}
		if s.HistClicks > 0 {
			s.HistCVR = float64(s.HistOrders) / float64(s.HistClicks) * 100
		}

		// 计算平均客单价 ASP
		if s.HistOrders > 0 {
			s.AvgSalePrice = s.HistSales / float64(s.HistOrders)
		}

		// ── 竞价建议算法 ──
		// 公式：SuggestedBid = TargetACoS% × CVR% × ASP
		// 含义：在目标 ACoS 下，每次点击最多可支付的金额
		if s.HistCVR > 0 && s.AvgSalePrice > 0 {
			cvr := s.HistCVR / 100
			targetAcosRatio := targetACoS / 100
			s.SuggestedBid = targetAcosRatio * cvr * s.AvgSalePrice

			// 四舍五入到分
			s.SuggestedBid = math.Round(s.SuggestedBid*100) / 100
		} else if s.HistClicks > 0 && s.HistSales == 0 {
			// 有点击无转化 → 建议降低出价到当前 50%
			s.SuggestedBid = math.Round(s.CurrentBid*0.5*100) / 100
		} else {
			// 数据不足，保持当前出价
			s.SuggestedBid = s.CurrentBid
		}

		// 最低保底 $0.02
		if s.SuggestedBid < 0.02 { s.SuggestedBid = 0.02 }

		s.BidDelta = s.SuggestedBid - s.CurrentBid

		// 置信度评估
		switch {
		case s.HistClicks >= 50 && s.HistOrders >= 5:
			s.Confidence = "high"
		case s.HistClicks >= 20 && s.HistOrders >= 2:
			s.Confidence = "medium"
		default:
			s.Confidence = "low"
		}

		// 建议原因
		switch {
		case s.HistSales == 0 && s.HistClicks >= 20:
			s.Reason = "有大量点击但无转化，建议大幅降低出价或暂停"
		case s.HistACoS > targetACoS*1.5:
			s.Reason = fmt.Sprintf("ACoS (%.0f%%) 远超目标 (%.0f%%)，建议降低出价", s.HistACoS, targetACoS)
		case s.HistACoS > targetACoS:
			s.Reason = fmt.Sprintf("ACoS (%.0f%%) 高于目标 (%.0f%%)，适当降价优化", s.HistACoS, targetACoS)
		case s.HistACoS > 0 && s.HistACoS < targetACoS*0.6:
			s.Reason = fmt.Sprintf("ACoS (%.0f%%) 表现优秀，可适当提高竞价争取更多流量", s.HistACoS)
		case s.HistACoS > 0:
			s.Reason = "ACoS 在目标范围内，维持当前策略"
		default:
			s.Reason = "数据不足，建议积累更多投放数据后调整"
		}

		results = append(results, s)
	}

	return results, nil
}
