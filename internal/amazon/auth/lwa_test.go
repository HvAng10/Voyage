package auth

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestLWATokenManager(t *testing.T) {
	manager := NewLWATokenManager("client-id", "secret", "refresh-token")

	// 注入假代理对象来模拟拦截发往亚马逊网关的请求
	manager.httpClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.String(), "auth/o2/token") {
				t.Errorf("Unexpected URL: %s", req.URL.String())
			}
			if req.Method != "POST" {
				t.Errorf("Expected POST request, got %s", req.Method)
			}

			// 模拟正常响应
			jsonResp := `{"access_token": "mock-access-token", "expires_in": 3600, "token_type": "bearer"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
				Header:     make(http.Header),
			}, nil
		},
	}

	// 1. 测试 Token 首次获取
	token, err := manager.GetAccessToken()
	if err != nil {
		t.Fatalf("GetAccessToken failed: %v", err)
	}
	if token != "mock-access-token" {
		t.Errorf("Expected mock-access-token, got %s", token)
	}

	// 2. 测试缓存机制：第二次调用不应该触发网络请求机制 (roundTripFunc)
	triggered := false
	manager.httpClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			triggered = true
			return nil, nil
		},
	}
	token2, err := manager.GetAccessToken()
	if err != nil {
		t.Fatalf("GetAccessToken 2 failed: %v", err)
	}
	if token2 != "mock-access-token" || triggered {
		t.Errorf("Cache failed: token=%s, network triggered=%v", token2, triggered)
	}

	// 3. 测试强制废除缓存和更新
	manager.Invalidate()
	
	manager.httpClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			jsonResp := `{"access_token": "new-access-token", "expires_in": 3600, "token_type": "bearer"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
				Header:     make(http.Header),
			}, nil
		},
	}

	token3, err := manager.GetAccessToken()
	if err != nil {
		t.Fatalf("GetAccessToken after invalidate failed: %v", err)
	}
	if token3 != "new-access-token" {
		t.Errorf("Expected new-access-token, got %s", token3)
	}
}

func TestUpdateCredentials(t *testing.T) {
	manager := NewLWATokenManager("abc", "def", "123")
	// 注入将要被置空的缓存
	manager.cached = &cachedToken{accessToken: "old", expiresAt: time.Now().Add(time.Hour)}
	
	manager.UpdateCredentials("a", "b", "c")
	
	if manager.clientID != "a" {
		t.Errorf("client id 发生未成功修改错误")
	}
	if manager.cached != nil {
		t.Errorf("UpdateCredentials 后，缓存理应立刻随之清空并作废，仍驻留则代表拦截失败")
	}
}
