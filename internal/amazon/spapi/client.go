// Package spapi 实现 Amazon SP-API HTTP 客户端基础
// 官方端点：https://developer-docs.amazon.com/sp-api/docs/sp-api-endpoints
package spapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"voyage/internal/amazon/auth"
)

// SP-API 区域端点（官方）
// 来源：https://developer-docs.amazon.com/sp-api/docs/sp-api-endpoints
const (
	EndpointNA = "https://sellingpartnerapi-na.amazon.com"
	EndpointEU = "https://sellingpartnerapi-eu.amazon.com"
	EndpointFE = "https://sellingpartnerapi-fe.amazon.com"

	// 沙盒端点
	SandboxNA = "https://sandbox.sellingpartnerapi-na.amazon.com"
	SandboxEU = "https://sandbox.sellingpartnerapi-eu.amazon.com"
	SandboxFE = "https://sandbox.sellingpartnerapi-fe.amazon.com"
)

// RegionEndpoints 区域到生产端点的映射
var RegionEndpoints = map[string]string{
	"na": EndpointNA,
	"eu": EndpointEU,
	"fe": EndpointFE,
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts  int           // 最大重试次数（不含首次）
	InitialDelay time.Duration // 初始退避延迟
	MaxDelay     time.Duration // 最大退避延迟
}

var DefaultRetryConfig = RetryConfig{
	MaxAttempts:  3,
	InitialDelay: 1 * time.Second,
	MaxDelay:     30 * time.Second,
}

// Client SP-API HTTP 客户端
type Client struct {
	tokenManager *auth.LWATokenManager
	httpClient   *http.Client
	baseURL      string
	retryConfig  RetryConfig
}

// NewClient 创建 SP-API 客户端
// region: na / eu / fe
func NewClient(tokenManager *auth.LWATokenManager, region string, sandbox bool) *Client {
	baseURL := RegionEndpoints[region]
	if sandbox {
		switch region {
		case "eu":
			baseURL = SandboxEU
		case "fe":
			baseURL = SandboxFE
		default:
			baseURL = SandboxNA
		}
	}

	return &Client{
		tokenManager: tokenManager,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:     baseURL,
		retryConfig: DefaultRetryConfig,
	}
}

// SPAPIError SP-API 返回的错误结构
type SPAPIError struct {
	StatusCode    int
	RetryAfterSec int // 429 时 Retry-After 头指定的等待秒数
	Errors     []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details string `json:"details"`
	} `json:"errors"`
}

func (e *SPAPIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("SP-API 错误 [HTTP %d] %s: %s", e.StatusCode, e.Errors[0].Code, e.Errors[0].Message)
	}
	return fmt.Sprintf("SP-API 错误 [HTTP %d]", e.StatusCode)
}

// Get 执行 GET 请求，带自动重试和指数退避
func (c *Client) Get(ctx context.Context, path string, params url.Values, result interface{}) error {
	return c.doWithRetry(ctx, http.MethodGet, path, params, nil, result)
}

// PostForm 执行带表单参数的 POST 请求
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.doWithRetry(ctx, http.MethodPost, path, nil, body, result)
}

// doWithRetry 带指数退避重试的 HTTP 请求执行器ï¼429 时优先使用 Retry-Afterï¼
func (c *Client) doWithRetry(ctx context.Context, method, path string, params url.Values, body interface{}, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryConfig.MaxAttempts; attempt++ {
		if attempt > 0 {
			// 确定等待时间：优先用 Retry-After，否则指数退避
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * c.retryConfig.InitialDelay
			if apiErr, ok := lastErr.(*SPAPIError); ok && apiErr.RetryAfterSec > 0 {
				delay = time.Duration(apiErr.RetryAfterSec) * time.Second
				slog.Info("SP-API 429 限流，使用 Retry-After", "wait_s", apiErr.RetryAfterSec, "path", path)
			}
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}
			slog.Info("SP-API 请求重试", "attempt", attempt, "delay_s", delay.Seconds(), "path", path)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := c.do(ctx, method, path, params, body, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// 判断是否应该重试
		if apiErr, ok := err.(*SPAPIError); ok {
			// 4xx 错误（除 429）不重试
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 {
				return err
			}
		}
	}

	return fmt.Errorf("SP-API 请求在 %d 次重试后仍失败: %w", c.retryConfig.MaxAttempts, lastErr)
}

// do 执行单次 HTTP 请求
func (c *Client) do(ctx context.Context, method, path string, params url.Values, bodyData interface{}, result interface{}) error {
	// 获取 access_token
	accessToken, err := c.tokenManager.GetAccessToken()
	if err != nil {
		return fmt.Errorf("获取 access_token 失败: %w", err)
	}

	// 构建 URL
	reqURL := c.baseURL + path
	if params != nil && len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	// 构建请求体
	var bodyReader io.Reader
	if bodyData != nil {
		bodyBytes, err := json.Marshal(bodyData)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("构建 HTTP 请求失败: %w", err)
	}

	// 设置 SP-API 必需 Headers
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("x-amz-access-token", accessToken)  // SP-API 也接受此 header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Voyage/1.0 (Language=Go; Platform=Windows)")

	slog.Debug("SP-API 请求", "method", method, "url", reqURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	// 处理错误响应
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr SPAPIError
		apiErr.StatusCode = resp.StatusCode
		json.Unmarshal(respBody, &apiErr) // 忽略解析错误，保留状态码
		// 解析 Retry-After 头（429 限流时 Amazon 会返回）
		if resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if sec, err := strconv.Atoi(ra); err == nil {
					apiErr.RetryAfterSec = sec
				}
			}
		}
		return &apiErr
	}

	// 解析响应
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}
