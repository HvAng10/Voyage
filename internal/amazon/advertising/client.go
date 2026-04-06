// Package advertising 实现 Amazon Advertising API v3
// 官方文档：https://advertising.amazon.com/API/docs/en-us/guides/get-started/overview
// 报告端点：POST https://advertising-api.amazon.com/reporting/reports
package advertising

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"voyage/internal/amazon/auth"
)

// Advertising API 区域端点（官方）
// 来源：https://advertising.amazon.com/API/docs/en-us/guides/get-started/endpoints
const (
	AdsEndpointNA = "https://advertising-api.amazon.com"
	AdsEndpointEU = "https://advertising-api-eu.amazon.com"
	AdsEndpointFE = "https://advertising-api-fe.amazon.com"
)

// AdsRegionEndpoints 区域到广告 API 端点映射
var AdsRegionEndpoints = map[string]string{
	"na": AdsEndpointNA,
	"eu": AdsEndpointEU,
	"fe": AdsEndpointFE,
}

// Client Advertising API 客户端
type Client struct {
	tokenManager *auth.LWATokenManager
	httpClient   *http.Client
	baseURL      string
	clientID     string
	profileID    string // Amazon Advertising Profile ID
	accountID    string // Advertiser Account ID
}

// NewClient 创建广告 API 客户端
// profileID: 从 https://advertising-api.amazon.com/v2/profiles 获取
func NewClient(tokenManager *auth.LWATokenManager, region, clientID, profileID, accountID string) *Client {
	return &Client{
		tokenManager: tokenManager,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		baseURL:      AdsRegionEndpoints[region],
		clientID:     clientID,
		profileID:    profileID,
		accountID:    accountID,
	}
}

// Profile 广告账户配置文件
type Profile struct {
	ProfileId   int64  `json:"profileId"`
	CountryCode string `json:"countryCode"`
	CurrencyCode string `json:"currencyCode"`
	Timezone    string `json:"timezone"`
	AccountInfo struct {
		MarketplaceStringId string `json:"marketplaceStringId"`
		Id                  string `json:"id"`
		Type                string `json:"type"`
		Name                string `json:"name"`
	} `json:"accountInfo"`
}

// Campaign 广告活动（Sponsored Products）
type Campaign struct {
	CampaignId    string  `json:"campaignId"`
	Name          string  `json:"name"`
	CampaignType  string  `json:"campaignType"`
	TargetingType string  `json:"targetingType"`
	State         string  `json:"state"`
	DailyBudget   float64 `json:"dailyBudget"`
	StartDate     string  `json:"startDate"`
	EndDate       string  `json:"endDate,omitempty"`
}

// ============================================================
// Advertising API v3 异步报告
// 官方文档：https://advertising.amazon.com/API/docs/en-us/guides/reporting/v3/report-types
// ============================================================

// CreateReportRequest v3 报告请求体
type CreateReportRequest struct {
	Name          string             `json:"name"`
	StartDate     string             `json:"startDate"`    // YYYY-MM-DD
	EndDate       string             `json:"endDate"`      // YYYY-MM-DD
	Configuration ReportConfiguration `json:"configuration"`
}

// ReportConfiguration 报告配置
type ReportConfiguration struct {
	AdProduct    string         `json:"adProduct"`   // SPONSORED_PRODUCTS / SPONSORED_BRANDS / SPONSORED_DISPLAY
	GroupBy      []string       `json:"groupBy"`     // 维度分组
	Columns      []string       `json:"columns"`     // 需要的指标列
	ReportTypeId string         `json:"reportTypeId"` // 报告类型 ID
	TimeUnit     string         `json:"timeUnit"`    // DAILY / SUMMARY
	Format       string         `json:"format"`      // GZIP_JSON
	Filters      []ReportFilter `json:"filters,omitempty"`
}

// ReportFilter 报告过滤条件
type ReportFilter struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
}

// CreateReportResponse v3 报告创建响应
type CreateReportResponse struct {
	ReportId  string `json:"reportId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// GetReportResponse v3 报告状态响应
type GetReportResponse struct {
	ReportId      string `json:"reportId"`
	Status        string `json:"status"`      // PENDING / PROCESSING / COMPLETED / FAILED
	StatusDetails string `json:"statusDetails"`
	Url           string `json:"url"`         // 下载 URL（COMPLETED 时有效）
	FileSize      int64  `json:"fileSize"`
	CreatedAt     string `json:"createdAt"`
	CompletedAt   string `json:"completedAt"`
}

// GetProfiles 获取广告账户配置文件列表
func (c *Client) GetProfiles(ctx context.Context) ([]Profile, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/v2/profiles", nil)
	if err != nil {
		return nil, err
	}

	var profiles []Profile
	return profiles, json.Unmarshal(body, &profiles)
}

// GetCampaigns 获取 Sponsored Products 广告活动列表
func (c *Client) GetCampaigns(ctx context.Context, stateFilter string) ([]Campaign, error) {
	path := "/sp/campaigns"
	if stateFilter != "" {
		path += "?stateFilter=" + stateFilter
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var campaigns []Campaign
	return campaigns, json.Unmarshal(body, &campaigns)
}

// CreateReport 创建 v3 异步广告报告
func (c *Client) CreateReport(ctx context.Context, req CreateReportRequest) (*CreateReportResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequestWithContentType(ctx, http.MethodPost, "/reporting/reports",
		"application/vnd.createasyncreportrequest.v3+json", reqBytes)
	if err != nil {
		return nil, fmt.Errorf("创建广告报告失败: %w", err)
	}

	var resp CreateReportResponse
	return &resp, json.Unmarshal(body, &resp)
}

// GetReport 查询广告报告状态
func (c *Client) GetReport(ctx context.Context, reportId string) (*GetReportResponse, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/reporting/reports/"+reportId, nil)
	if err != nil {
		return nil, err
	}

	var resp GetReportResponse
	return &resp, json.Unmarshal(body, &resp)
}

// WaitForReport 轮询等待广告报告完成，返回下载 URL
func (c *Client) WaitForReport(ctx context.Context, reportId string, pollInterval, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			report, err := c.GetReport(ctx, reportId)
			if err != nil {
				return "", err
			}

			switch report.Status {
			case "COMPLETED":
				if report.Url == "" {
					return "", fmt.Errorf("广告报告 %s 已完成但无下载 URL", reportId)
				}
				slog.Info("广告报告已完成", "reportId", reportId)
				return report.Url, nil
			case "FAILED":
				return "", fmt.Errorf("广告报告 %s 生成失败: %s", reportId, report.StatusDetails)
			}

			if time.Now().After(deadline) {
				return "", fmt.Errorf("等待广告报告 %s 超时", reportId)
			}
		}
	}
}

// ============================================================
// 报告配置函数集
// ============================================================

// DefaultSPCampaignReportConfig SP 活动层级日报告配置
// 官方参考：https://advertising.amazon.com/API/docs/en-us/guides/reporting/v3/report-types
func DefaultSPCampaignReportConfig(startDate, endDate string) CreateReportRequest {
	return CreateReportRequest{
		Name:      fmt.Sprintf("SP活动日报-%s-%s", startDate, endDate),
		StartDate: startDate,
		EndDate:   endDate,
		Configuration: ReportConfiguration{
			AdProduct:    "SPONSORED_PRODUCTS",
			GroupBy:      []string{"campaign", "date"},
			ReportTypeId: "spCampaigns",
			TimeUnit:     "DAILY",
			Format:       "GZIP_JSON",
			Columns: []string{
				"campaignId", "campaignName", "campaignStatus",
				"dailyBudget", "startDate", "endDate",
				"impressions", "clicks", "cost",
				"purchases7d", "purchases14d", "purchases30d",
				"purchasesSameSku7d", "purchasesSameSku14d", "purchasesSameSku30d",
				"sales7d", "sales14d", "sales30d",
				"unitsSoldClicks7d", "unitsSoldClicks14d", "unitsSoldClicks30d",
				"date",
			},
		},
	}
}

// SPSearchTermReportConfig SP 搜索词报告配置（买家实际搜索词 vs 卖家投放词）
// 数据延迟：T+3（广告 API 固有延迟，近 3 天数据可能不完整）
// 官方参考：https://advertising.amazon.com/API/docs/en-us/guides/reporting/v3/report-types#sponsored-products
func SPSearchTermReportConfig(startDate, endDate string) CreateReportRequest {
	return CreateReportRequest{
		Name:      fmt.Sprintf("SP搜索词日报-%s-%s", startDate, endDate),
		StartDate: startDate,
		EndDate:   endDate,
		Configuration: ReportConfiguration{
			AdProduct:    "SPONSORED_PRODUCTS",
			GroupBy:      []string{"searchTerm", "keyword", "campaign", "adGroup", "date"},
			ReportTypeId: "spSearchTerm",
			TimeUnit:     "DAILY",
			Format:       "GZIP_JSON",
			Columns: []string{
				"date", "campaignId", "adGroupId",
				"keywordId", "keywordText", "searchTerm", "matchType",
				"impressions", "clicks", "cost",
				"purchases7d", "sales7d",
				"clickThroughRate", "costPerClick",
				"purchases14d", "sales14d",
			},
		},
	}
}

// SBCampaignReportConfig Sponsored Brands 活动层级日报告配置
// 数据延迟：T+3
// 官方参考：https://advertising.amazon.com/API/docs/en-us/guides/reporting/v3/report-types#sponsored-brands
func SBCampaignReportConfig(startDate, endDate string) CreateReportRequest {
	return CreateReportRequest{
		Name:      fmt.Sprintf("SB活动日报-%s-%s", startDate, endDate),
		StartDate: startDate,
		EndDate:   endDate,
		Configuration: ReportConfiguration{
			AdProduct:    "SPONSORED_BRANDS",
			GroupBy:      []string{"campaign", "date"},
			ReportTypeId: "sbCampaigns",
			TimeUnit:     "DAILY",
			Format:       "GZIP_JSON",
			Columns: []string{
				"date", "campaignId", "campaignName", "campaignStatus",
				"impressions", "clicks", "cost",
				"purchases14d", "sales14d",
				"purchases30d", "sales30d",
				"clickThroughRate", "costPerClick",
			},
		},
	}
}

// SDCampaignReportConfig Sponsored Display 活动层级日报告配置
// 数据延迟：T+3
// 官方参考：https://advertising.amazon.com/API/docs/en-us/guides/reporting/v3/report-types#sponsored-display
func SDCampaignReportConfig(startDate, endDate string) CreateReportRequest {
	return CreateReportRequest{
		Name:      fmt.Sprintf("SD活动日报-%s-%s", startDate, endDate),
		StartDate: startDate,
		EndDate:   endDate,
		Configuration: ReportConfiguration{
			AdProduct:    "SPONSORED_DISPLAY",
			GroupBy:      []string{"campaign", "date"},
			ReportTypeId: "sdCampaigns",
			TimeUnit:     "DAILY",
			Format:       "GZIP_JSON",
			Columns: []string{
				"date", "campaignId", "campaignName", "campaignStatus",
				"impressions", "clicks", "cost",
				"purchasesSameSku14d", "salesSameSku14d",
				"purchasesSameSku30d", "salesSameSku30d",
				"clickThroughRate", "costPerClick",
			},
		},
	}
}

// doRequest 执行广告 API 请求（JSON Content-Type）
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return c.doRequestWithContentType(ctx, method, path, "application/json", body)
}

// doRequestWithContentType 执行广告 API 请求（指定 Content-Type）
func (c *Client) doRequestWithContentType(ctx context.Context, method, path, contentType string, reqBody []byte) ([]byte, error) {
	accessToken, err := c.tokenManager.GetAccessToken()
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if reqBody != nil {
		bodyReader = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	// 广告 API 必需的 Headers（官方文档要求）
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Amazon-Ads-ClientId", c.clientID)
	req.Header.Set("Amazon-Advertising-API-Scope", c.profileID)
	req.Header.Set("Amazon-Ads-AccountId", c.accountID)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("广告 API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("广告 API 错误 [HTTP %d]: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
