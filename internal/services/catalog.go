// Package services - Catalog Items 元数据同步服务
// 纯只读，仅调用 getCatalogItem / searchCatalogItems 补充 products 表元数据
package services

import (
	"context"
	"log/slog"
	"time"

	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
)

// CatalogService 商品元数据同步服务
type CatalogService struct {
	db     *database.DB
	client *spapi.Client
}

func NewCatalogService(db *database.DB, client *spapi.Client) *CatalogService {
	return &CatalogService{db: db, client: client}
}

// SyncProductCatalog 从 Catalog Items API 补充商品元信息
// 只更新缺失元数据的 ASIN（brand/image_url 为空）
// Rate limit: 2 req/s → 每批 10 个 ASIN，间隔 500ms
func (s *CatalogService) SyncProductCatalog(ctx context.Context, accountID int64, marketplaceID string) (int, error) {
	slog.Info("开始同步商品元数据", "account", accountID, "marketplace", marketplaceID)

	// 查找缺失元数据的 ASIN（brand 或 image_url 为空）
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT asin FROM products
		WHERE account_id = ? AND marketplace_id = ?
		  AND (brand = '' OR brand IS NULL OR image_url = '' OR image_url IS NULL)
		  AND asin != ''
		LIMIT 200
	`, accountID, marketplaceID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var asins []string
	for rows.Next() {
		var asin string
		if err := rows.Scan(&asin); err != nil {
			continue
		}
		asins = append(asins, asin)
	}

	if len(asins) == 0 {
		slog.Info("所有商品元数据已完整，无需同步")
		return 0, nil
	}

	// 分批调用（每批 10 个，遵守 Rate Limit）
	updated := 0
	batchSize := 10
	for i := 0; i < len(asins); i += batchSize {
		end := i + batchSize
		if end > len(asins) {
			end = len(asins)
		}
		batch := asins[i:end]

		items, err := s.client.GetCatalogItems(ctx, marketplaceID, batch)
		if err != nil {
			slog.Warn("Catalog API 调用失败", "batch", i/batchSize, "error", err)
			// 失败不中断，继续下一批
			time.Sleep(2 * time.Second)
			continue
		}

		for _, item := range items {
			if item.ASIN == "" {
				continue
			}
			_, err := s.db.ExecContext(ctx, `
				UPDATE products SET
					brand = CASE WHEN (brand = '' OR brand IS NULL) THEN ? ELSE brand END,
					category = CASE WHEN (category = '' OR category IS NULL) THEN ? ELSE category END,
					image_url = CASE WHEN (image_url = '' OR image_url IS NULL) THEN ? ELSE image_url END,
					catalog_synced_at = ?
				WHERE account_id = ? AND marketplace_id = ? AND asin = ?
			`, item.Brand, item.Category, item.ImageURL,
				time.Now().UTC().Format("2006-01-02 15:04:05"),
				accountID, marketplaceID, item.ASIN)
			if err != nil {
				slog.Warn("更新商品元数据失败", "asin", item.ASIN, "error", err)
				continue
			}
			updated++
		}

		// Rate Limit 间隔
		if end < len(asins) {
			time.Sleep(600 * time.Millisecond)
		}
	}

	slog.Info("商品元数据同步完成", "account", accountID, "updated", updated, "total", len(asins))
	return updated, nil
}
