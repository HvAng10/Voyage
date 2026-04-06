// Package spapi - Catalog Items API v2022-04-01（纯只读接口）
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/catalog-items-api-v2022-04-01-reference
// 注意：不使用 putListingsItem 等写操作，只用 getCatalogItem
package spapi

import (
	"context"
	"net/url"
	"strings"
)

// CatalogItem 商品元数据
type CatalogItem struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Brand    string `json:"brand"`
	Category string `json:"category"`
	ImageURL string `json:"imageUrl"`
}

// catalogItemResponse Catalog Items API 响应结构
type catalogItemResponse struct {
	ASIN              string `json:"asin"`
	Summaries         []struct {
		MarketplaceID string `json:"marketplaceId"`
		ItemName      string `json:"itemName"`
		Brand         string `json:"brand"`
		BrowseNode    string `json:"browseNodeId"`
		Classification struct {
			DisplayName string `json:"displayName"`
		} `json:"classification"`
	} `json:"summaries"`
	Images []struct {
		MarketplaceID string `json:"marketplaceId"`
		Images        []struct {
			Variant string `json:"variant"`
			Link    string `json:"link"`
			Width   int    `json:"width"`
			Height  int    `json:"height"`
		} `json:"images"`
	} `json:"images"`
}

// catalogSearchResponse Catalog Search API 响应
type catalogSearchResponse struct {
	Items []catalogItemResponse `json:"items"`
}

// GetCatalogItem 获取单个 ASIN 的元数据（只读）
// Rate limit: 2 req/s
func (c *Client) GetCatalogItem(ctx context.Context, marketplaceID string, asin string) (*CatalogItem, error) {
	params := url.Values{}
	params.Set("marketplaceIds", marketplaceID)
	params.Set("includedData", "summaries,images")

	var resp catalogItemResponse
	path := "/catalog/2022-04-01/items/" + asin
	if err := c.Get(ctx, path, params, &resp); err != nil {
		return nil, err
	}

	item := &CatalogItem{ASIN: asin}

	// 提取 summaries（取匹配的站点）
	for _, s := range resp.Summaries {
		if s.MarketplaceID == marketplaceID || item.Title == "" {
			item.Title = s.ItemName
			item.Brand = s.Brand
			if s.Classification.DisplayName != "" {
				item.Category = s.Classification.DisplayName
			}
		}
	}

	// 提取主图
	for _, imgGroup := range resp.Images {
		if imgGroup.MarketplaceID == marketplaceID || item.ImageURL == "" {
			for _, img := range imgGroup.Images {
				if img.Variant == "MAIN" && img.Link != "" {
					item.ImageURL = img.Link
					break
				}
			}
		}
	}

	return item, nil
}

// GetCatalogItems 批量获取商品元数据（每次最多 20 个 ASIN）
// 内部分批调用 searchCatalogItems
func (c *Client) GetCatalogItems(ctx context.Context, marketplaceID string, asins []string) ([]CatalogItem, error) {
	params := url.Values{}
	params.Set("marketplaceIds", marketplaceID)
	params.Set("identifiers", strings.Join(asins, ","))
	params.Set("identifiersType", "ASIN")
	params.Set("includedData", "summaries,images")

	var resp catalogSearchResponse
	if err := c.Get(ctx, "/catalog/2022-04-01/items", params, &resp); err != nil {
		return nil, err
	}

	var items []CatalogItem
	for _, r := range resp.Items {
		item := CatalogItem{ASIN: r.ASIN}
		for _, s := range r.Summaries {
			if s.MarketplaceID == marketplaceID || item.Title == "" {
				item.Title = s.ItemName
				item.Brand = s.Brand
				if s.Classification.DisplayName != "" {
					item.Category = s.Classification.DisplayName
				}
			}
		}
		for _, imgGroup := range r.Images {
			for _, img := range imgGroup.Images {
				if img.Variant == "MAIN" && img.Link != "" {
					item.ImageURL = img.Link
					break
				}
			}
		}
		items = append(items, item)
	}

	return items, nil
}
