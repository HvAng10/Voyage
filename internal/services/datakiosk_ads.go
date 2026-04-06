// Package services - Data Kiosk + 广告数据同步服务
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"voyage/internal/amazon/advertising"
	"voyage/internal/amazon/datakiosk"
	"voyage/internal/database"
)

// DataKioskService Data Kiosk 数据同步服务
type DataKioskService struct {
	db     *database.DB
	client *datakiosk.Client
}

func NewDataKioskService(db *database.DB, client *datakiosk.Client) *DataKioskService {
	return &DataKioskService{db: db, client: client}
}

// SyncSalesTraffic 同步销售与流量数据（Analytics_SalesAndTraffic）
func (s *DataKioskService) SyncSalesTraffic(ctx context.Context, accountID int64, marketplaceID, dateStart, dateEnd string) (int, error) {
	slog.Info("Data Kiosk 同步销售与流量", "account", accountID, "range", dateStart+"~"+dateEnd)

	gqlQuery := datakiosk.SalesTrafficByDateQuery(dateStart, dateEnd, marketplaceID)
	queryResp, err := s.client.CreateQuery(ctx, gqlQuery)
	if err != nil {
		return 0, fmt.Errorf("提交 Data Kiosk 查询失败: %w", err)
	}

	docID, err := s.client.WaitForQuery(ctx, queryResp.QueryId, 2*time.Minute, 2*time.Hour)
	if err != nil {
		return 0, err
	}

	docResp, err := s.client.GetDocument(ctx, docID)
	if err != nil {
		return 0, err
	}

	count := 0
	err = datakiosk.DownloadAndParseJSONL(ctx, docResp.DocumentUrl, func(raw []byte) error {
		return s.parseSalesTrafficByDate(ctx, accountID, marketplaceID, raw, &count)
	})
	if err != nil {
		return count, fmt.Errorf("解析 Data Kiosk 数据失败: %w", err)
	}

	// 同步 ASIN 维度
	asinQuery := datakiosk.SalesTrafficByAsinQuery(dateStart, dateEnd, marketplaceID)
	if asinQR, err := s.client.CreateQuery(ctx, asinQuery); err == nil {
		if asinDocID, err := s.client.WaitForQuery(ctx, asinQR.QueryId, 2*time.Minute, 2*time.Hour); err == nil {
			if asinDoc, err := s.client.GetDocument(ctx, asinDocID); err == nil {
				datakiosk.DownloadAndParseJSONL(ctx, asinDoc.DocumentUrl, func(raw []byte) error {
					return s.parseSalesTrafficByAsin(ctx, accountID, marketplaceID, raw)
				})
			}
		}
	}

	slog.Info("Data Kiosk 同步完成", "records", count)
	return count, nil
}

type moneyField struct {
	Amount       float64 `json:"amount"`
	CurrencyCode string  `json:"currencyCode"`
}

type salesTrafficByDateRecord struct {
	StartDate                    string     `json:"startDate"`
	MarketplaceId                string     `json:"marketplaceId"`
	OrderedProductSales          moneyField `json:"orderedProductSales"`
	OrderedProductSalesB2B       moneyField `json:"orderedProductSalesB2B"`
	UnitsOrdered                 int        `json:"unitsOrdered"`
	UnitsOrderedB2B              int        `json:"unitsOrderedB2B"`
	TotalOrderItems              int        `json:"totalOrderItems"`
	TotalOrderItemsB2B           int        `json:"totalOrderItemsB2B"`
	PageViews                    int        `json:"pageViews"`
	PageViewsB2B                 int        `json:"pageViewsB2B"`
	Sessions                     int        `json:"sessions"`
	SessionsB2B                  int        `json:"sessionsB2B"`
	BrowserSessions              int        `json:"browserSessions"`
	MobileAppSessions            int        `json:"mobileAppSessions"`
	UnitSessionPercentage        float64    `json:"unitSessionPercentage"`
	OrderItemSessionPercentage   float64    `json:"orderItemSessionPercentage"`
	AverageOfferCount            float64    `json:"averageOfferCount"`
	FeedbackReceived             int        `json:"feedbackReceived"`
	NegativeFeedbackReceived     int        `json:"negativeFeedbackReceived"`
	ReceivedNegativeFeedbackRate float64    `json:"receivedNegativeFeedbackRate"`
}

type salesTrafficByAsinRecord struct {
	ChildAsin            string     `json:"childAsin"`
	ParentAsin           string     `json:"parentAsin"`
	OrderedProductSales  moneyField `json:"orderedProductSales"`
	UnitsOrdered         int        `json:"unitsOrdered"`
	TotalOrderItems      int        `json:"totalOrderItems"`
	PageViews            int        `json:"pageViews"`
	Sessions             int        `json:"sessions"`
	UnitSessionPercentage float64   `json:"unitSessionPercentage"`
	BuyBoxPercentage     float64    `json:"featuredOfferBuyBoxPercentage"`
}

func (s *DataKioskService) parseSalesTrafficByDate(ctx context.Context, accountID int64, mktID string, raw []byte, count *int) error {
	var wrapper struct {
		Analytics struct {
			ByDate []salesTrafficByDateRecord `json:"salesAndTrafficByDate"`
		} `json:"analytics_salesAndTraffic_2023_11_15"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Analytics.ByDate) > 0 {
		for _, rec := range wrapper.Analytics.ByDate {
			if s.upsertDailyRecord(ctx, accountID, mktID, rec) == nil {
				*count++
			}
		}
		return nil
	}
	var rec salesTrafficByDateRecord
	if err := json.Unmarshal(raw, &rec); err == nil && rec.StartDate != "" {
		if s.upsertDailyRecord(ctx, accountID, mktID, rec) == nil {
			*count++
		}
	}
	return nil
}

func (s *DataKioskService) upsertDailyRecord(ctx context.Context, accountID int64, mktID string, rec salesTrafficByDateRecord) error {
	if rec.StartDate == "" {
		return fmt.Errorf("空日期")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sales_traffic_daily (
			account_id, marketplace_id, date,
			ordered_product_sales, ordered_product_sales_b2b,
			units_ordered, units_ordered_b2b,
			total_order_items, total_order_items_b2b,
			page_views, page_views_b2b, sessions, sessions_b2b,
			browser_sessions, mobile_app_sessions,
			unit_session_percentage, order_item_session_percentage,
			average_offer_count,
			feedback_received, negative_feedback_received, received_negative_feedback_rate
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id,marketplace_id,date) DO UPDATE SET
			ordered_product_sales = excluded.ordered_product_sales,
			units_ordered         = excluded.units_ordered,
			page_views            = excluded.page_views,
			sessions              = excluded.sessions,
			unit_session_percentage = excluded.unit_session_percentage
	`,
		accountID, mktID, rec.StartDate,
		rec.OrderedProductSales.Amount, rec.OrderedProductSalesB2B.Amount,
		rec.UnitsOrdered, rec.UnitsOrderedB2B,
		rec.TotalOrderItems, rec.TotalOrderItemsB2B,
		rec.PageViews, rec.PageViewsB2B, rec.Sessions, rec.SessionsB2B,
		rec.BrowserSessions, rec.MobileAppSessions,
		rec.UnitSessionPercentage, rec.OrderItemSessionPercentage,
		rec.AverageOfferCount,
		rec.FeedbackReceived, rec.NegativeFeedbackReceived, rec.ReceivedNegativeFeedbackRate,
	)
	return err
}

func (s *DataKioskService) parseSalesTrafficByAsin(ctx context.Context, accountID int64, mktID string, raw []byte) error {
	var wrapper struct {
		Analytics struct {
			ByAsin []salesTrafficByAsinRecord `json:"salesAndTrafficByAsin"`
		} `json:"analytics_salesAndTraffic_2023_11_15"`
	}
	date := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")

	parse := func(r salesTrafficByAsinRecord) {
		asin := r.ChildAsin
		if asin == "" {
			asin = r.ParentAsin
		}
		if asin == "" {
			return
		}
		parentAsin := r.ParentAsin
		if parentAsin == asin {
			parentAsin = "" // 如果 child 和 parent 相同，则不是变体
		}
		s.db.ExecContext(ctx, `
			INSERT INTO sales_traffic_by_asin (
				account_id, marketplace_id, asin, date,
				ordered_product_sales, units_ordered, total_order_items,
				page_views, sessions, unit_session_percentage, buy_box_percentage,
				parent_asin
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(account_id,marketplace_id,asin,date) DO UPDATE SET
				ordered_product_sales   = excluded.ordered_product_sales,
				units_ordered           = excluded.units_ordered,
				buy_box_percentage      = excluded.buy_box_percentage,
				parent_asin             = excluded.parent_asin
		`, accountID, mktID, asin, date,
			r.OrderedProductSales.Amount, r.UnitsOrdered, r.TotalOrderItems,
			r.PageViews, r.Sessions, r.UnitSessionPercentage, r.BuyBoxPercentage,
			parentAsin,
		)
	}

	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Analytics.ByAsin) > 0 {
		for _, r := range wrapper.Analytics.ByAsin {
			parse(r)
		}
		return nil
	}
	var r salesTrafficByAsinRecord
	if err := json.Unmarshal(raw, &r); err == nil {
		parse(r)
	}
	return nil
}

// ── 广告同步服务 ─────────────────────────────────────────

// AdsService 广告数据同步服务
type AdsService struct {
	db     *database.DB
	client *advertising.Client
}

func NewAdsService(db *database.DB, client *advertising.Client) *AdsService {
	return &AdsService{db: db, client: client}
}

// SyncCampaigns 同步广告活动列表
func (s *AdsService) SyncCampaigns(ctx context.Context, accountID int64, mktID string) (int, error) {
	campaigns, err := s.client.GetCampaigns(ctx, "enabled,paused")
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for _, c := range campaigns {
		tx.ExecContext(ctx, `
			INSERT INTO ad_campaigns (campaign_id,account_id,ads_profile_id,marketplace_id,name,campaign_type,targeting_type,state,daily_budget,start_date,end_date,synced_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,datetime('now'))
			ON CONFLICT(campaign_id,account_id) DO UPDATE SET name=excluded.name,state=excluded.state,daily_budget=excluded.daily_budget,synced_at=datetime('now')
		`, c.CampaignId, accountID, "", mktID, c.Name, c.CampaignType, c.TargetingType, c.State, c.DailyBudget, c.StartDate, c.EndDate)
	}
	return len(campaigns), tx.Commit()
}

// SyncAdPerformance 同步广告效果数据（v3 异步报告）
func (s *AdsService) SyncAdPerformance(ctx context.Context, accountID int64, mktID, dateStart, dateEnd string) (int, error) {
	slog.Info("同步广告效果", "account", accountID, "range", dateStart+"~"+dateEnd)

	req := advertising.DefaultSPCampaignReportConfig(dateStart, dateEnd)
	reportResp, err := s.client.CreateReport(ctx, req)
	if err != nil {
		return 0, err
	}

	dlURL, err := s.client.WaitForReport(ctx, reportResp.ReportId, 1*time.Minute, 60*time.Minute)
	if err != nil {
		return 0, err
	}

	return s.parseAndStoreAdReport(ctx, accountID, dlURL)
}

type adReportRecord struct {
	CampaignId        string  `json:"campaignId"`
	Date              string  `json:"date"`
	Impressions       int     `json:"impressions"`
	Clicks            int     `json:"clicks"`
	Cost              float64 `json:"cost"`
	Purchases7d       int     `json:"purchases7d"`
	Purchases14d      int     `json:"purchases14d"`
	Purchases30d      int     `json:"purchases30d"`
	Sales7d           float64 `json:"sales7d"`
	Sales14d          float64 `json:"sales14d"`
	Sales30d          float64 `json:"sales30d"`
	UnitsSoldClicks7d int     `json:"unitsSoldClicks7d"`
}

func (s *AdsService) parseAndStoreAdReport(ctx context.Context, accountID int64, dlURL string) (int, error) {
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var records []adReportRecord
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

		_, err := tx.ExecContext(ctx, `
			INSERT INTO ad_performance_daily (
				campaign_id,account_id,date,impressions,clicks,cost,
				attributed_sales_7d,attributed_sales_14d,attributed_sales_30d,
				attributed_conversions_7d,attributed_conversions_14d,attributed_conversions_30d,
				attributed_units_ordered_7d,click_through_rate,cost_per_click,acos,roas
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(campaign_id,account_id,date) DO UPDATE SET
				impressions=excluded.impressions,clicks=excluded.clicks,cost=excluded.cost,
				attributed_sales_7d=excluded.attributed_sales_7d,acos=excluded.acos,roas=excluded.roas
		`, r.CampaignId, accountID, r.Date,
			r.Impressions, r.Clicks, r.Cost,
			r.Sales7d, r.Sales14d, r.Sales30d,
			r.Purchases7d, r.Purchases14d, r.Purchases30d,
			r.UnitsSoldClicks7d, ctr, cpc, acos, roas,
		)
		if err == nil {
			count++
		}
	}
	return count, tx.Commit()
}
