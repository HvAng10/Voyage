// Package spapi - Reports API v2021-06-30 实现
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/reports-api-v2021-06-30-reference
package spapi

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	reportsAPIBase = "/reports/2021-06-30"
)

// 常用报告类型常量
// 官方列表：https://developer-docs.amazon.com/sp-api/docs/report-type-values
const (
	// FBA 库存报告（注：两次成功请求间隔至少 30 分钟）
	ReportTypeFBAInventory = "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA"
	// 结算报告（由 Amazon 自动生成，只需轮询）
	ReportTypeSettlementFlat = "GET_V2_SETTLEMENT_REPORT_DATA_FLAT_FILE"
	// 订单报告（备用，Orders API 是首选）
	ReportTypeOrders = "GET_FLAT_FILE_ALL_ORDERS_DATA_BY_LAST_UPDATE_GENERAL"
)

// CreateReportRequest 创建报告请求体
type CreateReportRequest struct {
	ReportType      string       `json:"reportType"`
	MarketplaceIds  []string     `json:"marketplaceIds"`
	DataStartTime   *string      `json:"dataStartTime,omitempty"`
	DataEndTime     *string      `json:"dataEndTime,omitempty"`
	ReportOptions   map[string]string `json:"reportOptions,omitempty"`
}

// CreateReportResponse 创建报告响应
type CreateReportResponse struct {
	ReportId string `json:"reportId"`
}

// GetReportResponse 查询报告状态响应
type GetReportResponse struct {
	ReportId              string   `json:"reportId"`
	ReportType            string   `json:"reportType"`
	DataStartTime         string   `json:"dataStartTime"`
	DataEndTime           string   `json:"dataEndTime"`
	MarketplaceIds        []string `json:"marketplaceIds"`
	ReportScheduleId      string   `json:"reportScheduleId"`
	CreatedTime           string   `json:"createdTime"`
	ProcessingStatus      string   `json:"processingStatus"` // CANCELLED / DONE / FATAL / IN_QUEUE / IN_PROGRESS
	ProcessingStartTime   string   `json:"processingStartTime"`
	ProcessingEndTime     string   `json:"processingEndTime"`
	ReportDocumentId      string   `json:"reportDocumentId"` // 非空时可下载
}

// GetReportDocumentResponse 获取报告文档下载 URL 响应
type GetReportDocumentResponse struct {
	ReportDocumentId  string `json:"reportDocumentId"`
	Url               string `json:"url"`               // 预签名 S3 URL（有效期约 5 分钟）
	CompressionAlgorithm string `json:"compressionAlgorithm"` // GZIP / 空
}

// GetReportsResponse 列表报告响应
type GetReportsResponse struct {
	Reports   []GetReportResponse `json:"reports"`
	NextToken string              `json:"nextToken"`
}

// CreateReport 创建报告任务
// 官方 Rate limit: 0.0167 req/s（Burst: 15）
func (c *Client) CreateReport(ctx context.Context, req CreateReportRequest) (*CreateReportResponse, error) {
	var resp CreateReportResponse
	if err := c.Post(ctx, reportsAPIBase+"/reports", req, &resp); err != nil {
		return nil, fmt.Errorf("创建报告失败 [%s]: %w", req.ReportType, err)
	}
	return &resp, nil
}

// GetReport 查询报告处理状态
// 官方 Rate limit: 2 req/s（Burst: 15）
func (c *Client) GetReport(ctx context.Context, reportId string) (*GetReportResponse, error) {
	var resp GetReportResponse
	if err := c.Get(ctx, reportsAPIBase+"/reports/"+reportId, nil, &resp); err != nil {
		return nil, fmt.Errorf("查询报告状态失败 [%s]: %w", reportId, err)
	}
	return &resp, nil
}

// GetReportDocument 获取报告文档下载信息
// 官方 Rate limit: 0.0167 req/s（Burst: 15）
func (c *Client) GetReportDocument(ctx context.Context, reportDocumentId string) (*GetReportDocumentResponse, error) {
	var resp GetReportDocumentResponse
	if err := c.Get(ctx, reportsAPIBase+"/documents/"+reportDocumentId, nil, &resp); err != nil {
		return nil, fmt.Errorf("获取报告文档失败 [%s]: %w", reportDocumentId, err)
	}
	return &resp, nil
}

// GetReports 列出已完成的报告（用于获取 Settlement 报告，Amazon 自动生成）
// 官方 Rate limit: 0.0222 req/s（Burst: 10）
func (c *Client) GetReports(ctx context.Context, reportTypes []string, marketplaceIds []string, createdSince *time.Time) (*GetReportsResponse, error) {
	params := url.Values{}
	for _, rt := range reportTypes {
		params.Add("reportTypes", rt)
	}
	for _, mid := range marketplaceIds {
		params.Add("marketplaceIds", mid)
	}
	if createdSince != nil {
		params.Set("createdSince", createdSince.UTC().Format(time.RFC3339))
	}
	params.Set("processingStatuses", "DONE")
	params.Set("pageSize", "100")

	var resp GetReportsResponse
	if err := c.Get(ctx, reportsAPIBase+"/reports", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WaitForReport 轮询等待报告完成，返回 reportDocumentId
// 每隔 pollInterval 查询一次，超时后返回错误
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

			switch report.ProcessingStatus {
			case "DONE":
				if report.ReportDocumentId == "" {
					return "", fmt.Errorf("报告 %s 已完成但无文档 ID（可能数据为空）", reportId)
				}
				return report.ReportDocumentId, nil
			case "FATAL", "CANCELLED":
				return "", fmt.Errorf("报告 %s 处理失败，状态：%s", reportId, report.ProcessingStatus)
			// IN_QUEUE / IN_PROGRESS：继续等待
			}

			if time.Now().After(deadline) {
				return "", fmt.Errorf("等待报告 %s 超时（已等待 %.0f 分钟）", reportId, timeout.Minutes())
			}
		}
	}
}
