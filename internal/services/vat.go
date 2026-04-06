// Package services - 欧洲站 VAT 税率管理模块
// 提供 EU 各站点标准 VAT 税率查询、自定义税率覆盖、含税/不含税价格换算
package services

import (
	"fmt"

	"voyage/internal/database"
)

// VATRate VAT 税率记录
type VATRate struct {
	CountryCode string  `json:"countryCode"`
	CountryName string  `json:"countryName"`
	MarketplaceID string `json:"marketplaceId"`
	StandardRate float64 `json:"standardRate"` // 标准税率（百分比）
	ReducedRate  float64 `json:"reducedRate"`  // 降低税率（如适用）
	CustomRate   *float64 `json:"customRate"`  // 用户自定义覆盖税率
	EffectiveRate float64 `json:"effectiveRate"` // 实际生效税率
	IsCustom     bool    `json:"isCustom"`     // 是否使用自定义
}

// 欧洲站 VAT 标准税率（2024 年数据）
var euVATRates = map[string]struct {
	countryName string
	marketplaceID string
	standardRate float64
	reducedRate  float64
}{
	"DE": {"德国", "A1PA6795UKMFR9", 19.0, 7.0},
	"GB": {"英国", "A1F83G8C2ARO7P", 20.0, 5.0},
	"FR": {"法国", "A13V1IB3VIYZZH", 20.0, 5.5},
	"IT": {"意大利", "APJ6JRA9NG5V4", 22.0, 10.0},
	"ES": {"西班牙", "A1RKKUPIHCS9HS", 21.0, 10.0},
	"NL": {"荷兰", "A1805IZSGTT6HS", 21.0, 9.0},
	"SE": {"瑞典", "A2NODRKZP88ZB9", 25.0, 12.0},
	"PL": {"波兰", "A1C3SOZRARQ6R3", 23.0, 8.0},
	"BE": {"比利时", "AMEN7PMS3EDWL", 21.0, 6.0},
}

// GetVATRates 获取所有 EU 站点 VAT 税率（含用户自定义覆盖）
func GetVATRates(db *database.DB, accountID int64) []VATRate {
	var result []VATRate

	for cc, info := range euVATRates {
		rate := VATRate{
			CountryCode:   cc,
			CountryName:   info.countryName,
			MarketplaceID: info.marketplaceID,
			StandardRate:  info.standardRate,
			ReducedRate:   info.reducedRate,
			EffectiveRate: info.standardRate,
			IsCustom:      false,
		}

		// 查询用户自定义覆盖
		var customRate float64
		err := db.QueryRow(`
			SELECT custom_vat_rate FROM vat_config
			WHERE account_id=? AND country_code=?`,
			accountID, cc).Scan(&customRate)
		if err == nil {
			rate.CustomRate = &customRate
			rate.EffectiveRate = customRate
			rate.IsCustom = true
		}

		result = append(result, rate)
	}

	return result
}

// GetVATRateByMarketplace 获取指定站点的生效 VAT 税率
func GetVATRateByMarketplace(db *database.DB, accountID int64, marketplaceID string) (float64, string) {
	for cc, info := range euVATRates {
		if info.marketplaceID == marketplaceID {
			// 先查自定义
			var customRate float64
			err := db.QueryRow(`SELECT custom_vat_rate FROM vat_config WHERE account_id=? AND country_code=?`,
				accountID, cc).Scan(&customRate)
			if err == nil {
				return customRate, cc
			}
			return info.standardRate, cc
		}
	}
	return 0, "" // 非 EU 站返回 0
}

// SaveCustomVATRate 保存用户自定义 VAT 税率
func SaveCustomVATRate(db *database.DB, accountID int64, countryCode string, rate float64) error {
	if rate < 0 || rate > 50 {
		return fmt.Errorf("税率必须在 0%% ~ 50%% 之间")
	}
	_, err := db.Exec(`
		INSERT INTO vat_config (account_id, country_code, custom_vat_rate, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(account_id, country_code) DO UPDATE SET
			custom_vat_rate = excluded.custom_vat_rate,
			updated_at = datetime('now')
	`, accountID, countryCode, rate)
	return err
}

// ResetVATRate 重置为默认标准税率（删除自定义覆盖）
func ResetVATRate(db *database.DB, accountID int64, countryCode string) error {
	_, err := db.Exec(`DELETE FROM vat_config WHERE account_id=? AND country_code=?`,
		accountID, countryCode)
	return err
}

// CalcVATBreakdown 增值税拆分计算（含税价 → 税前 + 税额）
type VATBreakdown struct {
	InclusivePrice float64 `json:"inclusivePrice"` // 含税价
	ExclusivePrice float64 `json:"exclusivePrice"` // 不含税价
	VATAmount      float64 `json:"vatAmount"`       // VAT 金额
	VATRate        float64 `json:"vatRate"`          // 税率（%）
	CountryCode    string  `json:"countryCode"`
}

// CalcVATFromInclusive 从含税价计算 VAT 拆分
func CalcVATFromInclusive(inclusivePrice, vatRatePct float64) VATBreakdown {
	rate := vatRatePct / 100
	exclusive := inclusivePrice / (1 + rate)
	return VATBreakdown{
		InclusivePrice: inclusivePrice,
		ExclusivePrice: exclusive,
		VATAmount:      inclusivePrice - exclusive,
		VATRate:        vatRatePct,
	}
}

// GetFinanceSummaryWithVAT 获取含 VAT 拆分的财务摘要
func GetFinanceSummaryWithVAT(db *database.DB, accountID int64, marketplaceID, dateStart, dateEnd string) map[string]interface{} {
	result := map[string]interface{}{}

	vatRate, countryCode := GetVATRateByMarketplace(db, accountID, marketplaceID)
	result["vatRate"] = vatRate
	result["countryCode"] = countryCode
	result["isEU"] = countryCode != ""

	if countryCode == "" {
		// 非 EU 站，无 VAT
		return result
	}

	// 查询总销售额
	var totalSales float64
	db.QueryRow(`SELECT COALESCE(SUM(ordered_product_sales),0)
		FROM sales_traffic_daily WHERE account_id=? AND marketplace_id=? AND date>=? AND date<=?`,
		accountID, marketplaceID, dateStart, dateEnd).Scan(&totalSales)

	breakdown := CalcVATFromInclusive(totalSales, vatRate)
	result["totalSalesInclVAT"] = breakdown.InclusivePrice
	result["totalSalesExclVAT"] = breakdown.ExclusivePrice
	result["vatAmount"] = breakdown.VATAmount
	result["vatBreakdown"] = breakdown

	return result
}
