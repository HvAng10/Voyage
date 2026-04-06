// Package auth 实现 Amazon LWA（Login with Amazon）OAuth2 认证
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/authorizing-selling-partner-api-applications
package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// LWA Token 端点（官方固定地址）
	lwaTokenURL = "https://api.amazon.com/auth/o2/token"

	// access_token 提前刷新时间（过期前 5 分钟）
	refreshBeforeExpiry = 5 * time.Minute
)

// lwaTokenResponse LWA Token 接口响应
type lwaTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // 秒，通常 3600
}

// cachedToken 已缓存的 access_token
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// LWATokenManager LWA Token 管理器，负责 token 的获取、缓存和自动刷新
type LWATokenManager struct {
	mu           sync.Mutex
	clientID     string
	clientSecret string
	refreshToken string
	cached       *cachedToken
	httpClient   *http.Client
}

// NewLWATokenManager 创建 Token 管理器
// 凭证在调用方解密后传入，不在本模块持久化存储
func NewLWATokenManager(clientID, clientSecret, refreshToken string) *LWATokenManager {
	return &LWATokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetAccessToken 获取有效的 access_token（自动处理缓存和刷新）
func (m *LWATokenManager) GetAccessToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查缓存是否仍有效（预留 5 分钟余量）
	if m.cached != nil && time.Now().Before(m.cached.expiresAt.Add(-refreshBeforeExpiry)) {
		return m.cached.accessToken, nil
	}

	return m.refreshAccessToken()
}

// refreshAccessToken 调用 LWA Token 端点刷新 access_token（已加锁状态下调用）
func (m *LWATokenManager) refreshAccessToken() (string, error) {
	slog.Info("刷新 LWA access_token")

	// 构建请求参数（官方文档规定的参数格式）
	params := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {m.refreshToken},
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
	}

	req, err := http.NewRequest(http.MethodPost, lwaTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("构建 LWA Token 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LWA Token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 LWA Token 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LWA Token 请求错误 [HTTP %d]: %s", resp.StatusCode, string(body))
	}

	var tokenResp lwaTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("解析 LWA Token 响应失败: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("LWA 返回空的 access_token")
	}

	// 缓存 token
	m.cached = &cachedToken{
		accessToken: tokenResp.AccessToken,
		expiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	// 如果 LWA 返回了新的 refresh_token，更新本地（官方说明：通常不返回）
	if tokenResp.RefreshToken != "" {
		m.refreshToken = tokenResp.RefreshToken
		slog.Info("LWA 返回了新的 refresh_token，已更新")
	}

	slog.Info("access_token 刷新成功", "expires_in_s", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

// UpdateCredentials 更新凭证（切换账户时使用）
func (m *LWATokenManager) UpdateCredentials(clientID, clientSecret, refreshToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientID = clientID
	m.clientSecret = clientSecret
	m.refreshToken = refreshToken
	m.cached = nil // 清除旧缓存
}

// Invalidate 强制使缓存失效，下次访问时重新获取
func (m *LWATokenManager) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cached = nil
}
