// Package spapi - Product Pricing API（只读接口）
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/product-pricing-api-v0-reference
package spapi

import (
	"context"
	"net/url"
	"strings"
)

// CompetitivePricingItem 竞品价格条目
type CompetitivePricingItem struct {
	ASIN           string  `json:"-"`
	ListingPrice   float64 `json:"-"`
	ShippingPrice  float64 `json:"-"`
	LandedPrice    float64 `json:"-"`
	BuyBoxPrice    float64 `json:"-"`
	IsBuyBoxWinner bool    `json:"-"`
	NumberOfOffers int     `json:"-"`
}

// competitivePricingResponse Amazon 竞品价格 API 响应结构
type competitivePricingResponse struct {
	Payload []struct {
		ASIN    string `json:"ASIN"`
		Product struct {
			CompetitivePricing struct {
				CompetitivePrices []struct {
					CompetitivePrice struct {
						Condition    string `json:"condition,omitempty"`
						BelongsToRequester bool `json:"belongsToRequester"`
						ListingPrice struct {
							Amount         float64 `json:"Amount"`
							CurrencyCode   string  `json:"CurrencyCode"`
						} `json:"ListingPrice"`
						Shipping struct {
							Amount       float64 `json:"Amount"`
							CurrencyCode string  `json:"CurrencyCode"`
						} `json:"Shipping"`
						LandedPrice struct {
							Amount       float64 `json:"Amount"`
							CurrencyCode string  `json:"CurrencyCode"`
						} `json:"LandedPrice"`
					} `json:"CompetitivePrice"`
				} `json:"CompetitivePrices"`
				NumberOfOfferListings []struct {
					Count     int    `json:"count"`
					Condition string `json:"condition"`
				} `json:"NumberOfOfferListings"`
			} `json:"CompetitivePricing"`
		} `json:"Product"`
	} `json:"payload"`
}

// GetCompetitivePricing 获取一批 ASIN 的竞品价格（只读）
// 官方 Rate limit: 0.5 req/s (Burst: 1)
// 每次最多 20 个 ASIN
func (c *Client) GetCompetitivePricing(ctx context.Context, marketplaceID string, asins []string) ([]CompetitivePricingItem, error) {
	params := url.Values{}
	params.Set("MarketplaceId", marketplaceID)
	params.Set("ItemType", "Asin")
	params.Set("Asins", strings.Join(asins, ","))

	var resp competitivePricingResponse
	if err := c.Get(ctx, "/products/pricing/v0/competitivePrice", params, &resp); err != nil {
		return nil, err
	}

	var items []CompetitivePricingItem
	for _, p := range resp.Payload {
		item := CompetitivePricingItem{
			ASIN: p.ASIN,
		}

		// 遍历竞品价格，找到 Buy Box 价格
		for _, cp := range p.Product.CompetitivePricing.CompetitivePrices {
			price := cp.CompetitivePrice
			if price.BelongsToRequester {
				item.IsBuyBoxWinner = true
				item.ListingPrice = price.ListingPrice.Amount
				item.ShippingPrice = price.Shipping.Amount
				item.LandedPrice = price.LandedPrice.Amount
			}
			// Buy Box 价格取 LandedPrice
			if price.LandedPrice.Amount > 0 {
				item.BuyBoxPrice = price.LandedPrice.Amount
			}
		}

		// 竞争卖家数量
		for _, o := range p.Product.CompetitivePricing.NumberOfOfferListings {
			if o.Condition == "New" || o.Condition == "" {
				item.NumberOfOffers = o.Count
			}
		}

		items = append(items, item)
	}

	return items, nil
}
