// Package services - 订单同步服务
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
)

// OrdersService 订单数据同步
type OrdersService struct {
	db     *database.DB
	client *spapi.Client
}

func NewOrdersService(db *database.DB, client *spapi.Client) *OrdersService {
	return &OrdersService{db: db, client: client}
}

// SyncOrders 增量同步订单数据
// accountID: 当前账户 ID
// marketplaceIDs: 目标站点列表
// fullSync: true 则从 90 天前开始，false 则从上次同步时间开始
func (s *OrdersService) SyncOrders(ctx context.Context, accountID int64, marketplaceIDs []string, fullSync bool) (int, error) {
	// 确定增量起始时间
	var lastUpdatedAfter *time.Time
	if !fullSync {
		var lastSync string
		err := s.db.QueryRow(`
			SELECT COALESCE(MAX(last_update_date), '')
			FROM orders WHERE account_id = ?
		`, accountID).Scan(&lastSync)
		if err == nil && lastSync != "" {
			t, err := time.Parse(time.RFC3339, lastSync)
			if err == nil {
				// 往前 1 小时避免漏单
				t = t.Add(-1 * time.Hour)
				lastUpdatedAfter = &t
			}
		}
	}
	if lastUpdatedAfter == nil {
		// 默认拉取近 90 天
		t := time.Now().UTC().AddDate(0, 0, -90)
		lastUpdatedAfter = &t
	}

	slog.Info("开始同步订单", "account", accountID, "since", lastUpdatedAfter.Format(time.RFC3339))

	totalSynced := 0
	nextToken := ""

	for {
		resp, err := s.client.ListOrders(ctx, marketplaceIDs, lastUpdatedAfter, nextToken)
		if err != nil {
			return totalSynced, fmt.Errorf("拉取订单列表失败: %w", err)
		}

		// 批量写入订单
		if err := s.upsertOrders(ctx, accountID, resp.Payload.Orders); err != nil {
			return totalSynced, err
		}
		totalSynced += len(resp.Payload.Orders)

		nextToken = resp.Payload.NextToken
		if nextToken == "" {
			break
		}

		// Rate limit: 0.0167 req/s → 约 60s 一次，但 burst=20，实际可快一些
		// 保守处理：每页之间等待 1s
		select {
		case <-ctx.Done():
			return totalSynced, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	// 同步订单行项目（对 Unshipped/Pending 状态的订单）
	if err := s.syncOrderItems(ctx, accountID); err != nil {
		slog.Warn("同步订单行项目部分失败", "error", err)
	}

	slog.Info("订单同步完成", "account", accountID, "total", totalSynced)
	return totalSynced, nil
}

// upsertOrders 批量 UPSERT 订单到数据库
func (s *OrdersService) upsertOrders(ctx context.Context, accountID int64, orders []spapi.Order) error {
	if len(orders) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO orders (
			amazon_order_id, account_id, marketplace_id, order_status,
			fulfillment_channel, sales_channel, order_total, currency_code,
			purchase_date, last_update_date, ship_service_level,
			is_business_order, is_prime,
			ship_city, ship_state, ship_country, ship_postal_code,
			synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(amazon_order_id) DO UPDATE SET
			order_status        = excluded.order_status,
			fulfillment_channel = excluded.fulfillment_channel,
			order_total         = excluded.order_total,
			last_update_date    = excluded.last_update_date,
			ship_service_level  = excluded.ship_service_level,
			is_business_order   = excluded.is_business_order,
			is_prime            = excluded.is_prime,
			ship_city           = excluded.ship_city,
			ship_state          = excluded.ship_state,
			ship_country        = excluded.ship_country,
			ship_postal_code    = excluded.ship_postal_code,
			synced_at           = datetime('now')
	`)
	if err != nil {
		return fmt.Errorf("准备 UPSERT 语句失败: %w", err)
	}
	defer stmt.Close()

	for _, o := range orders {
		var orderTotal *float64
		var currency string
		if o.OrderTotal != nil {
			if v, err := strconv.ParseFloat(o.OrderTotal.Amount, 64); err == nil {
				orderTotal = &v
			}
			currency = o.OrderTotal.CurrencyCode
		}

		var city, state, country, postal string
		if o.ShippingAddress != nil {
			city   = o.ShippingAddress.City
			state  = o.ShippingAddress.StateOrRegion
			country = o.ShippingAddress.CountryCode
			postal = o.ShippingAddress.PostalCode
		}

		boolToInt := func(b bool) int {
			if b { return 1 }
			return 0
		}

		if _, err := stmt.ExecContext(ctx,
			o.AmazonOrderId, accountID, o.MarketplaceId, o.OrderStatus,
			o.FulfillmentChannel, o.SalesChannel, orderTotal, currency,
			o.PurchaseDate, o.LastUpdateDate, o.ShipServiceLevel,
			boolToInt(o.IsBusinessOrder), boolToInt(o.IsPrime),
			city, state, country, postal,
		); err != nil {
			slog.Warn("写入订单失败", "order_id", o.AmazonOrderId, "error", err)
		}
	}

	return tx.Commit()
}

// syncOrderItems 同步近期未出货订单的行项目
func (s *OrdersService) syncOrderItems(ctx context.Context, accountID int64) error {
	// 只同步最近 7 天内更新的订单的行项目
	rows, err := s.db.QueryContext(ctx, `
		SELECT amazon_order_id FROM orders
		WHERE account_id = ? AND order_status IN ('Pending','Unshipped','PartiallyShipped')
		  AND last_update_date >= datetime('now', '-7 days')
		  AND item_count = 0
		LIMIT 50
	`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var orderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			orderIDs = append(orderIDs, id)
		}
	}

	for _, orderID := range orderIDs {
		if err := s.syncSingleOrderItems(ctx, accountID, orderID); err != nil {
			slog.Warn("同步订单行项目失败", "order_id", orderID, "error", err)
		}
		// Rate limit: 0.5 req/s
		time.Sleep(2 * time.Second)
	}
	return nil
}

// syncSingleOrderItems 同步单个订单的行项目
func (s *OrdersService) syncSingleOrderItems(ctx context.Context, accountID int64, orderID string) error {
	nextToken := ""
	itemCount := 0

	for {
		resp, err := s.client.ListOrderItems(ctx, orderID, nextToken)
		if err != nil {
			return err
		}

		tx, _ := s.db.Begin()
		for _, item := range resp.Payload.OrderItems {
			extractAmount := func(m *spapi.MoneyType) float64 {
				if m == nil { return 0 }
				v, _ := strconv.ParseFloat(m.Amount, 64)
				return v
			}

			tx.ExecContext(ctx, `
				INSERT INTO order_items (
					order_item_id, amazon_order_id, asin, seller_sku, title,
					quantity_ordered, quantity_shipped,
					item_price, item_tax, shipping_price, shipping_tax,
					gift_wrap_price, promotion_discount, cod_fee, condition_id
				) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(order_item_id) DO UPDATE SET
					quantity_shipped = excluded.quantity_shipped,
					item_price = excluded.item_price,
					promotion_discount = excluded.promotion_discount
			`,
				item.OrderItemId, orderID, item.ASIN, item.SellerSKU, item.Title,
				item.QuantityOrdered, item.QuantityShipped,
				extractAmount(item.ItemPrice),
				extractAmount(item.ItemTax),
				extractAmount(item.ShippingPrice),
				extractAmount(item.ShippingTax),
				extractAmount(item.GiftWrapPrice),
				extractAmount(item.PromotionDiscount),
				extractAmount(item.CODFee),
				item.ConditionId,
			)
			itemCount++
		}
		tx.Commit()

		nextToken = resp.Payload.NextToken
		if nextToken == "" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 更新订单的 item_count
	s.db.ExecContext(ctx,
		"UPDATE orders SET item_count = ? WHERE amazon_order_id = ? AND account_id = ?",
		itemCount, orderID, accountID,
	)
	return nil
}

// ── 销售分析查询 ───────────────────────────────────────

// SalesAnalyticsQuery 销售分析查询参数
type SalesAnalyticsQuery struct {
	AccountID     int64
	MarketplaceID string
	DateStart     string // YYYY-MM-DD
	DateEnd       string // YYYY-MM-DD
	Granularity   string // day / week / month
}

// SalesByAsin ASIN 维度销售数据
type SalesByAsin struct {
	ASIN             string  `json:"asin"`
	Title            string  `json:"title"`
	Sales            float64 `json:"sales"`
	Units            int     `json:"units"`
	PageViews        int     `json:"pageViews"`
	Sessions         int     `json:"sessions"`
	ConversionRate   float64 `json:"conversionRate"`
	BuyBoxPct        float64 `json:"buyBoxPct"`
}

// GetSalesByAsin 获取 ASIN 维度销售排行
func (s *DashboardService) GetSalesByAsin(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]SalesByAsin, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT
			t.asin,
			COALESCE(p.title, t.asin) as title,
			COALESCE(SUM(t.ordered_product_sales), 0) as sales,
			COALESCE(SUM(t.units_ordered), 0) as units,
			COALESCE(SUM(t.page_views), 0) as page_views,
			COALESCE(SUM(t.sessions), 0) as sessions,
			COALESCE(AVG(t.unit_session_percentage), 0) as conv_rate,
			COALESCE(AVG(t.buy_box_percentage), 0) as buy_box_pct
		FROM sales_traffic_by_asin t
		LEFT JOIN products p ON t.asin = p.asin
			AND p.account_id = t.account_id AND p.marketplace_id = t.marketplace_id
		WHERE t.account_id = ? AND t.marketplace_id = ?
		  AND t.date >= ? AND t.date <= ?
		GROUP BY t.asin
		ORDER BY sales DESC
		LIMIT ?
	`, accountID, marketplaceID, dateStart, dateEnd, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SalesByAsin
	for rows.Next() {
		var item SalesByAsin
		if err := rows.Scan(
			&item.ASIN, &item.Title, &item.Sales, &item.Units,
			&item.PageViews, &item.Sessions, &item.ConversionRate, &item.BuyBoxPct,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
