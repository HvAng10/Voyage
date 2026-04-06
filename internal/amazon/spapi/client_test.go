package spapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"voyage/internal/amazon/auth"
)

type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestClient_RetryAndAuth(t *testing.T) {
	// 利用底层接口劫持拦截发送给 Amazon 的任何 HTTP 数据并进行验证
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	var retryCount int
	
	http.DefaultTransport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "auth/o2/token") {
				// 给 tokenManager 返回虚假的 Token
				jsonResp := `{"access_token": "lwa-token-123", "expires_in": 3600, "token_type": "bearer"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
					Header:     make(http.Header),
				}, nil
			}

			// 验证向后台 SP-API 拦截的 Auth 头是否符合官方携带需求
			if req.Header.Get("x-amz-access-token") != "lwa-token-123" {
				t.Errorf("Header 丢失或 token 伪装传送失败")
			}
			if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
				t.Errorf("Authorization 头缺失强制要求的 Bearer 标记")
			}

			// 第一次直接引发 Amazon Amazon 常见的 429 请求频次超量
			if retryCount == 0 {
				retryCount++
				body := `{"errors":[{"code":"QuotaExceeded","message":"Too many requests"}]}`
				resp := &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
					Header:     make(http.Header),
				}
				// 植入 Retry-After 指示需要排队多久（测试阶段加速模拟）
				resp.Header.Set("Retry-After", "1")
				return resp, nil
			}

			// 第二次模拟 Amazon 返回的查询成功
			retryCount++
			body := `{"payload": "success_response"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}, nil
		},
	}

	tokenMgr := auth.NewLWATokenManager("1", "2", "3")
	client := NewClient(tokenMgr, "na", true)
	
	// 重定义等待上限和基数极小化，让耗时不再强制等待 Amazon 给出的响应上限
	client.retryConfig.InitialDelay = time.Millisecond * 1

	var result struct{ Payload string }
	err := client.Get(context.Background(), "/test-business-endpoint", nil, &result)
	
	if err != nil {
		t.Fatalf("Expected success bypass, got %v", err)
	}

	if result.Payload != "success_response" {
		t.Errorf("Expected payload 'success_response', got %s", result.Payload)
	}

	if retryCount != 2 {
		t.Errorf("Expected API count to reach 2 (one for limiting and block bypass, another for success parsing payload), got %d", retryCount)
	}
}

func TestClient_PostForm(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	http.DefaultTransport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "auth/o2/token") {
				jsonResp := `{"access_token": "token", "expires_in": 3600}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
				}, nil
			}
			
			// 确保证明 POST JSON 被完全应用
			if req.Method != http.MethodPost {
				t.Errorf("Expected POST method, got %v", req.Method)
			}
			
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			}, nil
		},
	}

	tokenMgr := auth.NewLWATokenManager("1", "2", "3")
	client := NewClient(tokenMgr, "eu", true) // Sandbox EU
	
	err := client.Post(context.Background(), "/post_action", map[string]string{"simulate": "data"}, nil)
	if err != nil {
		t.Fatalf("Expected POST bypass mechanism, got err: %v", err)
	}
}
