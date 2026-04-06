// Package services - SP 搜索词报告同步（Search Term Report）与 SB/SD 广告扩展
// 数据延迟：搜索词数据约 T+3 可用（Amazon 广告 API 说明）
package services

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"voyage/internal/amazon/advertising"
	"voyage/internal/database"
)

// ── SP 搜索词报告 ────────────────────────────────────────

// SearchTermReportService SP 搜索词报告同步服务
type SearchTermReportService struct {
	db     *database.DB
	client *advertising.Client
}

func NewSearchTermReportService(db *database.DB, client *advertising.Client) *SearchTermReportService {
	return &SearchTermReportService{db: db, client: client}
}

// spSearchTermRecord SP 搜索词报告单条记录
type spSearchTermRecord struct {
	Date        string  `json:"date"`
	CampaignId  string  `json:"campaignId"`
	AdGroupId   string  `json:"adGroupId"`
	KeywordId   string  `json:"keywordId"`
	KeywordText string  `json:"keywordText"`
	SearchTerm  string  `json:"searchTerm"`
	MatchType   string  `json:"matchType"`
	Impressions int     `json:"impressions"`
	Clicks      int     `json:"clicks"`
	Cost        float64 `json:"cost"`
	Purchases7d int     `json:"purchases7d"`
	Sales7d     float64 `json:"sales7d"`
}

// SyncSearchTermReport 同步 SP 搜索词报告
// 注意：数据延迟约 T+3，参数 dateEnd 建议为 3 天前
func (s *SearchTermReportService) SyncSearchTermReport(ctx context.Context, accountID int64, marketplaceID, dateStart, dateEnd string) (int, error) {
	slog.Info("开始同步 SP 搜索词报告", "account", accountID, "range", dateStart+"~"+dateEnd)

	logID := logSyncStart(s.db, accountID, marketplaceID, "ad_search_terms")

	req := advertising.SPSearchTermReportConfig(dateStart, dateEnd)
	reportResp, err := s.client.CreateReport(ctx, req)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, fmt.Errorf("创建搜索词报告失败: %w", err)
	}

	dlURL, err := s.client.WaitForReport(ctx, reportResp.ReportId, 1*time.Minute, 60*time.Minute)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	count, err := s.downloadAndStoreSearchTerms(ctx, accountID, dlURL)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}

	logSyncEnd(s.db, logID, "success", count, "")
	slog.Info("搜索词报告同步完成", "account", accountID, "records", count)
	return count, nil
}

func (s *SearchTermReportService) downloadAndStoreSearchTerms(ctx context.Context, accountID int64, dlURL string) (int, error) {
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" || resp.Header.Get("Content-Type") == "application/x-gzip" {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return 0, gzErr
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var records []spSearchTermRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return 0, fmt.Errorf("解析搜索词报告失败: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0
	for _, r := range records {
		if r.SearchTerm == "" {
			continue
		}

		var ctr, cvr, cpc, acos float64
		if r.Impressions > 0 {
			ctr = float64(r.Clicks) / float64(r.Impressions) * 100
		}
		if r.Clicks > 0 {
			cpc = r.Cost / float64(r.Clicks)
			cvr = float64(r.Purchases7d) / float64(r.Clicks) * 100
		}
		if r.Sales7d > 0 {
			acos = r.Cost / r.Sales7d * 100
		}

		_, execErr := tx.ExecContext(ctx, `
			INSERT INTO ad_search_terms (
				account_id, date, campaign_id, ad_group_id, keyword_id, keyword_text,
				search_term, match_type, impressions, clicks, cost,
				purchases_7d, sales_7d, click_through_rate, cost_per_click, conversion_rate, acos
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(account_id, date, campaign_id, search_term, keyword_id) DO UPDATE SET
				impressions = excluded.impressions,
				clicks = excluded.clicks,
				cost = excluded.cost,
				purchases_7d = excluded.purchases_7d,
				sales_7d = excluded.sales_7d,
				acos = excluded.acos
		`, accountID, r.Date, r.CampaignId, r.AdGroupId, r.KeywordId, r.KeywordText,
			r.SearchTerm, r.MatchType, r.Impressions, r.Clicks, r.Cost,
			r.Purchases7d, r.Sales7d, ctr, cpc, cvr, acos,
		)
		if execErr == nil {
			count++
		}
	}

	return count, tx.Commit()
}

// ── SB / SD 广告扩展 ─────────────────────────────────────

// SyncSBPerformance 同步 Sponsored Brands 广告效果
// 数据延迟：T+3
func (s *SearchTermReportService) SyncSBPerformance(ctx context.Context, accountID int64, marketplaceID, dateStart, dateEnd string) (int, error) {
	slog.Info("开始同步 SB 广告效果", "account", accountID)
	logID := logSyncStart(s.db, accountID, marketplaceID, "ad_sb_performance")

	req := advertising.SBCampaignReportConfig(dateStart, dateEnd)
	reportResp, err := s.client.CreateReport(ctx, req)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	dlURL, err := s.client.WaitForReport(ctx, reportResp.ReportId, 1*time.Minute, 60*time.Minute)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	count, err := s.storeGenericAdReport(ctx, accountID, dlURL)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}
	logSyncEnd(s.db, logID, "success", count, "")
	return count, nil
}

// SyncSDPerformance 同步 Sponsored Display 广告效果
// 数据延迟：T+3
func (s *SearchTermReportService) SyncSDPerformance(ctx context.Context, accountID int64, marketplaceID, dateStart, dateEnd string) (int, error) {
	slog.Info("开始同步 SD 广告效果", "account", accountID)
	logID := logSyncStart(s.db, accountID, marketplaceID, "ad_sd_performance")

	req := advertising.SDCampaignReportConfig(dateStart, dateEnd)
	reportResp, err := s.client.CreateReport(ctx, req)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	dlURL, err := s.client.WaitForReport(ctx, reportResp.ReportId, 1*time.Minute, 60*time.Minute)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}

	count, err := s.storeGenericAdReport(ctx, accountID, dlURL)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", count, err.Error())
		return count, err
	}
	logSyncEnd(s.db, logID, "success", count, "")
	return count, nil
}

// storeGenericAdReport SB/SD 报告存入 ad_performance_daily（通过活动表 campaign_type 区别）
func (s *SearchTermReportService) storeGenericAdReport(ctx context.Context, accountID int64, dlURL string) (int, error) {
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return 0, gzErr
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	// SB/SD 与 SP 报告基础字段一致
	var records []spSearchTermRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0
	for _, r := range records {
		if r.CampaignId == "" {
			continue
		}
		var cpc, acos, roas, ctr float64
		if r.Clicks > 0 {
			cpc = r.Cost / float64(r.Clicks)
		}
		if r.Sales7d > 0 {
			acos = r.Cost / r.Sales7d * 100
			roas = r.Sales7d / r.Cost
		}
		if r.Impressions > 0 {
			ctr = float64(r.Clicks) / float64(r.Impressions) * 100
		}

		_, execErr := tx.ExecContext(ctx, `
			INSERT INTO ad_performance_daily (
				campaign_id, account_id, date, impressions, clicks, cost,
				attributed_sales_7d, attributed_conversions_7d,
				click_through_rate, cost_per_click, acos, roas
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(campaign_id, account_id, date) DO UPDATE SET
				impressions = impressions + excluded.impressions,
				clicks = clicks + excluded.clicks,
				cost = cost + excluded.cost,
				attributed_sales_7d = attributed_sales_7d + excluded.attributed_sales_7d,
				acos = excluded.acos,
				roas = excluded.roas
		`, r.CampaignId, accountID, r.Date,
			r.Impressions, r.Clicks, r.Cost, r.Sales7d, r.Purchases7d,
			ctr, cpc, acos, roas,
		)
		if execErr == nil {
			count++
		}
	}

	return count, tx.Commit()
}

// ── 搜索词数据查询 ────────────────────────────────────────

// SearchTermStat 搜索词效果统计
type SearchTermStat struct {
	SearchTerm      string  `json:"searchTerm"`
	KeywordText     string  `json:"keywordText"`
	MatchType       string  `json:"matchType"`
	Impressions     int     `json:"impressions"`
	Clicks          int     `json:"clicks"`
	Cost            float64 `json:"cost"`
	Sales7d         float64 `json:"sales7d"`
	Purchases7d     int     `json:"purchases7d"`
	CTR             float64 `json:"ctr"`
	CPC             float64 `json:"cpc"`
	CVR             float64 `json:"cvr"`
	ACoS            float64 `json:"acos"`
	// 优化建议标签
	OptTag          string  `json:"optTag"` // negate / exact / ok
	// 数据延迟标注
	DataLatencyNote string  `json:"dataLatencyNote"`
}

// GetSearchTermStats 查询搜索词效果（聚合指定日期段）
func GetSearchTermStats(db *database.DB, accountID int64, dateStart, dateEnd string) ([]SearchTermStat, error) {
	rows, err := db.Query(`
		SELECT
			search_term,
			MAX(keyword_text) as keyword_text,
			MAX(match_type) as match_type,
			SUM(impressions) as total_imp,
			SUM(clicks) as total_clicks,
			SUM(cost) as total_cost,
			SUM(sales_7d) as total_sales,
			SUM(purchases_7d) as total_purchases
		FROM ad_search_terms
		WHERE account_id = ? AND date >= ? AND date <= ?
		GROUP BY search_term
		ORDER BY total_cost DESC
		LIMIT 200
	`, accountID, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	latencyNote := "搜索词数据延迟约 T+3（广告 API 固有延迟），近 3 天内数据可能尚不完整"

	var result []SearchTermStat
	for rows.Next() {
		var stat SearchTermStat
		if err := rows.Scan(&stat.SearchTerm, &stat.KeywordText, &stat.MatchType,
			&stat.Impressions, &stat.Clicks, &stat.Cost, &stat.Sales7d, &stat.Purchases7d); err != nil {
			continue
		}

		if stat.Impressions > 0 {
			stat.CTR = float64(stat.Clicks) / float64(stat.Impressions) * 100
		}
		if stat.Clicks > 0 {
			stat.CPC = stat.Cost / float64(stat.Clicks)
			stat.CVR = float64(stat.Purchases7d) / float64(stat.Clicks) * 100
		}
		if stat.Sales7d > 0 {
			stat.ACoS = stat.Cost / stat.Sales7d * 100
		}

		// 自动打优化建议标签
		switch {
		case stat.Clicks > 5 && stat.Purchases7d == 0:
			stat.OptTag = "negate" // 高点击无转化：建议否词
		case stat.Purchases7d >= 2 && stat.MatchType == "BROAD":
			stat.OptTag = "exact" // 宽泛匹配高转化：建议加精准词
		default:
			stat.OptTag = "ok"
		}

		stat.DataLatencyNote = latencyNote
		result = append(result, stat)
	}
	return result, nil
}
