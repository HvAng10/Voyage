// Package datakiosk 实现 Amazon Data Kiosk API
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/data-kiosk-api-v2023-11-15-reference
// Schema Explorer：https://developer-docs.amazon.com/sp-api/docs/data-kiosk-schema-explorer
package datakiosk

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
	"voyage/internal/amazon/spapi"
)

const (
	// Data Kiosk API 基础路径（版本 2023-11-15）
	dataKioskBase = "/dataKiosk/2023-11-15"
)

// Client Data Kiosk 客户端
type Client struct {
	tokenManager *auth.LWATokenManager
	httpClient   *http.Client
	baseURL      string
}

// NewClient 创建 Data Kiosk 客户端（使用 SP-API 端点）
func NewClient(tokenManager *auth.LWATokenManager, region string, sandbox bool) *Client {
	baseURL := spapi.RegionEndpoints[region]
	if sandbox {
		switch region {
		case "eu":
			baseURL = spapi.SandboxEU
		case "fe":
			baseURL = spapi.SandboxFE
		default:
			baseURL = spapi.SandboxNA
		}
	}
	return &Client{
		tokenManager: tokenManager,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		baseURL:      baseURL,
	}
}

// CreateQueryRequest 创建查询请求
type CreateQueryRequest struct {
	Query string `json:"query"` // GraphQL 查询字符串
}

// CreateQueryResponse 创建查询响应
type CreateQueryResponse struct {
	QueryId string `json:"queryId"`
}

// GetQueryResponse 查询状态响应
type GetQueryResponse struct {
	QueryId                 string `json:"queryId"`
	Query                   string `json:"query"`
	CreatedTime             string `json:"createdTime"`
	ProcessingStatus        string `json:"processingStatus"` // CANCELLED / DONE / FATAL / IN_PROGRESS / IN_QUEUE
	ProcessingStartTime     string `json:"processingStartTime"`
	ProcessingEndTime       string `json:"processingEndTime"`
	DocumentId              string `json:"documentId"` // 非空时可下载
	DataDocumentId          string `json:"dataDocumentId"`
	StatusDetails           string `json:"statusDetails"`
}

// GetDocumentResponse 获取文档下载信息响应
type GetDocumentResponse struct {
	DocumentId          string `json:"documentId"`
	DocumentUrl         string `json:"documentUrl"` // 预签名 S3 URL
	CompressionAlgorithm string `json:"compressionAlgorithm"` // GZIP / 空
}

// CreateQuery 提交 GraphQL 查询
// 官方 Rate limit: 0.0167 req/s（Burst: 15）
func (c *Client) CreateQuery(ctx context.Context, gqlQuery string) (*CreateQueryResponse, error) {
	reqBody, _ := json.Marshal(CreateQueryRequest{Query: gqlQuery})

	accessToken, err := c.tokenManager.GetAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+dataKioskBase+"/queries",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Data Kiosk CreateQuery 失败 [HTTP %d]: %s", resp.StatusCode, body)
	}

	var result CreateQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	slog.Info("Data Kiosk 查询已提交", "queryId", result.QueryId)
	return &result, nil
}

// GetQuery 查询处理状态
// 官方 Rate limit: 0.0167 req/s（Burst: 15）
func (c *Client) GetQuery(ctx context.Context, queryId string) (*GetQueryResponse, error) {
	accessToken, err := c.tokenManager.GetAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+dataKioskBase+"/queries/"+queryId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 Data Kiosk 状态失败 [HTTP %d]: %s", resp.StatusCode, body)
	}

	var result GetQueryResponse
	return &result, json.Unmarshal(body, &result)
}

// GetDocument 获取数据文档下载链接
func (c *Client) GetDocument(ctx context.Context, documentId string) (*GetDocumentResponse, error) {
	accessToken, err := c.tokenManager.GetAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+dataKioskBase+"/documents/"+documentId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 Data Kiosk 文档失败 [HTTP %d]: %s", resp.StatusCode, body)
	}

	var result GetDocumentResponse
	return &result, json.Unmarshal(body, &result)
}

// WaitForQuery 轮询等待 Data Kiosk 查询完成
func (c *Client) WaitForQuery(ctx context.Context, queryId string, pollInterval, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			q, err := c.GetQuery(ctx, queryId)
			if err != nil {
				return "", err
			}

			switch q.ProcessingStatus {
			case "DONE":
				docID := q.DocumentId
				if docID == "" {
					docID = q.DataDocumentId
				}
				if docID == "" {
					return "", fmt.Errorf("Data Kiosk 查询 %s 完成但无文档 ID", queryId)
				}
				return docID, nil
			case "FATAL", "CANCELLED":
				return "", fmt.Errorf("Data Kiosk 查询 %s 失败: %s %s", queryId, q.ProcessingStatus, q.StatusDetails)
			}

			if time.Now().After(deadline) {
				return "", fmt.Errorf("等待 Data Kiosk 查询 %s 超时", queryId)
			}
		}
	}
}

// DownloadAndParseJSONL 下载 JSONL 格式的 Data Kiosk 结果文件
// 每行是一个独立的 JSON 对象，逐行解析
func DownloadAndParseJSONL(ctx context.Context, downloadURL string, onRecord func([]byte) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return parseJSONL(resp.Body, onRecord)
}

// parseJSONL 逐行解析 JSONL 数据
func parseJSONL(r io.Reader, onRecord func([]byte) error) error {
	decoder := json.NewDecoder(r)
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("解析 JSONL 行失败: %w", err)
		}
		if err := onRecord(raw); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================
// 标准 GraphQL 查询模板（来自 Data Kiosk Schema Explorer）
// 官方：https://developer-docs.amazon.com/sp-api/docs/data-kiosk-schema-explorer
// ============================================================

// SalesTrafficByDateQuery 构建按日期维度的销售与流量查询
func SalesTrafficByDateQuery(startDate, endDate, marketplaceId string) string {
	return fmt.Sprintf(`{
  analytics_salesAndTraffic_2023_11_15 {
    salesAndTrafficByDate(
      startDate: "%s"
      endDate: "%s"
      marketplaceIds: ["%s"]
      granularity: DAY
    ) {
      startDate
      endDate
      marketplaceId
      orderedProductSales {
        amount
        currencyCode
      }
      orderedProductSalesB2B {
        amount
        currencyCode
      }
      unitsOrdered
      unitsOrderedB2B
      totalOrderItems
      totalOrderItemsB2B
      pageViews
      pageViewsB2B
      sessions
      sessionsB2B
      browserSessions
      mobileAppSessions
      unitSessionPercentage
      orderItemSessionPercentage
      averageOfferCount
      averageParentItems
      feedbackReceived
      negativeFeedbackReceived
      receivedNegativeFeedbackRate
    }
  }
}`, startDate, endDate, marketplaceId)
}

// SalesTrafficByAsinQuery 构建按 ASIN 维度的销售与流量查询
func SalesTrafficByAsinQuery(startDate, endDate, marketplaceId string) string {
	return fmt.Sprintf(`{
  analytics_salesAndTraffic_2023_11_15 {
    salesAndTrafficByAsin(
      startDate: "%s"
      endDate: "%s"
      marketplaceIds: ["%s"]
    ) {
      parentAsin
      childAsin
      marketplaceId
      orderedProductSales {
        amount
        currencyCode
      }
      unitsOrdered
      totalOrderItems
      pageViews
      sessions
      unitSessionPercentage
      featuredOfferBuyBoxPercentage: buyBoxPercentage
    }
  }
}`, startDate, endDate, marketplaceId)
}
