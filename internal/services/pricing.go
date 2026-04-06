// Package services - 竞品价格监控服务（Competitive Pricing API，只读）
// 数据延迟：近实时（API 返回的是缓存价格，约 15 分钟更新）
// 限流：getCompetitivePricing - 0.5 req/s（Burst: 1），需分批处理
package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
)

// PricingService 竞品价格监控（只读，不调用任何定价写操作接口）
type PricingService struct {
	db     *database.DB
	client *spapi.Client
}

func NewPricingService(db *database.DB, client *spapi.Client) *PricingService {
	return &PricingService{db: db, client: client}
}

// CompetitivePriceItem 竞品价格条目
type CompetitivePriceItem struct {
	ASIN           string  `json:"asin"`
	SKU            string  `json:"sku"`
	Title          string  `json:"title"`
	ListingPrice   float64 `json:"listingPrice"`
	BuyBoxPrice    float64 `json:"buyBoxPrice"`
	IsBuyBoxWinner bool    `json:"isBuyBoxWinner"`
	NumberOfOffers int     `json:"numberOfOffers"`
	SnapshotDate   string  `json:"snapshotDate"`
	// 价格异动相关
	PrevBuyBoxPrice float64 `json:"prevBuyBoxPrice"` // 昨日 Buy Box 价格
	PriceChangePct  float64 `json:"priceChangePct"`  // 价格变化幅度 %（负数 = 降价）
	BuyBoxLostDays  int     `json:"buyBoxLostDays"`  // 连续失去 Buy Box 天数
}

// PriceAlertConfig 价格预警阈值配置
type PriceAlertConfig struct {
	AccountID            int64   `json:"accountId"`
	PriceDropThreshold   float64 `json:"priceDropThreshold"`   // 降价幅度阈值（%），超过触发 warning
	PriceSurgeThreshold  float64 `json:"priceSurgeThreshold"`  // 涨价幅度阈值（%），超过触发 info
	BuyBoxCriticalHours  int     `json:"buyBoxCriticalHours"`  // Buy Box 丢失持续 N 小时升级为 critical
}

// GetPriceAlertConfig 获取价格预警配置
func GetPriceAlertConfig(db *database.DB, accountID int64) *PriceAlertConfig {
	cfg := &PriceAlertConfig{
		AccountID:           accountID,
		PriceDropThreshold:  10.0,
		PriceSurgeThreshold: 15.0,
		BuyBoxCriticalHours: 24,
	}
	db.QueryRow(`
		SELECT price_drop_threshold, price_surge_threshold, buybox_critical_hours
		FROM price_alert_config WHERE account_id = ?
	`, accountID).Scan(
		&cfg.PriceDropThreshold, &cfg.PriceSurgeThreshold, &cfg.BuyBoxCriticalHours,
	)
	return cfg
}

// SavePriceAlertConfig 保存价格预警配置
func SavePriceAlertConfig(db *database.DB, cfg *PriceAlertConfig) error {
	_, err := db.Exec(`
		INSERT INTO price_alert_config (account_id, price_drop_threshold, price_surge_threshold, buybox_critical_hours, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(account_id) DO UPDATE SET
			price_drop_threshold  = excluded.price_drop_threshold,
			price_surge_threshold = excluded.price_surge_threshold,
			buybox_critical_hours = excluded.buybox_critical_hours,
			updated_at = datetime('now')
	`, cfg.AccountID, cfg.PriceDropThreshold, cfg.PriceSurgeThreshold, cfg.BuyBoxCriticalHours)
	return err
}

// SyncCompetitivePrices 批量获取在售 ASIN 的竞品价格
// 限制：每次最多 20 个 ASIN（API 限制），间隔 2 秒避免触发 0.5 req/s 限流
func (s *PricingService) SyncCompetitivePrices(ctx context.Context, accountID int64, marketplaceID string) (int, error) {
	slog.Info("开始同步竞品价格", "account", accountID, "marketplace", marketplaceID)

	logID := logSyncStart(s.db, accountID, marketplaceID, "competitive_pricing")

	// 获取所有在售 ASIN（从最新库存快照）
	rows, err := s.db.Query(`
		SELECT DISTINCT asin FROM inventory_snapshots
		WHERE account_id=? AND marketplace_id=?
		  AND snapshot_date=(SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id=? AND marketplace_id=?)
		  AND fulfillable_qty > 0 AND asin != ''
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		logSyncEnd(s.db, logID, "failed", 0, err.Error())
		return 0, err
	}
	// 必须在进入批量处理循环前关闭 rows，防止与循环内 QueryRow 竞争写连接（SQLite 单写者）
	var asins []string
	for rows.Next() {
		var asin string
		if err := rows.Scan(&asin); err == nil && asin != "" {
			asins = append(asins, asin)
		}
	}
	rows.Close() // 提前关闭，不使用 defer（defer 在函数末尾才执行会阻塞后续写操作）

	if len(asins) == 0 {
		logSyncEnd(s.db, logID, "success", 0, "无在售 ASIN")
		return 0, nil
	}

	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	count := 0

	// 分批处理，每批最多 20 个 ASIN
	batchSize := 20
	for i := 0; i < len(asins); i += batchSize {
		end := i + batchSize
		if end > len(asins) {
			end = len(asins)
		}
		batch := asins[i:end]

		// 调用 getCompetitivePricing（只读）
		resp, err := s.client.GetCompetitivePricing(ctx, marketplaceID, batch)
		if err != nil {
			slog.Warn("获取竞品价格失败", "batch", i/batchSize, "error", err)
			continue
		}

		for _, item := range resp {
			// 查询昨日 Buy Box 价格（用于计算价格变动）
			var prevBuyBoxPrice float64
			s.db.QueryRow(`
				SELECT COALESCE(buy_box_price, 0) FROM competitive_prices
				WHERE account_id=? AND marketplace_id=? AND asin=? AND snapshot_date=?
			`, accountID, marketplaceID, item.ASIN, yesterday).Scan(&prevBuyBoxPrice)

			// 计算价格变动百分比（负数 = 降价）
			changePct := 0.0
			if prevBuyBoxPrice > 0 && item.BuyBoxPrice > 0 {
				changePct = (item.BuyBoxPrice - prevBuyBoxPrice) / prevBuyBoxPrice * 100
				changePct = math.Round(changePct*100) / 100 // 保留 2 位小数
			}

			_, err := s.db.Exec(`
				INSERT INTO competitive_prices (
					account_id, marketplace_id, asin, condition_type,
					listing_price, shipping_price, landed_price,
					buy_box_price, is_buy_box_winner, number_of_offers,
					prev_buy_box_price, price_change_pct, snapshot_date
				) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(account_id,marketplace_id,asin,snapshot_date) DO UPDATE SET
					listing_price=excluded.listing_price,
					buy_box_price=excluded.buy_box_price,
					is_buy_box_winner=excluded.is_buy_box_winner,
					number_of_offers=excluded.number_of_offers,
					prev_buy_box_price=excluded.prev_buy_box_price,
					price_change_pct=excluded.price_change_pct
			`, accountID, marketplaceID, item.ASIN, "New",
				item.ListingPrice, item.ShippingPrice, item.LandedPrice,
				item.BuyBoxPrice, boolToInt(item.IsBuyBoxWinner), item.NumberOfOffers,
				prevBuyBoxPrice, changePct, today)
			if err == nil {
				count++
			}
		}

		// 限流保护：每批间隔 2 秒
		if end < len(asins) {
			select {
			case <-ctx.Done():
				logSyncEnd(s.db, logID, "failed", count, ctx.Err().Error())
				return count, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}

	// 检查价格异动预警
	s.checkPriceChangeAlerts(accountID, marketplaceID, today)

	logSyncEnd(s.db, logID, "success", count, "")
	slog.Info("竞品价格同步完成", "account", accountID, "synced", count, "total_asins", len(asins))
	return count, nil
}

// checkPriceChangeAlerts 检查价格异动预警（含 Buy Box 丢失升级逻辑）
// ⚠️ 使用两阶段模式（先读后写）：先将所有数据收集到内存并关闭 rows，
// 再遍历内存数据执行预警写入，避免 MaxOpenConns=1 下的连接池死锁。
func (s *PricingService) checkPriceChangeAlerts(accountID int64, marketplaceID, snapshotDate string) {
	// 读取配置阈值
	cfg := GetPriceAlertConfig(s.db, accountID)

	rows, err := s.db.Query(`
		SELECT cp.asin,
		       cp.buy_box_price, cp.listing_price, cp.number_of_offers,
		       cp.is_buy_box_winner,
		       COALESCE(cp.prev_buy_box_price, 0),
		       COALESCE(cp.price_change_pct, 0)
		FROM competitive_prices cp
		WHERE cp.account_id=? AND cp.marketplace_id=? AND cp.snapshot_date=?
		  AND cp.buy_box_price > 0
	`, accountID, marketplaceID, snapshotDate)
	if err != nil {
		return
	}

	// ── 阶段1：只读数据到内存 ──
	type priceRow struct {
		asin            string
		buyBoxPrice     float64
		listingPrice    float64
		offers          int
		isBBW           int
		prevBuyBoxPrice float64
		changePct       float64
	}
	var items []priceRow
	for rows.Next() {
		var r priceRow
		if err := rows.Scan(
			&r.asin, &r.buyBoxPrice, &r.listingPrice, &r.offers,
			&r.isBBW, &r.prevBuyBoxPrice, &r.changePct,
		); err != nil {
			continue
		}
		items = append(items, r)
	}
	rows.Close() // 提前关闭，释放连接给后续写操作

	// ── 阶段2：遍历内存数据，执行预警写入 ──
	for _, r := range items {
		// ── 1. 价格降幅超过阈值 → warning ──────────────────────────
		if r.prevBuyBoxPrice > 0 && r.changePct < 0 && math.Abs(r.changePct) >= cfg.PriceDropThreshold {
			// 24h 去重
			var dup int
			s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='price_drop'
				AND related_entity_id=? AND is_dismissed=0 AND created_at>datetime('now','-24 hours')`,
				accountID, r.asin).Scan(&dup)
			if dup == 0 {
				s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
					VALUES (?,?,?,?,?,?,?)`,
					accountID, "price_drop", "warning",
					"📉 竞品价格大幅下降",
					fmt.Sprintf("ASIN %s 的 Buy Box 价格从 %.2f 降至 %.2f（降幅 %.1f%%，超过阈值 %.0f%%）",
						r.asin, r.prevBuyBoxPrice, r.buyBoxPrice, math.Abs(r.changePct), cfg.PriceDropThreshold),
					"product", r.asin,
				)
				slog.Info("生成价格降幅预警", "asin", r.asin, "changePct", r.changePct)
			}
		}

		// ── 2. Buy Box 丢失 → warning / 持续超过阈值 → critical ────
		if r.isBBW == 0 {
			// 查询连续失去 Buy Box 的天数
			var lostDays int
			s.db.QueryRow(`
				SELECT COUNT(*) FROM competitive_prices
				WHERE account_id=? AND marketplace_id=? AND asin=?
				  AND is_buy_box_winner=0
				  AND snapshot_date >= date(?, ?)
			`, accountID, marketplaceID, r.asin,
				snapshotDate, fmt.Sprintf("-%d days", cfg.BuyBoxCriticalHours/24)).Scan(&lostDays)

			criticalDays := cfg.BuyBoxCriticalHours / 24
			if criticalDays < 1 {
				criticalDays = 1
			}

			// 确定严重程度
			severity := "warning"
			title := "💲 Buy Box 丢失"
			msgPrefix := ""
			if lostDays >= criticalDays {
				severity = "critical"
				title = "🚨 Buy Box 长期丢失（已升级）"
				msgPrefix = fmt.Sprintf("⚠️ 已持续 %d 天失去 Buy Box！", lostDays)
			}

			// 去重：critical 每 4 小时可重复告一次，warning 每 24 小时
			dedupHours := 24
			if severity == "critical" {
				dedupHours = 4
			}
			var dup int
			s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM alerts WHERE account_id=? AND alert_type='price_change'
				AND related_entity_id=? AND severity=? AND is_dismissed=0
				AND created_at>datetime('now','-%d hours')`, dedupHours),
				accountID, r.asin, severity).Scan(&dup)

			if dup == 0 {
				s.db.Exec(`INSERT INTO alerts (account_id,alert_type,severity,title,message,related_entity_type,related_entity_id)
					VALUES (?,?,?,?,?,?,?)`,
					accountID, "price_change", severity, title,
					fmt.Sprintf("%sASIN %s 未持有 Buy Box（Buy Box 价格 %.2f，你的价格 %.2f，竞争卖家数 %d）",
						msgPrefix, r.asin, r.buyBoxPrice, r.listingPrice, r.offers),
					"product", r.asin,
				)
				slog.Info("生成 Buy Box 丢失预警", "asin", r.asin, "severity", severity, "lostDays", lostDays)
			}
		}
	}
}

// GetCompetitivePrices 查询最新竞品价格数据（前端展示用）
func GetCompetitivePrices(db *database.DB, accountID int64, marketplaceID string) ([]CompetitivePriceItem, error) {
	rows, err := db.Query(`
		SELECT cp.asin,
			COALESCE(p.seller_sku, '') as sku,
			COALESCE(p.title, cp.asin) as title,
			COALESCE(cp.listing_price, 0),
			COALESCE(cp.buy_box_price, 0),
			cp.is_buy_box_winner,
			COALESCE(cp.number_of_offers, 0),
			cp.snapshot_date,
			COALESCE(cp.prev_buy_box_price, 0),
			COALESCE(cp.price_change_pct, 0),
			-- 连续丢失 Buy Box 天数
			COALESCE((
				SELECT COUNT(*) FROM competitive_prices h
				WHERE h.account_id=cp.account_id AND h.marketplace_id=cp.marketplace_id
				  AND h.asin=cp.asin AND h.is_buy_box_winner=0
				  AND h.snapshot_date >= date(cp.snapshot_date, '-7 days')
			), 0) as lost_days
		FROM competitive_prices cp
		LEFT JOIN products p ON cp.asin=p.asin AND cp.account_id=p.account_id AND cp.marketplace_id=p.marketplace_id
		WHERE cp.account_id=? AND cp.marketplace_id=?
		  AND cp.snapshot_date=(SELECT MAX(snapshot_date) FROM competitive_prices WHERE account_id=? AND marketplace_id=?)
		ORDER BY cp.is_buy_box_winner ASC, cp.number_of_offers DESC
	`, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CompetitivePriceItem
	for rows.Next() {
		var item CompetitivePriceItem
		var isBBW int
		if err := rows.Scan(
			&item.ASIN, &item.SKU, &item.Title,
			&item.ListingPrice, &item.BuyBoxPrice,
			&isBBW, &item.NumberOfOffers, &item.SnapshotDate,
			&item.PrevBuyBoxPrice, &item.PriceChangePct,
			&item.BuyBoxLostDays,
		); err != nil {
			slog.Warn("扫描竞品价格行失败", "error", err)
			continue
		}
		item.IsBuyBoxWinner = isBBW == 1
		result = append(result, item)
	}
	return result, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
