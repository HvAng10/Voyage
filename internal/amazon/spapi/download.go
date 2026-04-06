// Package spapi - 报告文档下载工具
package spapi

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DownloadReportDocument 从 Amazon 预签名 URL 下载报告文档内容
// compressionAlgorithm: "GZIP" 或 ""（无压缩）
// 预签名 URL 不需要额外鉴权头
func (c *Client) DownloadReportDocument(ctx context.Context, downloadURL, compressionAlgorithm string) (string, error) {
	if downloadURL == "" {
		return "", fmt.Errorf("下载地址为空")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造下载请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载报告失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载报告 HTTP 错误: %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body

	// 支持 GZIP 解压
	if strings.EqualFold(compressionAlgorithm, "GZIP") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("GZIP 解压失败: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取报告内容失败: %w", err)
	}
	return string(data), nil
}
