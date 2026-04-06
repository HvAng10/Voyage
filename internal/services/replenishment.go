// Package services - 补货决策引擎（含旺季系数支持）
// 纯基于已同步的 FBA 库存快照 + 销售数据计算，不调用额外 API
package services

import (
	"log/slog"
	"math"
	"sort"
	"time"

	"voyage/internal/database"
)

// ReplenishmentAdvice 补货建议条目
type ReplenishmentAdvice struct {
	SKU              string  `json:"sku"`
	ASIN             string  `json:"asin"`
	Title            string  `json:"title"`
	CurrentStock     int     `json:"currentStock"`      // 当前 FBA 可售
	InboundQty       int     `json:"inboundQty"`        // 在途数量
	ReservedQty      int     `json:"reservedQty"`       // 预留数量
	DailyAvgSales    float64 `json:"dailyAvgSales"`     // 近 30 天日均（原始）
	SeasonFactor     float64 `json:"seasonFactor"`      // 旺季系数（默认 1.0）
	EffectiveDailyAvg float64 `json:"effectiveDailyAvg"` // 有效日均 = 日均 × 旺季系数
	DaysOfStock      float64 `json:"daysOfStock"`       // 可售天数（含在途，基于有效日均）
	LeadTimeDays     int     `json:"leadTimeDays"`      // 头程周期
	SafetyDays       int     `json:"safetyDays"`        // 安全余量天数
	TargetDays       int     `json:"targetDays"`        // 目标库存天数
	ReorderPoint     int     `json:"reorderPoint"`      // 再下单点（件数）
	SuggestedQty     int     `json:"suggestedQty"`      // 建议补货量
	Urgency          string  `json:"urgency"`           // critical/warning/ok
	EstCost          float64 `json:"estCost"`           // 预估采购成本
	UnitCost         float64 `json:"unitCost"`          // 单件采购成本
}

// SeasonConfig 账户级季度旺季系数配置
type SeasonConfig struct {
	AccountID       int64   `json:"accountId"`
	Q1Factor        float64 `json:"q1Factor"`        // 1-3月
	Q2Factor        float64 `json:"q2Factor"`        // 4-6月
	Q3Factor        float64 `json:"q3Factor"`        // 7-9月（小旺季）
	Q4Factor        float64 `json:"q4Factor"`        // 10-12月（Black Friday/圣诞）
	PrimeDayFactor  float64 `json:"primeDayFactor"`  // Prime Day（7月特殊系数）
	AutoApply       bool    `json:"autoApply"`       // 是否自动按季度使用
}

// getCurrentSeasonFactor 根据当前月份自动返回季度系数
func (sc *SeasonConfig) getCurrentSeasonFactor() float64 {
	month := time.Now().Month()
	// 7月 Prime Day 特殊系数（优先判断）
	if month == 7 && sc.PrimeDayFactor > 1.0 {
		return sc.PrimeDayFactor
	}
	switch {
	case month >= 1 && month <= 3:
		return sc.Q1Factor
	case month >= 4 && month <= 6:
		return sc.Q2Factor
	case month >= 7 && month <= 9:
		return sc.Q3Factor
	default: // 10-12
		return sc.Q4Factor
	}
}

// GetSeasonConfig 获取账户级季度系数配置
func GetSeasonConfig(db *database.DB, accountID int64) (*SeasonConfig, error) {
	cfg := &SeasonConfig{
		AccountID:      accountID,
		Q1Factor:       1.0, Q2Factor: 1.0,
		Q3Factor:       1.1, Q4Factor: 1.5,
		PrimeDayFactor: 1.3, AutoApply: false,
	}
	db.QueryRow(`
		SELECT q1_factor, q2_factor, q3_factor, q4_factor, prime_day_factor, auto_apply
		FROM season_config WHERE account_id = ?
	`, accountID).Scan(
		&cfg.Q1Factor, &cfg.Q2Factor, &cfg.Q3Factor, &cfg.Q4Factor,
		&cfg.PrimeDayFactor, &cfg.AutoApply,
	)
	return cfg, nil
}

// SaveSeasonConfig 保存账户级季度系数配置
func SaveSeasonConfig(db *database.DB, cfg *SeasonConfig) error {
	_, err := db.Exec(`
		INSERT INTO season_config (
			account_id, q1_factor, q2_factor, q3_factor, q4_factor,
			prime_day_factor, auto_apply, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(account_id) DO UPDATE SET
			q1_factor = excluded.q1_factor,
			q2_factor = excluded.q2_factor,
			q3_factor = excluded.q3_factor,
			q4_factor = excluded.q4_factor,
			prime_day_factor = excluded.prime_day_factor,
			auto_apply = excluded.auto_apply,
			updated_at = datetime('now')
	`, cfg.AccountID, cfg.Q1Factor, cfg.Q2Factor, cfg.Q3Factor, cfg.Q4Factor,
		cfg.PrimeDayFactor, boolToInt(cfg.AutoApply))
	return err
}

// GetReplenishmentAdvice 计算补货建议（含旺季系数）
// defaultLeadDays: 用户可从前端传入默认头程周期（默认 30 天）
// globalSeasonFactor: 全局旺季系数覆盖（0 表示使用各 SKU 配置值）
func GetReplenishmentAdvice(db *database.DB, accountID int64, marketplaceID string, defaultLeadDays int) ([]ReplenishmentAdvice, error) {
	if defaultLeadDays <= 0 {
		defaultLeadDays = 30
	}

	// ⚠️ 必须在 db.Query() 之前调用 GetSeasonConfig
	// 原因：写连接池 MaxOpenConns=1，db.Query() 会持有唯一连接直到 rows.Close()
	// 若在 rows 未关闭时再调 db.QueryRow()，会导致连接池死锁（永久阻塞）
	autoSeasonFactor := 0.0 // 0 表示不覆盖，使用各 SKU 自身配置
	if seasonCfg, _ := GetSeasonConfig(db, accountID); seasonCfg != nil && seasonCfg.AutoApply {
		autoSeasonFactor = seasonCfg.getCurrentSeasonFactor()
		slog.Info("补货建议使用自动季度系数", "factor", autoSeasonFactor, "account", accountID)
	}

	// 查询所有在售 SKU 的库存 + 日均销量 + 头程配置 + 采购成本 + 旺季系数
	rows, err := db.Query(`
		SELECT
			i.seller_sku,
			i.asin,
			COALESCE(p.title, i.asin) as title,
			COALESCE(i.fulfillable_qty, 0) as fulfillable,
			COALESCE(i.inbound_qty, 0) as inbound,
			COALESCE(i.reserved_qty, 0) as reserved,
			COALESCE(
				(SELECT SUM(t.units_ordered) * 1.0 / 30
				 FROM sales_traffic_by_asin t
				 WHERE t.asin = i.asin AND t.account_id = i.account_id
				   AND t.marketplace_id = i.marketplace_id
				   AND t.date >= date('now', '-32 days') AND t.date <= date('now', '-2 days')
				 ), 0
			) as daily_avg,
			COALESCE(rc.lead_time_days, ?) as lead_time,
			COALESCE(rc.safety_days, 7) as safety_days,
			COALESCE(rc.target_days, 60) as target_days,
			COALESCE(pc.unit_cost, 0) as unit_cost,
			COALESCE(rc.season_factor, 1.0) as season_factor
		FROM inventory_snapshots i
		LEFT JOIN products p ON i.asin = p.asin AND p.account_id = i.account_id AND p.marketplace_id = i.marketplace_id
		LEFT JOIN replenishment_config rc ON i.seller_sku = rc.seller_sku AND rc.account_id = i.account_id
		LEFT JOIN product_costs pc ON i.seller_sku = pc.seller_sku AND pc.account_id = i.account_id
		WHERE i.account_id = ? AND i.marketplace_id = ?
		  AND i.snapshot_date = (SELECT MAX(snapshot_date) FROM inventory_snapshots WHERE account_id = ? AND marketplace_id = ?)
		  AND i.asin != ''
		ORDER BY daily_avg DESC
	`, defaultLeadDays, accountID, marketplaceID, accountID, marketplaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ReplenishmentAdvice

	for rows.Next() {
		var a ReplenishmentAdvice
		if err := rows.Scan(
			&a.SKU, &a.ASIN, &a.Title,
			&a.CurrentStock, &a.InboundQty, &a.ReservedQty,
			&a.DailyAvgSales, &a.LeadTimeDays, &a.SafetyDays, &a.TargetDays,
			&a.UnitCost, &a.SeasonFactor,
		); err != nil {
			slog.Warn("扫描补货建议行失败", "error", err)
			continue
		}

		// 旺季系数保护（不能 < 0.1 或 > 5.0）
		if a.SeasonFactor < 0.1 {
			a.SeasonFactor = 1.0
		}
		if a.SeasonFactor > 5.0 {
			a.SeasonFactor = 5.0
		}

		// 若账户级 AutoApply 已启用，则覆盖各 SKU 自身的 season_factor
		if autoSeasonFactor > 0 {
			a.SeasonFactor = autoSeasonFactor
		}

		// 有效日均 = 原始日均 × 旺季系数
		a.EffectiveDailyAvg = a.DailyAvgSales * a.SeasonFactor

		// 计算可售天数（基于有效日均）
		totalAvailable := float64(a.CurrentStock + a.InboundQty)
		if a.EffectiveDailyAvg > 0 {
			a.DaysOfStock = totalAvailable / a.EffectiveDailyAvg
		} else if a.CurrentStock > 0 {
			a.DaysOfStock = 999 // 有库存但无销量
		}

		// 再下单点 = 有效日均 × (头程周期 + 安全余量)
		reorderDays := float64(a.LeadTimeDays + a.SafetyDays)
		a.ReorderPoint = int(math.Ceil(a.EffectiveDailyAvg * reorderDays))

		// 建议补货量 = 有效日均 × 目标天数 - 当前可售 - 在途
		suggestedFloat := a.EffectiveDailyAvg*float64(a.TargetDays) - totalAvailable
		if suggestedFloat < 0 {
			suggestedFloat = 0
		}
		a.SuggestedQty = int(math.Ceil(suggestedFloat))

		// 紧急程度判定（基于有效日均）
		if a.EffectiveDailyAvg <= 0 {
			a.Urgency = "ok"
		} else if a.DaysOfStock < float64(a.LeadTimeDays) {
			a.Urgency = "critical"
		} else if totalAvailable <= float64(a.ReorderPoint) {
			a.Urgency = "warning"
		} else {
			a.Urgency = "ok"
		}

		// 预估采购成本
		if a.UnitCost > 0 && a.SuggestedQty > 0 {
			a.EstCost = a.UnitCost * float64(a.SuggestedQty)
		}

		result = append(result, a)
	}

	sortByUrgency(result)
	return result, nil
}

// UpdateReplenishmentConfig 更新单个 SKU 的补货参数（含旺季系数）
func UpdateReplenishmentConfig(db *database.DB, accountID int64, sku string, leadTimeDays, safetyDays, targetDays int) error {
	_, err := db.Exec(`
		INSERT INTO replenishment_config (account_id, seller_sku, lead_time_days, safety_days, target_days, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(account_id, seller_sku) DO UPDATE SET
			lead_time_days = excluded.lead_time_days,
			safety_days = excluded.safety_days,
			target_days = excluded.target_days,
			updated_at = datetime('now')
	`, accountID, sku, leadTimeDays, safetyDays, targetDays)
	return err
}

// UpdateSkuSeasonFactor 更新单个 SKU 的旺季系数
func UpdateSkuSeasonFactor(db *database.DB, accountID int64, sku string, seasonFactor float64) error {
	if seasonFactor < 0.1 {
		seasonFactor = 1.0
	}
	_, err := db.Exec(`
		INSERT INTO replenishment_config (account_id, seller_sku, season_factor, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(account_id, seller_sku) DO UPDATE SET
			season_factor = excluded.season_factor,
			updated_at = datetime('now')
	`, accountID, sku, seasonFactor)
	return err
}

// ApplyGlobalSeasonFactor 将账户级旺季系数批量写入所有 SKU
// （会覆盖各 SKU 原有系数，慎用）
func ApplyGlobalSeasonFactor(db *database.DB, accountID int64, marketplaceID string, factor float64) error {
	if factor < 0.1 {
		factor = 1.0
	}
	_, err := db.Exec(`
		UPDATE replenishment_config SET season_factor = ?, updated_at = datetime('now')
		WHERE account_id = ?
	`, factor, accountID)
	if err != nil {
		return err
	}
	slog.Info("已批量应用旺季系数", "account", accountID, "factor", factor)
	return nil
}

// sortByUrgency 按紧急程度排序（critical > warning > ok，同级按有效日均销量降序）
func sortByUrgency(items []ReplenishmentAdvice) {
	urgencyOrder := map[string]int{"critical": 0, "warning": 1, "ok": 2}
	sort.Slice(items, func(i, j int) bool {
		oi := urgencyOrder[items[i].Urgency]
		oj := urgencyOrder[items[j].Urgency]
		if oi != oj {
			return oi < oj
		}
		return items[i].EffectiveDailyAvg > items[j].EffectiveDailyAvg
	})
}



