// Package services - 模拟数据生成与清空服务（供前端 UI 按钮调用）
// 覆盖全部功能模块的演示数据：双账户(US+UK)、销售/订单/财务/广告/库存/退货/竞品/补货/预警/汇率/旺季/VAT/价格预警
package services

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"voyage/internal/database"
)

// ── 模拟数据常量 ──────────────────────────────────────────

const (
	mockDaysToSeed = 90 // 生成近 90 天的历史数据
)

// 模拟账户定义（支持多账户全局总览）
type mockAccountDef struct {
	ID            int64
	Name          string
	SellerID      string
	Region        string
	MarketplaceID string
	ProfileID     string
	Currency      string
}

var mockAccounts = []mockAccountDef{
	{1, "MockStore US", "AXMOCK12345", "na", "ATVPDKIKX0DER", "PROF_US_001", "USD"},
	{2, "MockStore UK", "AXMOCK67890", "eu", "A1F83G8C2ARO7P", "PROF_UK_001", "GBP"},
}

// 模拟商品定义
type mockProduct struct {
	ASIN  string
	SKU   string
	Title string
	Cost  float64 // 采购成本
	Price float64 // 售价
}

// US 站商品
var mockProductsUS = []mockProduct{
	{"B0D12XXXAA", "SKU-EARBUDS-01", "Premium Wireless Earbuds Pro", 15.50, 49.99},
	{"B0D12XXXBB", "SKU-CHAIR-02", "Ergonomic Mesh Office Chair", 85.00, 259.99},
	{"B0D12XXXCC", "SKU-KEYBD-03", "Mechanical Gaming Keyboard RGB", 22.00, 69.99},
	{"B0D12XXXDD", "SKU-MOUSE-04", "Lightweight Gaming Mouse 65g", 12.00, 39.99},
	{"B0D12XXXEE", "SKU-USBHUB-05", "USB-C Hub 8-in-1 Adapter", 8.50, 29.99},
	{"B0D12XXXFF", "SKU-WEBCAM-06", "4K Streaming Webcam 60fps", 18.00, 59.99},
	{"B0D12XXXGG", "SKU-STAND-07", "Adjustable Laptop Stand Silver", 10.00, 34.99},
	{"B0D12XXXHH", "SKU-CABLE-08", "Braided USB-C to USB-C Cable 2m", 2.50, 12.99},
}

// UK 站商品（少量）
var mockProductsUK = []mockProduct{
	{"B0D12XXKAA", "SKU-EARBUDS-UK1", "Premium Wireless Earbuds Pro", 13.50, 39.99},
	{"B0D12XXKBB", "SKU-KEYBD-UK2", "Mechanical Gaming Keyboard RGB", 19.00, 54.99},
	{"B0D12XXKCC", "SKU-MOUSE-UK3", "Lightweight Gaming Mouse 65g", 10.50, 32.99},
	{"B0D12XXKDD", "SKU-USBHUB-UK4", "USB-C Hub 8-in-1 Adapter", 7.00, 24.99},
}

// 按账户返回对应商品
func getProductsForAccount(acctIdx int) []mockProduct {
	if acctIdx == 0 {
		return mockProductsUS
	}
	return mockProductsUK
}

type mockCampaign struct {
	ID, Name, Type, TargetingType string
	Budget                        float64
}

var mockCampaignsUS = []mockCampaign{
	{"CAMP_SP_AUTO_01", "SP Auto - Earbuds", "sponsoredProducts", "auto", 50.0},
	{"CAMP_SP_MAN_02", "SP Manual - Keyboard & Mouse", "sponsoredProducts", "manual", 80.0},
	{"CAMP_SP_MAN_03", "SP Manual - USB Accessories", "sponsoredProducts", "manual", 30.0},
}

var mockCampaignsUK = []mockCampaign{
	{"CAMP_UK_AUTO_01", "SP Auto - UK Earbuds", "sponsoredProducts", "auto", 35.0},
	{"CAMP_UK_MAN_02", "SP Manual - UK Keyboard", "sponsoredProducts", "manual", 50.0},
}

type mockAdGroup struct {
	ID, CampaignID, Name string
	DefaultBid           float64
}

var mockAdGroupsUS = []mockAdGroup{
	{"AG_01", "CAMP_SP_AUTO_01", "Auto Group - Earbuds", 0.75},
	{"AG_02", "CAMP_SP_MAN_02", "Keyboard Manual", 1.20},
	{"AG_03", "CAMP_SP_MAN_02", "Mouse Manual", 0.90},
	{"AG_04", "CAMP_SP_MAN_03", "USB Hub Manual", 0.60},
}

var mockAdGroupsUK = []mockAdGroup{
	{"AG_UK_01", "CAMP_UK_AUTO_01", "UK Auto Earbuds", 0.55},
	{"AG_UK_02", "CAMP_UK_MAN_02", "UK Keyboard Manual", 0.85},
}

type mockKeyword struct {
	ID, AdGroupID, Text, MatchType string
	Bid                            float64
}

var mockKeywordsUS = []mockKeyword{
	{"KW_01", "AG_02", "mechanical keyboard", "EXACT", 1.50},
	{"KW_02", "AG_02", "gaming keyboard rgb", "PHRASE", 1.20},
	{"KW_03", "AG_02", "keyboard for gaming", "BROAD", 0.85},
	{"KW_04", "AG_03", "gaming mouse lightweight", "EXACT", 1.10},
	{"KW_05", "AG_03", "wireless mouse", "BROAD", 0.70},
	{"KW_06", "AG_04", "usb c hub", "EXACT", 0.80},
	{"KW_07", "AG_04", "usb hub multiport", "PHRASE", 0.55},
	{"KW_08", "AG_04", "laptop accessories", "BROAD", 0.40},
}

var mockKeywordsUK = []mockKeyword{
	{"KW_UK_01", "AG_UK_01", "wireless earbuds", "EXACT", 0.65},
	{"KW_UK_02", "AG_UK_02", "gaming keyboard", "PHRASE", 0.90},
}

// GenerateMockData 生成模拟数据（事务批量写入，双账户 US+UK）
func GenerateMockData(db *database.DB) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": ""}

	tx, err := db.Begin()
	if err != nil {
		result["message"] = fmt.Sprintf("开启事务失败: %v", err)
		return result
	}
	defer tx.Rollback()

	now := time.Now()

	// ══════════════════════════════════════════════════════════
	// 1. 基础数据：Marketplace + 账户 + 账户站点绑定
	// ══════════════════════════════════════════════════════════
	tx.Exec(`INSERT OR IGNORE INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone)
		VALUES ('ATVPDKIKX0DER', 'US', 'Amazon.com', 'USD', 'na', 'America/Los_Angeles')`)
	tx.Exec(`INSERT OR IGNORE INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone)
		VALUES ('A1F83G8C2ARO7P', 'GB', 'Amazon.co.uk', 'GBP', 'eu', 'Europe/London')`)
	tx.Exec(`INSERT OR IGNORE INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone)
		VALUES ('A13V1IB3VIYBER', 'FR', 'Amazon.fr', 'EUR', 'eu', 'Europe/Paris')`)
	tx.Exec(`INSERT OR IGNORE INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone)
		VALUES ('A1PA6795UKMFR9', 'DE', 'Amazon.de', 'EUR', 'eu', 'Europe/Berlin')`)
	tx.Exec(`INSERT OR IGNORE INTO marketplace (marketplace_id, country_code, name, currency_code, region, timezone)
		VALUES ('A1VC38T7YXB528', 'JP', 'Amazon.co.jp', 'JPY', 'fe', 'Asia/Tokyo')`)

	for _, acct := range mockAccounts {
		tx.Exec(`INSERT OR IGNORE INTO accounts (id, name, seller_id, region, is_active) VALUES (?, ?, ?, ?, 1)`,
			acct.ID, acct.Name, acct.SellerID, acct.Region)
		tx.Exec(`INSERT OR IGNORE INTO account_marketplaces (account_id, marketplace_id) VALUES (?, ?)`,
			acct.ID, acct.MarketplaceID)
	}

	totalOrders := 0
	totalAlerts := 0

	// ══════════════════════════════════════════════════════════
	// 遍历每个账户生成数据
	// ══════════════════════════════════════════════════════════
	for acctIdx, acct := range mockAccounts {
		products := getProductsForAccount(acctIdx)
		var campaigns []mockCampaign
		var adGroups []mockAdGroup
		var keywords []mockKeyword
		if acctIdx == 0 {
			campaigns = mockCampaignsUS
			adGroups = mockAdGroupsUS
			keywords = mockKeywordsUS
		} else {
			campaigns = mockCampaignsUK
			adGroups = mockAdGroupsUK
			keywords = mockKeywordsUK
		}

		// ═══ 2. 商品 + 成本 ═══
		for _, p := range products {
			tx.Exec(`INSERT OR REPLACE INTO products (asin, account_id, marketplace_id, seller_sku, title, category, brand, image_url, your_price, currency_code, listing_status, fulfillment, open_date)
				VALUES (?, ?, ?, ?, ?, 'Electronics', 'MockBrand', '', ?, ?, 'Active', 'AFN', datetime('now', '-240 days'))`,
				p.ASIN, acct.ID, acct.MarketplaceID, p.SKU, p.Title, p.Price, acct.Currency)
			tx.Exec(`INSERT OR REPLACE INTO product_costs (account_id, seller_sku, asin, cost_currency, unit_cost, effective_from)
				VALUES (?, ?, ?, ?, ?, datetime('now', '-365 days'))`, acct.ID, p.SKU, p.ASIN, acct.Currency, p.Cost)
		}

		// ═══ 3. 广告活动 + 广告组 + 关键词 ═══
		for _, c := range campaigns {
			tx.Exec(`INSERT OR REPLACE INTO ad_campaigns (campaign_id, account_id, ads_profile_id, marketplace_id, name, campaign_type, targeting_type, state, daily_budget, budget_type, start_date)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'enabled', ?, 'daily', datetime('now', '-180 days'))`,
				c.ID, acct.ID, acct.ProfileID, acct.MarketplaceID, c.Name, c.Type, c.TargetingType, c.Budget)
		}
		for _, g := range adGroups {
			tx.Exec(`INSERT OR REPLACE INTO ad_groups (ad_group_id, account_id, campaign_id, name, state, default_bid)
				VALUES (?, ?, ?, ?, 'enabled', ?)`,
				g.ID, acct.ID, g.CampaignID, g.Name, g.DefaultBid)
		}
		for _, kw := range keywords {
			tx.Exec(`INSERT OR REPLACE INTO ad_keywords (keyword_id, account_id, ad_group_id, keyword_text, match_type, state, bid)
				VALUES (?, ?, ?, ?, ?, 'enabled', ?)`,
				kw.ID, acct.ID, kw.AdGroupID, kw.Text, kw.MatchType, kw.Bid)
		}

		// ═══ 4. 每日循环：销售 + 订单 + 财务 + 广告 ═══
		orderSeq := 1000 + int(acct.ID)*100000
		// UK 站销量因子（稍低于 US）
		salesFactor := 1.0
		if acctIdx == 1 {
			salesFactor = 0.6
		}

		for i := mockDaysToSeed - 1; i >= 0; i-- {
			d := now.AddDate(0, 0, -i)
			date := d.Format("2006-01-02")

			dailySales := 0.0
			dailyUnits := 0

			for _, p := range products {
				baseUnits := rand.Intn(8) + 3
				if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
					baseUnits = int(float64(baseUnits) * 1.3)
				}
				growthFactor := 1.0 + float64(mockDaysToSeed-i)/float64(mockDaysToSeed)*0.3
				units := int(float64(baseUnits) * growthFactor * salesFactor)
				if units < 1 {
					units = 1
				}
				sales := float64(units) * p.Price
				sessions := units*12 + rand.Intn(50)
				pageViews := sessions + rand.Intn(sessions/2+1)
				convRate := float64(units) / float64(sessions) * 100

				// ASIN 级销售流量
				tx.Exec(`INSERT OR REPLACE INTO sales_traffic_by_asin (account_id, marketplace_id, asin, date, ordered_product_sales, units_ordered, total_order_items, page_views, sessions, unit_session_percentage, buy_box_percentage)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					acct.ID, acct.MarketplaceID, p.ASIN, date, sales, units, units, pageViews, sessions, convRate, 90.0+rand.Float64()*10)

				// 模拟订单
				for u := 0; u < units; u += rand.Intn(2) + 1 {
					orderSeq++
					totalOrders++
					orderID := fmt.Sprintf("111-%07d-0000000", orderSeq)
					itemID := fmt.Sprintf("ITEM%d%d", orderSeq, u)
					qtyInOrder := 1
					if u+1 < units && rand.Float64() < 0.3 {
						qtyInOrder = 2
					}
					orderTotal := float64(qtyInOrder) * p.Price
					status := "Shipped"
					if rand.Float64() < 0.03 {
						status = "Canceled"
					}
					salesChannel := "Amazon.com"
					shipCountry := "US"
					if acctIdx == 1 {
						salesChannel = "Amazon.co.uk"
						shipCountry = "GB"
					}
					tx.Exec(`INSERT OR IGNORE INTO orders (amazon_order_id, account_id, marketplace_id, order_status, fulfillment_channel, sales_channel, order_total, currency_code, purchase_date, last_update_date, item_count, is_prime, ship_country)
						VALUES (?, ?, ?, ?, 'AFN', ?, ?, ?, ?, ?, ?, ?, ?)`,
						orderID, acct.ID, acct.MarketplaceID, status, salesChannel, orderTotal, acct.Currency,
						date+"T"+fmt.Sprintf("%02d:%02d:00Z", 8+rand.Intn(14), rand.Intn(60)),
						date+"T23:59:00Z", qtyInOrder, mockBoolInt(rand.Float64() < 0.4),
						shipCountry)
					tx.Exec(`INSERT OR IGNORE INTO order_items (order_item_id, amazon_order_id, asin, seller_sku, title, quantity_ordered, quantity_shipped, item_price, item_tax, shipping_price)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						itemID, orderID, p.ASIN, p.SKU, p.Title, qtyInOrder,
						func() int {
							if status == "Shipped" {
								return qtyInOrder
							}
							return 0
						}(),
						orderTotal, orderTotal*0.08, 0.0)
				}
				dailySales += sales
				dailyUnits += units
			}

			// 店铺级每日聚合
			totalSessions := dailyUnits*12 + rand.Intn(100)
			totalPageViews := totalSessions + rand.Intn(200)
			tx.Exec(`INSERT OR REPLACE INTO sales_traffic_daily (account_id, marketplace_id, date, ordered_product_sales, units_ordered, total_order_items, page_views, sessions, average_selling_price, unit_session_percentage, buy_box_percentage)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, date, dailySales, dailyUnits, dailyUnits, totalPageViews, totalSessions,
				dailySales/float64(dailyUnits), float64(dailyUnits)/float64(totalSessions)*100, 92.0+rand.Float64()*5)

			// 财务事件
			mktFee := dailySales * -(0.13 + rand.Float64()*0.03)
			fbaFee := dailySales * -(0.20 + rand.Float64()*0.05)
			tx.Exec(`INSERT OR REPLACE INTO financial_events (account_id, marketplace_id, event_type, posted_date, principal_amount, marketplace_fee, fba_fee, total_amount, currency_code)
				VALUES (?, ?, 'Order', ?, ?, ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, date+" 12:00:00", dailySales, mktFee, fbaFee, dailySales+mktFee+fbaFee, acct.Currency)

			// 广告效果数据
			for _, c := range campaigns {
				clicks := rand.Intn(40) + 10
				impressions := clicks * (25 + rand.Intn(15))
				cost := float64(clicks) * (0.50 + rand.Float64()*1.0)
				attrSales := cost * (2.5 + rand.Float64()*3.0)
				attrOrders := int(attrSales / 45.0)
				ctr := float64(clicks) / float64(impressions) * 100
				cpc := cost / float64(clicks)
				acos := cost / attrSales * 100

				tx.Exec(`INSERT OR REPLACE INTO ad_performance_daily (campaign_id, account_id, date, impressions, clicks, cost, attributed_sales_7d, attributed_conversions_7d, attributed_units_ordered_7d, click_through_rate, cost_per_click, acos)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					c.ID, acct.ID, date, impressions, clicks, cost, attrSales, attrOrders, attrOrders, ctr, cpc, acos)

				for _, placement := range []string{"Top of Search", "Rest of Search", "Product Pages"} {
					pctShare := 0.33 + rand.Float64()*0.1
					tx.Exec(`INSERT OR REPLACE INTO ad_placement_daily (campaign_id, account_id, date, placement, impressions, clicks, cost, sales_7d, orders_7d)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						c.ID, acct.ID, date, placement,
						int(float64(impressions)*pctShare), int(float64(clicks)*pctShare),
						cost*pctShare, attrSales*pctShare, int(float64(attrOrders)*pctShare))
				}
			}

			// 关键词效果数据
			for _, kw := range keywords {
				clicks := rand.Intn(15) + 2
				impressions := clicks * (30 + rand.Intn(20))
				cost := float64(clicks) * kw.Bid * (0.8 + rand.Float64()*0.4)
				attrSales := cost * (2.0 + rand.Float64()*4.0)
				attrOrders := int(attrSales / 50.0)
				if attrOrders < 0 {
					attrOrders = 0
				}
				tx.Exec(`INSERT OR REPLACE INTO ad_keyword_performance (keyword_id, account_id, date, impressions, clicks, cost, attributed_sales_7d, attributed_orders_7d)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					kw.ID, acct.ID, date, impressions, clicks, cost, attrSales, attrOrders)
			}

			// 搜索词报告（每3天生成一批）
			if i%3 == 0 {
				searchTerms := []string{"wireless earbuds", "gaming keyboard", "usb c hub adapter", "mechanical keyboard cherry", "lightweight mouse gaming", "laptop stand adjustable", "webcam 4k", "usb cable type c"}
				for _, term := range searchTerms {
					clicks := rand.Intn(20) + 1
					impressions := clicks * (20 + rand.Intn(30))
					cost := float64(clicks) * (0.3 + rand.Float64()*0.8)
					sales := cost * (1.5 + rand.Float64()*4.0)
					orders := int(sales / 40.0)
					tx.Exec(`INSERT OR IGNORE INTO ad_search_terms (account_id, date, campaign_id, ad_group_id, keyword_id, keyword_text, search_term, match_type, impressions, clicks, cost, purchases_7d, sales_7d, acos)
						VALUES (?, ?, ?, ?, ?, ?, ?, 'BROAD', ?, ?, ?, ?, ?, ?)`,
						acct.ID, date, campaigns[rand.Intn(len(campaigns))].ID, adGroups[rand.Intn(len(adGroups))].ID,
						keywords[rand.Intn(len(keywords))].ID, keywords[rand.Intn(len(keywords))].Text, term,
						impressions, clicks, cost, orders, sales, func() float64 {
							if sales > 0 {
								return cost / sales * 100
							}
							return 0
						}())
				}
			}

			// economics_daily 聚合
			totalAdSpend := dailySales * (0.10 + rand.Float64()*0.10)
			totalFbaFees := math.Abs(fbaFee)
			totalRefFees := math.Abs(mktFee)
			totalCOGS := dailySales * 0.30
			netProceeds := dailySales - totalAdSpend - totalFbaFees - totalRefFees - totalCOGS
			tx.Exec(`INSERT OR REPLACE INTO economics_daily (account_id, marketplace_id, date, ordered_revenue, shipped_revenue, advertising_spend, fba_fees, referral_fees, cogs, net_proceeds)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, date, dailySales, dailySales*0.95, totalAdSpend, totalFbaFees, totalRefFees, totalCOGS, netProceeds)
		}

		// ═══ 4.5 结算报告 ═══
		settlementCount := 0
		for i := mockDaysToSeed; i >= 14; i -= 14 {
			settlementCount++
			stDate := now.AddDate(0, 0, -i)
			edDate := stDate.AddDate(0, 0, 13)
			depDate := edDate.AddDate(0, 0, 3)
			sId := fmt.Sprintf("SETTLEMENT-MOCK-%d-%04d", acct.ID, settlementCount)
			totalAmt := float64(rand.Intn(5000)+1000) + rand.Float64()
			tx.Exec(`INSERT OR REPLACE INTO settlement_reports (account_id, settlement_id, marketplace_id, settlement_start_date, settlement_end_date, deposit_date, total_amount, currency_code)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				acct.ID, sId, acct.MarketplaceID,
				stDate.Format("2006-01-02T00:00:00Z"),
				edDate.Format("2006-01-02T23:59:59Z"),
				depDate.Format("2006-01-02T12:00:00Z"),
				totalAmt, acct.Currency)
		}

		// ═══ 5. 库存快照 + 库龄 ═══
		snapshotDate := now.Format("2006-01-02")
		for idx, p := range products {
			fulfillable := rand.Intn(150) + 20
			inbound := rand.Intn(50)
			reserved := rand.Intn(10)
			tx.Exec(`INSERT OR REPLACE INTO inventory_snapshots (account_id, marketplace_id, seller_sku, asin, condition_type, fulfillable_qty, inbound_qty, reserved_qty, snapshot_date)
				VALUES (?, ?, ?, ?, 'NewItem', ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, p.SKU, p.ASIN, fulfillable, inbound, reserved, snapshotDate)

			over365 := 0
			ltsf := 0.0
			qty271_365 := 0
			if idx == 0 {
				over365 = 18
				ltsf = 52.30
				qty271_365 = 12
			} else if idx == len(products)-1 {
				qty271_365 = 8
				ltsf = 15.20
			}
			tx.Exec(`INSERT OR REPLACE INTO fba_inventory_age (account_id, marketplace_id, snapshot_date, sku, asin, product_name, condition, qty_0_90_days, qty_91_180_days, qty_181_270_days, qty_271_365_days, qty_over_365_days, est_ltsf, currency, fnsku)
				VALUES (?, ?, ?, ?, ?, ?, 'NewItem', ?, ?, ?, ?, ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, snapshotDate, p.SKU, p.ASIN, p.Title,
				fulfillable-over365-qty271_365, rand.Intn(20), rand.Intn(10), qty271_365, over365, ltsf, acct.Currency,
				fmt.Sprintf("X00%d%s", idx+1, strings.ToUpper(p.SKU[4:8])))
		}

		// ═══ 6. FBA 退货 ═══
		returnReasons := []string{"DEFECTIVE", "NOT_AS_DESCRIBED", "SWITCHEROO", "UNWANTED_ITEM", "MISSED_ESTIMATED_DELIVERY"}
		dispositions := []string{"SELLABLE", "DAMAGED", "CUSTOMER_DAMAGED"}
		returnCount := 35
		if acctIdx == 1 {
			returnCount = 15
		}
		for i := 0; i < returnCount; i++ {
			p := products[rand.Intn(len(products))]
			retDate := now.AddDate(0, 0, -rand.Intn(60)).Format("2006-01-02")
			tx.Exec(`INSERT INTO fba_returns (account_id, marketplace_id, return_date, order_id, sku, asin, product_name, quantity, detailed_disposition, reason, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'Unit returned to inventory')`,
				acct.ID, acct.MarketplaceID, retDate,
				fmt.Sprintf("111-%07d-0000000", 2000+int(acct.ID)*10000+i), p.SKU, p.ASIN, p.Title,
				rand.Intn(2)+1, dispositions[rand.Intn(len(dispositions))], returnReasons[rand.Intn(len(returnReasons))])
		}

		// ═══ 7. 竞品价格 ═══
		for _, p := range products {
			buyBoxPrice := p.Price * (0.95 + rand.Float64()*0.10)
			isBBW := 1
			if rand.Float64() < 0.2 {
				isBBW = 0
				buyBoxPrice = p.Price * 0.90
			}
			tx.Exec(`INSERT OR REPLACE INTO competitive_prices (account_id, marketplace_id, asin, condition_type, listing_price, shipping_price, landed_price, buy_box_price, is_buy_box_winner, number_of_offers, snapshot_date)
				VALUES (?, ?, ?, 'New', ?, 0, ?, ?, ?, ?, ?)`,
				acct.ID, acct.MarketplaceID, p.ASIN, p.Price, p.Price, buyBoxPrice, isBBW, rand.Intn(8)+2, snapshotDate)
		}

		// ═══ 8. 补货配置 ═══
		for _, p := range products {
			tx.Exec(`INSERT OR REPLACE INTO replenishment_config (account_id, seller_sku, lead_time_days, safety_days, target_days, moq, season_factor)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				acct.ID, p.SKU, 25+rand.Intn(20), 7+rand.Intn(7), 45+rand.Intn(30), rand.Intn(50)+10, 1.0+rand.Float64()*0.5)
		}

		// ═══ 9. 智能预警 ═══
		tx.Exec(`DELETE FROM alerts WHERE account_id = ?`, acct.ID)
		alertData := []struct{ Type, Severity, Title, Msg string }{
			{"ltsf_risk", "critical", "⚠️ 长期仓储费预警: " + products[0].SKU, fmt.Sprintf("有 18 件库存超 365 天，预计本月 LTSF 费用 $52.30，建议立即清仓或移除 (%s)", acct.Name)},
			{"stock_out", "critical", "🚨 库存告急: " + products[len(products)-1].SKU, fmt.Sprintf("当前可售库存不足，按日均销量计算即将断货 (%s)", acct.Name)},
			{"sales_drop", "warning", "📉 销量异常下降: " + products[0].Title, fmt.Sprintf("近 7 天销量较前 7 天下降 32%%，建议检查 Buy Box 和广告预算 (%s)", acct.Name)},
			{"acos_spike", "info", "📊 ACoS 持续偏高: " + campaigns[0].Name, fmt.Sprintf("近 14 天 ACoS 高于目标，建议优化关键词出价 (%s)", acct.Name)},
		}
		if acctIdx == 0 {
			alertData = append(alertData,
				struct{ Type, Severity, Title, Msg string }{
					"price_change", "warning", "💲 Buy Box 丢失: USB-C Hub",
					"ASIN B0D12XXXEE Buy Box 被竞争对手抢走（对方报价 $27.99 vs 你的 $29.99）",
				},
				struct{ Type, Severity, Title, Msg string }{
					"return_spike", "warning", "📦 退货率升高: Ergonomic Office Chair",
					"近 30 天退货率 8.2%（行业均值 3.5%），多因 DEFECTIVE，建议检查品控",
				},
			)
		}
		for _, a := range alertData {
			tx.Exec(`INSERT INTO alerts (account_id, alert_type, severity, title, message, is_dismissed) VALUES (?, ?, ?, ?, ?, 0)`,
				acct.ID, a.Type, a.Severity, a.Title, a.Msg)
		}
		totalAlerts += len(alertData)

		// ═══ 10. 旺季系数配置 (season_config: 季度系数 + Prime Day) ═══
		tx.Exec(`INSERT OR REPLACE INTO season_config (account_id, q1_factor, q2_factor, q3_factor, q4_factor, prime_day_factor, auto_apply)
			VALUES (?, 1.0, 1.0, 1.1, 1.5, 1.3, 1)`, acct.ID)

		// ═══ 11. 价格预警配置 ═══
		tx.Exec(`INSERT OR REPLACE INTO price_alert_config (account_id, price_drop_threshold, price_surge_threshold, buybox_critical_hours)
			VALUES (?, 10.0, 15.0, 24)`, acct.ID)
	}

	// ══════════════════════════════════════════════════════════
	// 全局数据（跨账户共享）
	// ══════════════════════════════════════════════════════════

	// ═══ 汇率数据 ═══
	rates := map[string]float64{
		"CNY": 1.0, "USD": 7.25, "EUR": 7.90, "GBP": 9.20, "JPY": 0.048, "CAD": 5.30, "AUD": 4.75, "INR": 0.087,
	}
	nowStr := now.Format("2006-01-02 15:04:05")
	for code, rate := range rates {
		tx.Exec(`INSERT OR REPLACE INTO currency_rates (currency_code, rate_to_cny, updated_at) VALUES (?, ?, ?)`, code, rate, nowStr)
	}

	// ═══ VAT 配置（UK 20%、DE 19%、FR 20%）═══
	tx.Exec(`INSERT OR REPLACE INTO vat_config (account_id, country_code, custom_vat_rate) VALUES (2, 'GB', 20.0)`)
	tx.Exec(`INSERT OR REPLACE INTO vat_config (account_id, country_code, custom_vat_rate) VALUES (2, 'DE', 19.0)`)
	tx.Exec(`INSERT OR REPLACE INTO vat_config (account_id, country_code, custom_vat_rate) VALUES (2, 'FR', 20.0)`)

	// ═══ 提交事务 ═══
	if err := tx.Commit(); err != nil {
		result["message"] = fmt.Sprintf("提交事务失败: %v", err)
		return result
	}

	result["success"] = true
	result["message"] = fmt.Sprintf("模拟数据生成完成：%d 个账户、%d+%d 个商品、%d 天销售记录、%d 条预警",
		len(mockAccounts), len(mockProductsUS), len(mockProductsUK), mockDaysToSeed, totalAlerts)
	return result
}

// ClearMockData 清空全部业务数据表（包含账户、站点、凭证等所有内容）
func ClearMockData(db *database.DB) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": ""}

	// 需要清空的全部数据表（按外键依赖顺序排列，子表在前）
	// 包含所有迁移创建的业务表，仅保留 schema_version 不清
	tables := []string{
		// 广告子表
		"ad_search_terms",
		"ad_keyword_performance",
		"ad_target_performance",
		"ad_placement_daily",
		"ad_performance_daily",
		"ad_keywords",
		"ad_targets",
		"ad_groups",
		"ad_campaigns",
		// 订单子表
		"order_items",
		"orders",
		// 销售与财务
		"sales_traffic_by_asin",
		"sales_traffic_daily",
		"economics_daily",
		"financial_events",
		"settlement_items",
		"settlement_reports",
		// 库存
		"inventory_snapshots",
		"fba_inventory_age",
		"fba_returns",
		// 竞品与定价
		"competitive_prices",
		"price_alert_config",
		// 补货
		"replenishment_config",
		// 商品与成本
		"product_costs",
		"products",
		// 预警
		"alerts",
		"alert_rules",
		// 旺季配置
		"season_config",
		// VAT 配置
		"vat_config",
		// 汇率
		"currency_rates",
		// 同步日志
		"sync_log",
		// 凭证（加密存储的 API 密钥）
		"account_credentials",
		// 账户与站点（放在最后，因为其他表有外键引用）
		"account_marketplaces",
		"accounts",
		// 站点参考表（用户要求清空干净）
		"marketplace",
	}

	tx, err := db.Begin()
	if err != nil {
		result["message"] = fmt.Sprintf("开启事务失败: %v", err)
		return result
	}
	defer tx.Rollback()

	// 暂时关闭外键约束以避免级联问题
	tx.Exec("PRAGMA foreign_keys = OFF")

	cleared := 0
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err == nil {
			cleared++
		}
	}

	tx.Exec("PRAGMA foreign_keys = ON")

	if err := tx.Commit(); err != nil {
		result["message"] = fmt.Sprintf("提交事务失败: %v", err)
		return result
	}

	result["success"] = true
	result["message"] = fmt.Sprintf("已彻底清空 %d 张数据表（含账户、站点、凭证、汇率等全部数据）", cleared)
	return result
}

func mockBoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
