// Package services - 统一数据查询服务
// 将分散的包级只读查询函数统一挂载到 DataService 结构体，
// 实现依赖注入风格一致性，并默认使用只读连接池
package services

import (
	"voyage/internal/database"
)

// DataService 统一数据查询服务（只读操作）
// 用于替代分散在各文件中的包级函数，统一依赖注入风格
type DataService struct {
	db *database.DB
}

// NewDataService 创建数据查询服务
func NewDataService(db *database.DB) *DataService {
	return &DataService{db: db}
}

// ── 库存查询 ──────────────────────────────────────────────

// GetInventoryItems 获取库存快照列表（代理 inventory.go 的包级函数）
func (s *DataService) GetInventoryItems(accountID int64, marketplaceID string) ([]InventoryItem, error) {
	return GetInventoryItems(s.db, accountID, marketplaceID)
}

// GetReplenishmentAdvice 获取补货建议（代理 replenishment.go）
func (s *DataService) GetReplenishmentAdvice(accountID int64, marketplaceID string, defaultLeadDays int) ([]ReplenishmentAdvice, error) {
	return GetReplenishmentAdvice(s.db, accountID, marketplaceID, defaultLeadDays)
}

// GetInventoryAge 获取库龄数据（代理 inventory_age.go）
func (s *DataService) GetInventoryAge(accountID int64, marketplaceID string) ([]InventoryAgeItem, *InventoryAgeSummary, error) {
	return GetInventoryAge(s.db, accountID, marketplaceID)
}

// ── 退货查询 ──────────────────────────────────────────────

// GetReturnRateByASIN 按 ASIN 退货率统计
func (s *DataService) GetReturnRateByASIN(accountID int64, marketplaceID string, days int) ([]ReturnRateByASIN, error) {
	return GetReturnRateByASIN(s.db, accountID, marketplaceID, days)
}

// GetReturnDetails 退货明细查询
func (s *DataService) GetReturnDetails(accountID int64, marketplaceID string, days int) ([]ReturnDetail, error) {
	return GetReturnDetails(s.db, accountID, marketplaceID, days)
}

// GetReturnReasonDistribution 退货原因分布
func (s *DataService) GetReturnReasonDistribution(accountID int64, marketplaceID string, days int) ([]ReturnReasonStat, error) {
	return GetReturnReasonDistribution(s.db, accountID, marketplaceID, days)
}

// ── 广告查询 ──────────────────────────────────────────────

// GetAdKeywords 关键词广告绩效
func (s *DataService) GetAdKeywords(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]AdKeywordRow, error) {
	return GetAdKeywords(s.db, accountID, marketplaceID, dateStart, dateEnd, limit)
}

// GetAdTargets ASIN 定向广告绩效
func (s *DataService) GetAdTargets(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]AdTargetRow, error) {
	return GetAdTargets(s.db, accountID, marketplaceID, dateStart, dateEnd, limit)
}

// GetAdCampaignDailyTrend 单活动每日趋势
func (s *DataService) GetAdCampaignDailyTrend(accountID int64, campaignID, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	return GetAdCampaignDailyTrend(s.db, accountID, campaignID, dateStart, dateEnd)
}

// GetAdPlacementStats 广告版位详情
func (s *DataService) GetAdPlacementStats(accountID int64, dateStart, dateEnd string) ([]PlacementRow, error) {
	return GetAdPlacementStats(s.db, accountID, dateStart, dateEnd)
}

// GetPlacementSummary 版位汇总
func (s *DataService) GetPlacementSummary(accountID int64, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	return GetPlacementSummary(s.db, accountID, dateStart, dateEnd)
}

// GetSearchTermStats 搜索词统计
func (s *DataService) GetSearchTermStats(accountID int64, dateStart, dateEnd string) ([]SearchTermStat, error) {
	return GetSearchTermStats(s.db, accountID, dateStart, dateEnd)
}

// GetBidSuggestions 竞价建议
func (s *DataService) GetBidSuggestions(accountID int64, marketplaceID, dateStart, dateEnd string, targetACoS float64) ([]BidSuggestion, error) {
	return GetBidSuggestions(s.db, accountID, marketplaceID, dateStart, dateEnd, targetACoS)
}

// ── 财务/价格查询 ─────────────────────────────────────────

// GetProductCosts 商品成本列表
func (s *DataService) GetProductCosts(accountID int64) ([]map[string]interface{}, error) {
	return GetProductCosts(s.db, accountID)
}

// GetCompetitivePrices 竞争价格
func (s *DataService) GetCompetitivePrices(accountID int64, marketplaceID string) ([]CompetitivePriceItem, error) {
	return GetCompetitivePrices(s.db, accountID, marketplaceID)
}

// GetVATRates EU VAT 税率
func (s *DataService) GetVATRates(accountID int64) []VATRate {
	return GetVATRates(s.db, accountID)
}

// GetFinanceSummaryWithVAT 含 VAT 拆分的财务摘要
func (s *DataService) GetFinanceSummaryWithVAT(accountID int64, marketplaceID, dateStart, dateEnd string) map[string]interface{} {
	return GetFinanceSummaryWithVAT(s.db, accountID, marketplaceID, dateStart, dateEnd)
}

// GetCrossAccountKPI 多账户汇总 KPI
func (s *DataService) GetCrossAccountKPI(dateStart, dateEnd string) (*CrossAccountKPI, error) {
	return GetCrossAccountKPI(s.db, dateStart, dateEnd)
}

// ── 导入/导出 ─────────────────────────────────────────────

// ImportCostCSV CSV 成本批量导入
func (s *DataService) ImportCostCSV(accountID int64, csvContent string, defaultCurrency string) CostImportResult {
	return ImportCostCSV(s.db, accountID, csvContent, defaultCurrency)
}

// ExportSalesCSV 导出销售数据
func (s *DataService) ExportSalesCSV(accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	return ExportSalesCSV(s.db, accountID, marketplaceID, dateStart, dateEnd)
}

// ExportInventoryCSV 导出库存数据
func (s *DataService) ExportInventoryCSV(accountID int64, marketplaceID string) (string, error) {
	return ExportInventoryCSV(s.db, accountID, marketplaceID)
}

// ExportAdvertisingCSV 导出广告数据
func (s *DataService) ExportAdvertisingCSV(accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	return ExportAdvertisingCSV(s.db, accountID, marketplaceID, dateStart, dateEnd)
}

// ExportFinanceCSV 导出财务数据
func (s *DataService) ExportFinanceCSV(accountID int64, marketplaceID, dateStart, dateEnd string) (string, error) {
	return ExportFinanceCSV(s.db, accountID, marketplaceID, dateStart, dateEnd)
}

// ── 补货配置写入 ──────────────────────────────────────────

// UpdateReplenishmentConfig 更新补货参数
func (s *DataService) UpdateReplenishmentConfig(accountID int64, sku string, leadTimeDays, safetyDays, targetDays int) error {
	return UpdateReplenishmentConfig(s.db, accountID, sku, leadTimeDays, safetyDays, targetDays)
}



