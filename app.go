package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"voyage/internal/amazon/advertising"
	"voyage/internal/amazon/auth"
	"voyage/internal/amazon/datakiosk"
	"voyage/internal/amazon/spapi"
	"voyage/internal/config"
	"voyage/internal/database"
	"voyage/internal/scheduler"
	"voyage/internal/services"
)

// App Wails 主应用结构体
type App struct {
	ctx context.Context

	cfg *config.AppConfig
	db  *database.DB

	activeAccountID int64

	tokenManager *auth.LWATokenManager
	spapiClient  *spapi.Client
	dkClient     *datakiosk.Client
	adsClient    *advertising.Client

	// 服务层
	dashSvc     *services.DashboardService
	financeSvc  *services.FinanceService
	alertsSvc   *services.AlertsService
	currencySvc *services.CurrencyService
	dataSvc     *services.DataService

	// 调度器
	sched *scheduler.SyncScheduler

	// 日志文件句柄（shutdown 时关闭）
	logFile *os.File
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 先初始化临时 logger（仅 stdout），用于配置加载阶段
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("Voyage 正在启动...")

	cfg, err := config.New()
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		return
	}
	a.cfg = cfg

	// 配置加载成功后，升级为 stdout + 文件双输出日志
	logFileName := filepath.Join(cfg.LogsDir, "voyage_"+time.Now().Format("20060102")+".log")
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Warn("无法打开日志文件，将仅输出到控制台", "error", err)
	} else {
		a.logFile = logFile
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		logger := slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
		slog.SetDefault(logger)
		slog.Info("日志已启用文件输出", "path", logFileName)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		slog.Error("数据库连接失败", "error", err)
		return
	}
	a.db = db

	// 初始化服务层
	a.dashSvc     = services.NewDashboardService(db)
	a.financeSvc  = services.NewFinanceService(db)
	a.alertsSvc   = services.NewAlertsService(db)
	a.currencySvc = services.NewCurrencyService(db)
	a.dataSvc     = services.NewDataService(db)

	// 启动时同步一次汇率（后续由调度器每日更新）
	go func() {
		if err := a.currencySvc.SyncExchangeRates(context.Background()); err != nil {
			slog.Warn("启动时汇率同步失败（将使用预置值）", "error", err)
		}
	}()

	slog.Info("Voyage 启动完成", "db", cfg.DBPath)
}

func (a *App) shutdown(ctx context.Context) {
	slog.Info("Voyage 正在关闭...")
	if a.db != nil {
		a.db.Close()
	}
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// ── 时区工具 ──────────────────────────────────────────────

func (a *App) GetCurrentTimes(timezone string) map[string]string {
	now := time.Now().UTC()
	cst := time.FixedZone("CST", 8*3600)
	beijingTime := now.In(cst).Format("2006-01-02 15:04:05")
	storeTime := beijingTime
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			storeTime = now.In(loc).Format("2006-01-02 15:04:05")
		}
	}
	return map[string]string{
		"beijing":  beijingTime,
		"store":    storeTime,
		"timezone": timezone,
		"utc":      now.Format(time.RFC3339),
	}
}

// ── 账户管理 ──────────────────────────────────────────────

type AccountInfo struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	SellerID     string   `json:"sellerId"`
	Region       string   `json:"region"`
	IsActive     bool     `json:"isActive"`
	Marketplaces []string `json:"marketplaces"`
}

func (a *App) GetAccounts() ([]AccountInfo, error) {
	if a.db == nil {
		return []AccountInfo{}, nil
	}
	rows, err := a.db.Query(`
		SELECT a.id, a.name, a.seller_id, a.region, a.is_active,
			GROUP_CONCAT(am.marketplace_id, ',') as marketplace_ids
		FROM accounts a
		LEFT JOIN account_marketplaces am ON a.id = am.account_id
		WHERE a.is_active = 1
		GROUP BY a.id ORDER BY a.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []AccountInfo
	for rows.Next() {
		var acc AccountInfo
		var mktStr *string
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.SellerID, &acc.Region, &acc.IsActive, &mktStr); err != nil { slog.Warn("扫描账户行失败", "error", err); continue }
		if mktStr != nil && *mktStr != "" {
			for _, m := range splitCSV(*mktStr) {
				if m != "" {
					acc.Marketplaces = append(acc.Marketplaces, m)
				}
			}
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func (a *App) CreateAccount(name, sellerID, region string, marketplaceIds []string) (int64, error) {
	if a.db == nil {
		return 0, fmt.Errorf("数据库未初始化")
	}
	tx, _ := a.db.Begin()
	defer tx.Rollback()

	r, err := tx.Exec("INSERT INTO accounts(name,seller_id,region) VALUES(?,?,?)", name, sellerID, region)
	if err != nil {
		return 0, err
	}
	id, _ := r.LastInsertId()
	for _, mid := range marketplaceIds {
		tx.Exec("INSERT OR IGNORE INTO account_marketplaces(account_id,marketplace_id) VALUES(?,?)", id, mid)
	}
	return id, tx.Commit()
}

// ── 凭证管理 ──────────────────────────────────────────────

func (a *App) SaveCredential(accountID int64, credType, plaintext string) error {
	if a.cfg == nil {
		return fmt.Errorf("应用配置未初始化")
	}
	encrypted, err := a.cfg.Encryptor.EncryptString(plaintext)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`
		INSERT INTO account_credentials(account_id,credential_type,encrypted_value,updated_at)
		VALUES(?,?,?,datetime('now'))
		ON CONFLICT(account_id,credential_type) DO UPDATE SET
			encrypted_value=excluded.encrypted_value, updated_at=excluded.updated_at
	`, accountID, credType, encrypted)
	return err
}

func (a *App) TestConnection(accountID int64) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": ""}
	creds, err := a.loadCredentials(accountID)
	if err != nil {
		result["message"] = "读取凭证失败: " + err.Error()
		return result
	}
	tm := auth.NewLWATokenManager(creds["lwa_client_id"], creds["lwa_client_secret"], creds["refresh_token"])
	if _, err = tm.GetAccessToken(); err != nil {
		result["message"] = "获取 Access Token 失败: " + err.Error()
		return result
	}
	result["success"] = true
	result["message"] = "连接测试成功 ✓"
	return result
}

func (a *App) loadCredentials(accountID int64) (map[string]string, error) {
	rows, err := a.db.Query("SELECT credential_type, encrypted_value FROM account_credentials WHERE account_id=?", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	creds := make(map[string]string)
	for rows.Next() {
		var ct string
		var enc []byte
		if err := rows.Scan(&ct, &enc); err != nil { slog.Warn("扫描凭证行失败", "error", err); continue }
		plain, err := a.cfg.Encryptor.DecryptString(enc)
		if err != nil {
			return nil, err
		}
		creds[ct] = plain
	}
	return creds, nil
}

// ── Marketplace ───────────────────────────────────────────

type MarketplaceInfo struct {
	MarketplaceID string `json:"marketplaceId"`
	CountryCode   string `json:"countryCode"`
	Name          string `json:"name"`
	CurrencyCode  string `json:"currencyCode"`
	Region        string `json:"region"`
	Timezone      string `json:"timezone"`
}

func (a *App) GetMarketplaces() ([]MarketplaceInfo, error) {
	if a.db == nil {
		return []MarketplaceInfo{}, nil
	}
	rows, err := a.db.Query("SELECT marketplace_id,country_code,name,currency_code,region,timezone FROM marketplace ORDER BY region,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []MarketplaceInfo
	for rows.Next() {
		var m MarketplaceInfo
		if err := rows.Scan(&m.MarketplaceID, &m.CountryCode, &m.Name, &m.CurrencyCode, &m.Region, &m.Timezone); err != nil { slog.Warn("扫描站点行失败", "error", err); continue }
		list = append(list, m)
	}
	return list, nil
}

func (a *App) GetMarketplaceTimezone(marketplaceID string) (string, error) {
	var tz string
	err := a.db.QueryRow("SELECT timezone FROM marketplace WHERE marketplace_id=?", marketplaceID).Scan(&tz)
	return tz, err
}

// ── 仪表盘数据 ────────────────────────────────────────────

func (a *App) GetDashboardKPI(accountID int64, marketplaceID, dateStart, dateEnd string) (*services.DashboardKPI, error) {
	if a.dashSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.dashSvc.GetKPI(accountID, marketplaceID, dateStart, dateEnd)
}

func (a *App) GetDailyTrend(accountID int64, marketplaceID, dateStart, dateEnd string) ([]services.DailyDataPoint, error) {
	if a.dashSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.dashSvc.GetDailyTrend(accountID, marketplaceID, dateStart, dateEnd)
}

// GetProfitMarginTrend 获取每日利润率趋势数据（仪表盘利润率折线图）
func (a *App) GetProfitMarginTrend(accountID int64, marketplaceID, dateStart, dateEnd string) ([]services.ProfitMarginPoint, error) {
	if a.dashSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.dashSvc.GetProfitMarginTrend(accountID, marketplaceID, dateStart, dateEnd)
}

// GetAsinProfitRank 获取 ASIN 利润率排行（最赚钱 / 最亏钱 Top 5）
func (a *App) GetAsinProfitRank(accountID int64, marketplaceID, dateStart, dateEnd string, topN int) (map[string]interface{}, error) {
	if a.dashSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	result, err := a.dashSvc.GetAsinProfitRank(accountID, marketplaceID, dateStart, dateEnd, topN)
	if err != nil {
		return nil, err
	}
	// 转为 map[string]interface{} 以兼容 Wails JSON 序列化
	return map[string]interface{}{
		"best":  result["best"],
		"worst": result["worst"],
	}, nil
}

func (a *App) GetSalesByAsin(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]services.SalesByAsin, error) {
	if a.dashSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.dashSvc.GetSalesByAsin(accountID, marketplaceID, dateStart, dateEnd, limit)
}

// ── 库存数据 ──────────────────────────────────────────────

func (a *App) GetInventoryItems(accountID int64, marketplaceID string) ([]services.InventoryItem, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.dataSvc.GetInventoryItems(accountID, marketplaceID)
}

// GetInventoryOverview 库存健康概览（仪表盘底部）
func (a *App) GetInventoryOverview(accountID int64, marketplaceID string) (*services.InventoryOverview, error) {
	if a.db == nil {
		return &services.InventoryOverview{}, nil
	}
	return a.dashSvc.GetInventoryOverview(accountID, marketplaceID)
}

// GetAdOverview 广告快速概览（仪表盘底部）
func (a *App) GetAdOverview(accountID int64, marketplaceID, dateStart, dateEnd string) (*services.AdOverview, error) {
	if a.db == nil {
		return &services.AdOverview{}, nil
	}
	return a.dashSvc.GetAdOverview(accountID, marketplaceID, dateStart, dateEnd)
}

// GetDailyProfitCalendar 获取指定月份每日利润日历数据
func (a *App) GetDailyProfitCalendar(accountID int64, marketplaceID, yearMonth string) ([]services.DailyProfitCell, error) {
	if a.dashSvc == nil { return []services.DailyProfitCell{}, nil }
	return a.dashSvc.GetDailyProfitCalendar(accountID, marketplaceID, yearMonth)
}

// GetPriceHistory 获取 ASIN 竞品价格历史趋势
func (a *App) GetPriceHistory(accountID int64, marketplaceID, asin string, days int) ([]map[string]interface{}, error) {
	if a.db == nil { return nil, nil }
	if days <= 0 { days = 90 }
	rows, err := a.db.Query(`
		SELECT snapshot_date, COALESCE(buy_box_price, 0), COALESCE(landed_price, 0),
			COALESCE(number_of_offers, 0), COALESCE(is_buy_box_winner, 0)
		FROM competitive_prices
		WHERE account_id=? AND marketplace_id=? AND asin=?
		  AND snapshot_date >= date('now', '-' || ? || ' days')
		ORDER BY snapshot_date ASC
	`, accountID, marketplaceID, asin, days)
	if err != nil { return nil, err }
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var date string
		var buyBox, landed float64
		var offers, winner int
		if err := rows.Scan(&date, &buyBox, &landed, &offers, &winner); err != nil { slog.Warn("扫描价格历史行失败", "error", err); continue }
		result = append(result, map[string]interface{}{
			"date": date, "buyBoxPrice": buyBox, "landedPrice": landed,
			"numberOfOffers": offers, "isBuyBoxWinner": winner == 1,
		})
	}
	return result, nil
}

// ── 广告数据查询 ──────────────────────────────────────────

type AdCampaignSummary struct {
	CampaignID   string  `json:"campaignId"`
	Name         string  `json:"name"`
	State        string  `json:"state"`
	DailyBudget  float64 `json:"dailyBudget"`
	TotalCost    float64 `json:"totalCost"`
	TotalSales   float64 `json:"totalSales"`
	TotalClicks  int     `json:"totalClicks"`
	Impressions  int     `json:"impressions"`
	ACoS         float64 `json:"acos"`
	ROAS         float64 `json:"roas"`
	CTR          float64 `json:"ctr"`
}

func (a *App) GetAdCampaigns(accountID int64, marketplaceID, dateStart, dateEnd string) ([]AdCampaignSummary, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	rows, err := a.db.Query(`
		SELECT
			c.campaign_id, c.name, c.state, c.daily_budget,
			COALESCE(SUM(p.cost),0),
			COALESCE(SUM(p.attributed_sales_7d),0),
			COALESCE(SUM(p.clicks),0),
			COALESCE(SUM(p.impressions),0),
			COALESCE(AVG(p.acos),0),
			COALESCE(AVG(p.roas),0),
			COALESCE(AVG(p.click_through_rate),0)
		FROM ad_campaigns c
		LEFT JOIN ad_performance_daily p ON c.campaign_id=p.campaign_id AND c.account_id=p.account_id
			AND p.date>=? AND p.date<=?
		WHERE c.account_id=? AND c.marketplace_id=?
		GROUP BY c.campaign_id
		ORDER BY SUM(p.cost) DESC
	`, dateStart, dateEnd, accountID, marketplaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AdCampaignSummary
	for rows.Next() {
		var s AdCampaignSummary
		if err := rows.Scan(&s.CampaignID, &s.Name, &s.State, &s.DailyBudget,
			&s.TotalCost, &s.TotalSales, &s.TotalClicks, &s.Impressions,
			&s.ACoS, &s.ROAS, &s.CTR); err != nil { slog.Warn("扫描广告活动行失败", "error", err); continue }
		result = append(result, s)
	}
	return result, nil
}

// ── 广告关键词 & 定向分析 ──────────────────────────────────

// GetAdKeywords 获取关键词级别广告绩效（Top N）
func (a *App) GetAdKeywords(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]services.AdKeywordRow, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.dataSvc.GetAdKeywords(accountID, marketplaceID, dateStart, dateEnd, limit)
}

// GetAdTargets 获取 ASIN 定向广告绩效
func (a *App) GetAdTargets(accountID int64, marketplaceID, dateStart, dateEnd string, limit int) ([]services.AdTargetRow, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.dataSvc.GetAdTargets(accountID, marketplaceID, dateStart, dateEnd, limit)
}

// GetAdCampaignTrend 获取单个活动的每日趋势（图表钻取）
func (a *App) GetAdCampaignTrend(accountID int64, campaignID, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.dataSvc.GetAdCampaignDailyTrend(accountID, campaignID, dateStart, dateEnd)
}

// ── 财务数据 ──────────────────────────────────────────────

func (a *App) GetFinanceSummary(accountID int64, marketplaceID, dateStart, dateEnd string) (*services.FinanceSummary, error) {
	if a.financeSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.financeSvc.GetFinanceSummary(accountID, marketplaceID, dateStart, dateEnd)
}

func (a *App) GetSettlements(accountID int64) ([]services.SettlementSummary, error) {
	if a.financeSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.financeSvc.GetSettlements(accountID, 30)
}

// ── 预警数据 ──────────────────────────────────────────────

func (a *App) GetAlerts(accountID int64, unreadOnly bool) ([]services.Alert, error) {
	if a.alertsSvc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	return a.alertsSvc.GetAlerts(accountID, unreadOnly)
}

func (a *App) MarkAlertRead(alertID int64) error {
	if a.alertsSvc == nil {
		return fmt.Errorf("服务未初始化")
	}
	return a.alertsSvc.MarkAlertRead(alertID)
}

func (a *App) DismissAlert(alertID int64) error {
	if a.alertsSvc == nil {
		return fmt.Errorf("服务未初始化")
	}
	return a.alertsSvc.DismissAlert(alertID)
}

func (a *App) GetUnreadAlertCount(accountID int64) int {
	if a.alertsSvc == nil {
		return 0
	}
	return a.alertsSvc.CountUnreadAlerts(accountID)
}

// ── 数据同步触发 ──────────────────────────────────────────

// TriggerSync 前端手动触发数据同步
func (a *App) TriggerSync(accountID int64, marketplaceID, syncType string) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": ""}

	if a.db == nil {
		result["message"] = "数据库未初始化"
		return result
	}

	creds, err := a.loadCredentials(accountID)
	if err != nil || creds["lwa_client_id"] == "" {
		result["message"] = "请先在设置页面配置 API 凭证"
		return result
	}

	// 获取账户区域
	var region string
	a.db.QueryRow("SELECT region FROM accounts WHERE id=?", accountID).Scan(&region)

	tm := auth.NewLWATokenManager(creds["lwa_client_id"], creds["lwa_client_secret"], creds["refresh_token"])
	client := spapi.NewClient(tm, region, false)

	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Hour)
	defer cancel()

	var records int
	var syncErr error

	switch syncType {
	case "orders":
		svc := services.NewOrdersService(a.db, client)
		var mktIDs []string
		if marketplaceID != "" {
			mktIDs = []string{marketplaceID}
		}
		records, syncErr = svc.SyncOrders(ctx, accountID, mktIDs, false)

	case "inventory":
		svc := services.NewInventoryService(a.db, client)
		records, syncErr = svc.SyncFBAInventory(ctx, accountID, marketplaceID)

	case "datakiosk":
		dkClient := datakiosk.NewClient(tm, region, false)
		svc := services.NewDataKioskService(a.db, dkClient)
		end := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
		start := time.Now().UTC().AddDate(0, 0, -32).Format("2006-01-02")
		records, syncErr = svc.SyncSalesTraffic(ctx, accountID, marketplaceID, start, end)

	case "ads":
		adsClientID := creds["ads_client_id"]
		adsProfileID := creds["ads_profile_id"]
		adsAccountID := creds["ads_account_id"]
		if adsClientID == "" || adsProfileID == "" {
			result["message"] = "请先在设置页面配置广告 API 凭证（Client ID / Profile ID）"
			return result
		}
		adsClient := advertising.NewClient(tm, region, adsClientID, adsProfileID, adsAccountID)
		adsSvc := services.NewAdsService(a.db, adsClient)
		adEnd := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
		adStart := time.Now().UTC().AddDate(0, 0, -33).Format("2006-01-02")
		records, syncErr = adsSvc.SyncAdPerformance(ctx, accountID, marketplaceID, adStart, adEnd)

	case "pricing":
		pricingSvc := services.NewPricingService(a.db, client)
		records, syncErr = pricingSvc.SyncCompetitivePrices(ctx, accountID, marketplaceID)

	case "finance":
		financeSyncSvc := services.NewFinanceSyncService(a.db, client)
		finEnd := time.Now().UTC().Format("2006-01-02")
		finStart := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02")
		records, syncErr = financeSyncSvc.SyncFinancialEvents(ctx, accountID, marketplaceID, finStart, finEnd)

	default:
		result["message"] = "未知的同步类型: " + syncType
		return result
	}

	// 同步完成后运行预警检查
	if syncErr == nil {
		a.alertsSvc.RunAlertChecks(accountID, marketplaceID)
	}

	if syncErr != nil {
		result["message"] = syncErr.Error()
		return result
	}

	result["success"] = true
	result["records"] = records
	result["message"] = fmt.Sprintf("同步完成，共处理 %d 条记录", records)
	return result
}

// GetSyncHistory 获取同步历史
func (a *App) GetSyncHistory(accountID int64) ([]map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.alertsSvc.GetSyncHistory(accountID, 20)
}

// SaveProductCost 保存商品成本（用于利润计算）
func (a *App) SaveProductCost(accountID int64, sku, currency string, cost float64) error {
	if a.db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	_, err := a.db.Exec(`
		INSERT INTO product_costs(account_id,seller_sku,cost_currency,unit_cost,effective_from)
		VALUES(?,?,?,?,date('now'))
		ON CONFLICT(account_id,seller_sku,effective_from) DO UPDATE SET unit_cost=excluded.unit_cost
	`, accountID, sku, currency, cost)
	return err
}

// ── 工具函数 ──────────────────────────────────────────────

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return append(result, s[start:])
}

// ── CSV 成本导入 & 导出 ────────────────────────────────────

// ImportCostCSV 前端触发：批量导入商品成本 CSV
func (a *App) ImportCostCSV(accountID int64, csvContent, defaultCurrency string) services.CostImportResult {
	if a.db == nil {
		return services.CostImportResult{Errors: []string{"数据库未初始化"}}
	}
	return a.dataSvc.ImportCostCSV(accountID, csvContent, defaultCurrency)
}

// GetProductCosts 获取成本列表
func (a *App) GetProductCosts(accountID int64) ([]map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	return a.dataSvc.GetProductCosts(accountID)
}

// ExportDataCSV 导出指定类型的数据为 CSV 文件（保存到 exports 目录）
func (a *App) ExportDataCSV(accountID int64, marketplaceID, dataType, dateStart, dateEnd string) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": "", "path": ""}
	if a.db == nil {
		result["message"] = "数据库未初始化"
		return result
	}

	var csvContent string
	var err error
	switch dataType {
	case "sales":
		csvContent, err = a.dataSvc.ExportSalesCSV(accountID, marketplaceID, dateStart, dateEnd)
	case "inventory":
		csvContent, err = a.dataSvc.ExportInventoryCSV(accountID, marketplaceID)
	case "advertising":
		csvContent, err = a.dataSvc.ExportAdvertisingCSV(accountID, marketplaceID, dateStart, dateEnd)
	case "finance":
		csvContent, err = a.dataSvc.ExportFinanceCSV(accountID, marketplaceID, dateStart, dateEnd)
	default:
		result["message"] = fmt.Sprintf("未知导出类型: %s", dataType)
		return result
	}

	if err != nil {
		result["message"] = fmt.Sprintf("查询数据失败: %v", err)
		return result
	}
	if csvContent == "" {
		result["message"] = "暂无数据可导出"
		return result
	}

	// 写入到 exports 目录
	exportsDir := "."
	if a.cfg != nil { exportsDir = a.cfg.ExportsDir }
	os.MkdirAll(exportsDir, 0755)

	ts := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("voyage_%s_%s.csv", dataType, ts)
	filePath := filepath.Join(exportsDir, fileName)

	// 写入 BOM + CSV 内容（Excel 中文兼容）
	if err := os.WriteFile(filePath, []byte("\xEF\xBB\xBF"+csvContent), 0644); err != nil {
		result["message"] = fmt.Sprintf("写入文件失败: %v", err)
		return result
	}

	var sizeStr string
	if stat, err := os.Stat(filePath); err == nil {
		sizeStr = fmt.Sprintf("%.1f KB", float64(stat.Size())/1024)
	}

	slog.Info("CSV 导出完成", "path", filePath, "size", sizeStr)
	result["success"] = true
	result["path"] = filePath
	result["message"] = fmt.Sprintf("已导出: %s", fileName)
	result["fileSize"] = sizeStr
	return result
}

// OpenFileInExplorer 在系统文件管理器中定位并选中指定文件
func (a *App) OpenFileInExplorer(filePath string) map[string]interface{} {
	if filePath == "" {
		return map[string]interface{}{"success": false, "message": "文件路径为空"}
	}
	// Windows: explorer /select, "filepath"
	cmd := exec.Command("explorer", "/select,", filePath)
	if err := cmd.Start(); err != nil {
		slog.Warn("打开资源管理器失败", "path", filePath, "error", err)
		return map[string]interface{}{"success": false, "message": fmt.Sprintf("无法打开: %v", err)}
	}
	return map[string]interface{}{"success": true}
}

// GetReturnRateByASIN 查询 ASIN 退货率统计（近 N 天），数据延迟 T+1
func (a *App) GetReturnRateByASIN(accountID int64, marketplaceID string, days int) ([]services.ReturnRateByASIN, error) {
if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
return a.dataSvc.GetReturnRateByASIN(accountID, marketplaceID, days)
}

// GetReturnDetails 查询退货明细列表
func (a *App) GetReturnDetails(accountID int64, marketplaceID string, days int) ([]services.ReturnDetail, error) {
if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
return a.dataSvc.GetReturnDetails(accountID, marketplaceID, days)
}

// GetReturnReasonDistribution 获取退货原因分布统计
func (a *App) GetReturnReasonDistribution(accountID int64, marketplaceID string, days int) ([]services.ReturnReasonStat, error) {
if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
return a.dataSvc.GetReturnReasonDistribution(accountID, marketplaceID, days)
}

// GetInventoryAge 查询最新库龄快照，数据约每月 15 日更新
func (a *App) GetInventoryAge(accountID int64, marketplaceID string) (map[string]interface{}, error) {
if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
items, summary, err := a.dataSvc.GetInventoryAge(accountID, marketplaceID)
if err != nil { return nil, err }
return map[string]interface{}{"items": items, "summary": summary}, nil
}

// GetSearchTermStats 查询搜索词效果统计，数据延迟 T+3
func (a *App) GetSearchTermStats(accountID int64, dateStart, dateEnd string) ([]services.SearchTermStat, error) {
if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
return a.dataSvc.GetSearchTermStats(accountID, dateStart, dateEnd)
}
// GetAsinDailyTrend 获取 ASIN 历史每日销售趋势（商品详情趋势图用，Data Kiosk T+2）
func (a *App) GetAsinDailyTrend(accountID int64, marketplaceID, asin, dateStart, dateEnd string) ([]services.AsinDailyPoint, error) {
if a.dashSvc == nil { return nil, fmt.Errorf("服务未初始化") }
return a.dashSvc.GetAsinDailyTrend(accountID, marketplaceID, asin, dateStart, dateEnd)
}

// GetAsinFeeInfo 获取 ASIN 基于真实财务事件的实际费率信息（非固定比例估算）
func (a *App) GetAsinFeeInfo(accountID int64, marketplaceID, asin, dateStart, dateEnd string) (*services.AsinFeeInfo, error) {
if a.dashSvc == nil { return nil, fmt.Errorf("服务未初始化") }
return a.dashSvc.GetAsinFeeInfo(accountID, marketplaceID, asin, dateStart, dateEnd)
}

// ── P1: 竞品价格监控 ──────────────────────────────────────────

// GetCompetitivePrices 获取最新竞品价格（Product Pricing API，只读）
func (a *App) GetCompetitivePrices(accountID int64, marketplaceID string) ([]services.CompetitivePriceItem, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetCompetitivePrices(accountID, marketplaceID)
}

// ── P1: 补货决策引擎 ──────────────────────────────────────────

// GetReplenishmentAdvice 获取补货建议（默认头程 30 天，前端可传自定义值）
func (a *App) GetReplenishmentAdvice(accountID int64, marketplaceID string, leadTimeDays int) ([]services.ReplenishmentAdvice, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetReplenishmentAdvice(accountID, marketplaceID, leadTimeDays)
}

// UpdateReplenishmentConfig 更新单个 SKU 的补货参数
func (a *App) UpdateReplenishmentConfig(accountID int64, sku string, leadTimeDays, safetyDays, targetDays int) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return a.dataSvc.UpdateReplenishmentConfig(accountID, sku, leadTimeDays, safetyDays, targetDays)
}

// GetSeasonConfig 获取账户季度旺季系数配置
func (a *App) GetSeasonConfig(accountID int64) (*services.SeasonConfig, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return services.GetSeasonConfig(a.db, accountID)
}

// SaveSeasonConfig 保存账户季度旺季系数配置
func (a *App) SaveSeasonConfig(cfg services.SeasonConfig) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.SaveSeasonConfig(a.db, &cfg)
}

// UpdateSkuSeasonFactor 更新单个 SKU 旺季系数
func (a *App) UpdateSkuSeasonFactor(accountID int64, sku string, seasonFactor float64) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.UpdateSkuSeasonFactor(a.db, accountID, sku, seasonFactor)
}

// ApplyGlobalSeasonFactor 将旺季系数批量应用到所有 SKU
func (a *App) ApplyGlobalSeasonFactor(accountID int64, marketplaceID string, factor float64) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.ApplyGlobalSeasonFactor(a.db, accountID, marketplaceID, factor)
}

// GetPriceAlertConfig 获取价格预警阈值配置
func (a *App) GetPriceAlertConfig(accountID int64) *services.PriceAlertConfig {
	if a.db == nil { return &services.PriceAlertConfig{AccountID: accountID, PriceDropThreshold: 10, PriceSurgeThreshold: 15, BuyBoxCriticalHours: 24} }
	return services.GetPriceAlertConfig(a.db, accountID)
}

// SavePriceAlertConfig 保存价格预警阈值配置
func (a *App) SavePriceAlertConfig(cfg services.PriceAlertConfig) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.SavePriceAlertConfig(a.db, &cfg)
}



// ── P1: 汇率 + 多账户合并 ──────────────────────────────────────

// GetExchangeRates 获取当前汇率表
func (a *App) GetExchangeRates() ([]services.CurrencyRate, error) {
	if a.currencySvc == nil { return nil, fmt.Errorf("服务未初始化") }
	return a.currencySvc.GetAllRates()
}

// SyncExchangeRates 手动触发汇率同步
func (a *App) SyncExchangeRates() error {
	if a.currencySvc == nil { return fmt.Errorf("服务未初始化") }
	return a.currencySvc.SyncExchangeRates(a.ctx)
}

// GetCrossAccountKPI 获取多账户合并 KPI（以 CNY 为基准货币）
func (a *App) GetCrossAccountKPI(dateStart, dateEnd string) (*services.CrossAccountKPI, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetCrossAccountKPI(dateStart, dateEnd)
}

// ── P2: 广告 Placement 报告 ──────────────────────────────────

// GetAdPlacementStats 查询活动级版位效果（按 campaign × placement 分组）
func (a *App) GetAdPlacementStats(accountID int64, dateStart, dateEnd string) ([]services.PlacementRow, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetAdPlacementStats(accountID, dateStart, dateEnd)
}

// GetPlacementSummary 查询版位汇总（按 placement 类型分组，不区分活动）
func (a *App) GetPlacementSummary(accountID int64, dateStart, dateEnd string) ([]map[string]interface{}, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetPlacementSummary(accountID, dateStart, dateEnd)
}

// ── P2: Catalog 元数据补充 ──────────────────────────────────

// SyncCatalogMetadata 手动触发商品元数据同步（只读 Catalog Items API）
func (a *App) SyncCatalogMetadata(accountID int64, marketplaceID string) (int, error) {
	if a.db == nil || a.spapiClient == nil { return 0, fmt.Errorf("客户端未初始化，请先配置 API 凭证") }
	catalogSvc := services.NewCatalogService(a.db, a.spapiClient)
	return catalogSvc.SyncProductCatalog(a.ctx, accountID, marketplaceID)
}

// ── 数据库备份与恢复 ──────────────────────────────────────

// BackupDatabase 将当前数据库备份到指定路径
// 使用 SQLite 的 VACUUM INTO 生成一份干净的、去碎片的完整副本
func (a *App) BackupDatabase(targetPath string) map[string]interface{} {
	result := map[string]interface{}{"success": false, "message": ""}
	if a.db == nil {
		result["message"] = "数据库未初始化"
		return result
	}
	if targetPath == "" {
		// 默认备份到 backups 子目录下（带时间戳）
		ts := time.Now().Format("20060102_150405")
		targetPath = a.cfg.BackupsDir + "/voyage_backup_" + ts + ".db"
	}
	// 转义路径中的单引号，防止 SQL 注入（与 scheduler.go 保持一致）
	safePath := strings.ReplaceAll(targetPath, "'", "''")
	_, err := a.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", safePath))
	if err != nil {
		result["message"] = fmt.Sprintf("备份失败: %v", err)
		return result
	}
	result["success"] = true
	result["path"] = targetPath
	result["message"] = fmt.Sprintf("数据库已备份至: %s", targetPath)
	slog.Info("数据库备份完成", "path", targetPath)
	return result
}

// GetDatabaseInfo 获取数据库信息（路径、文件大小、表数量、各子目录路径）
func (a *App) GetDatabaseInfo() map[string]interface{} {
	info := map[string]interface{}{
		"dbPath": "",
		"sizeBytes": 0,
		"sizeMB": "0",
		"tableCount": 0,
		"dataDir": "",
		"reportsDir": "",
		"backupsDir": "",
		"exportsDir": "",
	}
	if a.cfg == nil {
		return info
	}
	info["dbPath"] = a.cfg.DBPath
	info["dataDir"] = a.cfg.DataDir
	info["reportsDir"] = a.cfg.ReportsDir
	info["backupsDir"] = a.cfg.BackupsDir
	info["exportsDir"] = a.cfg.ExportsDir

	// 文件大小
	if stat, err := os.Stat(a.cfg.DBPath); err == nil {
		bytes := stat.Size()
		info["sizeBytes"] = bytes
		info["sizeMB"] = fmt.Sprintf("%.2f", float64(bytes)/1024/1024)
	}

	// 表数量
	if a.db != nil {
		var cnt int
		a.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&cnt)
		info["tableCount"] = cnt
	}
	return info
}

// ── 3.2 PDF 周报 / 月报 ─────────────────────────────────────

// GeneratePDFReport 生成 PDF 经营报告（周报或月报）
func (a *App) GeneratePDFReport(accountID int64, marketplaceID string, reportType string) map[string]interface{} {
	if a.db == nil {
		return map[string]interface{}{"success": false, "message": "数据库未初始化"}
	}
	reportsDir := "."
	if a.cfg != nil { reportsDir = a.cfg.ReportsDir }

	rType := services.ReportWeekly
	if reportType == "monthly" { rType = services.ReportMonthly }

	result := services.GenerateReport(a.db, accountID, marketplaceID, reportsDir, rType)
	return map[string]interface{}{
		"success":  result.Success,
		"path":     result.Path,
		"message":  result.Message,
		"fileSize": result.FileSize,
	}
}

// ── 3.3 欧洲站 VAT 税率 ─────────────────────────────────────

// GetVATRates 获取所有欧洲站 VAT 税率（含自定义覆盖）
func (a *App) GetVATRates(accountID int64) []services.VATRate {
	if a.db == nil { return nil }
	return a.dataSvc.GetVATRates(accountID)
}

// SaveCustomVATRate 保存自定义 VAT 税率
func (a *App) SaveCustomVATRate(accountID int64, countryCode string, rate float64) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.SaveCustomVATRate(a.db, accountID, countryCode, rate)
}

// ResetVATRate 重置 VAT 税率为默认值
func (a *App) ResetVATRate(accountID int64, countryCode string) error {
	if a.db == nil { return fmt.Errorf("数据库未初始化") }
	return services.ResetVATRate(a.db, accountID, countryCode)
}

// GetFinanceVATBreakdown 获取含 VAT 拆分的财务数据
func (a *App) GetFinanceVATBreakdown(accountID int64, marketplaceID, dateStart, dateEnd string) map[string]interface{} {
	if a.db == nil { return map[string]interface{}{"isEU": false} }
	return a.dataSvc.GetFinanceSummaryWithVAT(accountID, marketplaceID, dateStart, dateEnd)
}

// ── 4.2 广告竞价建议 ─────────────────────────────────────────

// GetBidSuggestions 获取广告关键词竞价建议
func (a *App) GetBidSuggestions(accountID int64, marketplaceID, dateStart, dateEnd string, targetACoS float64) ([]services.BidSuggestion, error) {
	if a.db == nil { return nil, fmt.Errorf("数据库未初始化") }
	return a.dataSvc.GetBidSuggestions(accountID, marketplaceID, dateStart, dateEnd, targetACoS)
}

// ── 模拟数据管理 ─────────────────────────────────────────────

// GenerateMockData 生成模拟数据（用于演示和测试）
func (a *App) GenerateMockData() map[string]interface{} {
	if a.db == nil {
		return map[string]interface{}{"success": false, "message": "数据库未初始化"}
	}
	slog.Info("开始生成模拟数据...")
	result := services.GenerateMockData(a.db)
	if result["success"] == true {
		slog.Info("模拟数据生成完成", "message", result["message"])
	}
	return result
}

// ClearMockData 清空所有业务数据（用于重置演示环境）
func (a *App) ClearMockData() map[string]interface{} {
	if a.db == nil {
		return map[string]interface{}{"success": false, "message": "数据库未初始化"}
	}
	slog.Warn("开始清空所有业务数据...")
	result := services.ClearMockData(a.db)
	if result["success"] == true {
		slog.Info("业务数据已清空", "message", result["message"])
	}
	return result
}