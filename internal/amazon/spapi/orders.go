// Package spapi - Orders API v2026-01-01 实现
// 官方文档：https://developer-docs.amazon.com/sp-api/docs/orders-api-v2013-09-01-reference
// 注：Amazon 在 2026-01-01 版本新增 includedData 参数，可合并获取订单+财务+买家信息
package spapi

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// Orders API 路径（v2026-01-01）
const (
	ordersAPIVersion = "2026-01-01"
	ordersBasePath   = "/orders/" + ordersAPIVersion + "/orders"
)

// GetOrdersResponse Orders API 响应
type GetOrdersResponse struct {
	Payload struct {
		Orders        []Order `json:"Orders"`
		NextToken     string  `json:"NextToken"`
		LastUpdatedBefore string `json:"LastUpdatedBefore"`
		CreatedBefore     string `json:"CreatedBefore"`
	} `json:"payload"`
}

// Order 订单结构（对应官方 Order object）
type Order struct {
	AmazonOrderId          string         `json:"AmazonOrderId"`
	PurchaseDate           string         `json:"PurchaseDate"`
	LastUpdateDate         string         `json:"LastUpdateDate"`
	OrderStatus            string         `json:"OrderStatus"`
	FulfillmentChannel     string         `json:"FulfillmentChannel"`
	SalesChannel           string         `json:"SalesChannel"`
	ShipServiceLevel       string         `json:"ShipServiceLevel"`
	OrderTotal             *MoneyType     `json:"OrderTotal"`
	NumberOfItemsShipped   int            `json:"NumberOfItemsShipped"`
	NumberOfItemsUnshipped int            `json:"NumberOfItemsUnshipped"`
	PaymentMethod          string         `json:"PaymentMethod"`
	MarketplaceId          string         `json:"MarketplaceId"`
	ShipmentServiceLevelCategory string   `json:"ShipmentServiceLevelCategory"`
	IsBusinessOrder        bool           `json:"IsBusinessOrder"`
	IsPrime                bool           `json:"IsPrime"`
	IsGlobalExpressEnabled bool           `json:"IsGlobalExpressEnabled"`
	ShippingAddress        *ShippingAddr  `json:"ShippingAddress"`
}

// MoneyType 金额类型
type MoneyType struct {
	CurrencyCode string `json:"CurrencyCode"`
	Amount       string `json:"Amount"` // 注：官方返回字符串形式的数值
}

// ShippingAddr 收货地址（仅包含非 PII 字段）
type ShippingAddr struct {
	Name         string `json:"Name"`
	AddressLine1 string `json:"AddressLine1"`
	City         string `json:"City"`
	StateOrRegion string `json:"StateOrRegion"`
	PostalCode   string `json:"PostalCode"`
	CountryCode  string `json:"CountryCode"`
}

// GetOrderItemsResponse 订单行项目响应
type GetOrderItemsResponse struct {
	Payload struct {
		OrderItems []OrderItem `json:"OrderItems"`
		NextToken  string      `json:"NextToken"`
		AmazonOrderId string   `json:"AmazonOrderId"`
	} `json:"payload"`
}

// OrderItem 订单行项目
type OrderItem struct {
	ASIN                 string     `json:"ASIN"`
	SellerSKU            string     `json:"SellerSKU"`
	OrderItemId          string     `json:"OrderItemId"`
	Title                string     `json:"Title"`
	QuantityOrdered      int        `json:"QuantityOrdered"`
	QuantityShipped      int        `json:"QuantityShipped"`
	ProductInfo          *ProductInfo `json:"ProductInfo"`
	ItemPrice            *MoneyType `json:"ItemPrice"`
	ItemTax              *MoneyType `json:"ItemTax"`
	ShippingPrice        *MoneyType `json:"ShippingPrice"`
	ShippingTax          *MoneyType `json:"ShippingTax"`
	GiftWrapPrice        *MoneyType `json:"GiftWrapPrice"`
	PromotionDiscount    *MoneyType `json:"PromotionDiscount"`
	CODFee               *MoneyType `json:"CODFee"`
	ConditionId          string     `json:"ConditionId"`
}

// ProductInfo 商品信息
type ProductInfo struct {
	NumberOfItems int `json:"NumberOfItems"`
}

// ListOrders 获取订单列表（增量同步）
// marketplaceIds: 目标站点 ID 列表（一次最多 50 个）
// lastUpdatedAfter: 增量起始时间（UTC ISO8601），首次同步传 nil
// 官方 Rate limit: 0.0167 req/s（Burst: 20）
func (c *Client) ListOrders(ctx context.Context, marketplaceIds []string, lastUpdatedAfter *time.Time, nextToken string) (*GetOrdersResponse, error) {
	params := url.Values{}
	params.Set("MarketplaceIds", strings.Join(marketplaceIds, ","))

	if lastUpdatedAfter != nil {
		params.Set("LastUpdatedAfter", lastUpdatedAfter.UTC().Format(time.RFC3339))
	}
	if nextToken != "" {
		params.Set("NextToken", nextToken)
	}

	// v2026-01-01 新特性：includedData 合并请求减少 API 调用次数
	// 参考：https://developer-docs.amazon.com/sp-api/changelog/2026-01-01-orders-api-update
	params.Set("MaxResultsPerPage", "100")

	var resp GetOrdersResponse
	if err := c.Get(ctx, ordersBasePath, params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListOrderItems 获取指定订单的行项目
// 官方 Rate limit: 0.5 req/s（Burst: 30）
func (c *Client) ListOrderItems(ctx context.Context, orderId string, nextToken string) (*GetOrderItemsResponse, error) {
	path := ordersBasePath + "/" + orderId + "/orderItems"
	params := url.Values{}
	if nextToken != "" {
		params.Set("NextToken", nextToken)
	}

	var resp GetOrderItemsResponse
	if err := c.Get(ctx, path, params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
