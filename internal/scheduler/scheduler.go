// Package scheduler 实现后台定时同步任务调度器
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"voyage/internal/amazon/auth"
	"voyage/internal/amazon/advertising"
	"voyage/internal/amazon/datakiosk"
	"voyage/internal/amazon/spapi"
	"voyage/internal/database"
	"voyage/internal/services"
)

// SyncScheduler 定时同步调度器
type SyncScheduler struct {
	db        *database.DB
	alertsSvc *services.AlertsService
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool
	mu        sync.Mutex

	// 凭证加载回调（由 App 提供）
	credsLoader func(accountID int64) (map[string]string, error)
}

// New 创建调度器实例
func New(db *database.DB, alertsSvc *services.AlertsService, credsLoader func(accountID int64) (map[string]string, error)) *SyncScheduler {
	return &SyncScheduler{
		db:          db,
		alertsSvc:   alertsSvc,
		stopCh:      make(chan struct{}),
		credsLoader: credsLoader,
	}
}

// Start 启动后台调度器
func (s *SyncScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(ctx)
	slog.Info("数据同步调度器已启动")
}

// Stop 停止调度器
func (s *SyncScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	close(s.stopCh)
	s.wg.Wait()
	slog.Info("数据同步调度器已停止")
}

// run 主调度循环
func (s *SyncScheduler) run(ctx context.Context) {
	defer s.wg.Done()

	// 启动后等待 30 秒再执行首次同步（让主进程完全初始化）
	select {
	case <-time.After(30 * time.Second):
	case <-s.stopCh:
		return
	case <-ctx.Done():
		return
	}

	// 首次同步
	s.runAllJobs(ctx)

	// 订单同步：每 30 分钟一次（Amazon 限流要求）
	ordersTicker := time.NewTicker(30 * time.Minute)
	// Data Kiosk / 库存：每 6 小时一次
	dailyTicker := time.NewTicker(6 * time.Hour)
	// 广告同步：每 6 小时一次（广告数据 T+3）
	adsTicker := time.NewTicker(6 * time.Hour)
	// 竞品价格同步：每 4 小时一次（在 Rate Limit 承载范围内）
	pricingTicker := time.NewTicker(4 * time.Hour)
	// 汇率同步：每 24 小时一次
	currencyTicker := time.NewTicker(24 * time.Hour)
	// 数据清理：每 24 小时检查一次（实际每月 1 号执行）
	cleanupTicker := time.NewTicker(24 * time.Hour)
	// 自动备份：每 24 小时检查，实际每周日凌晨2点执行
	backupTicker := time.NewTicker(24 * time.Hour)

	defer ordersTicker.Stop()
	defer dailyTicker.Stop()
	defer adsTicker.Stop()
	defer pricingTicker.Stop()
	defer currencyTicker.Stop()
	defer cleanupTicker.Stop()
	defer backupTicker.Stop()

	for {
		select {
		case <-ordersTicker.C:
			s.runOrdersSync(ctx)
		case <-dailyTicker.C:
			s.runDailySync(ctx)
		case <-adsTicker.C:
			s.runAdsSync(ctx)
		case <-pricingTicker.C:
			s.runPricingSync(ctx)
		case <-currencyTicker.C:
			s.runCurrencySync(ctx)
		case <-cleanupTicker.C:
			s.runDataCleanup()
		case <-backupTicker.C:
			s.runAutoBackup()
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// runAllJobs 执行所有同步任务（启动时调用）
func (s *SyncScheduler) runAllJobs(ctx context.Context) {
	slog.Info("定时任务：开始全量同步")
	s.runOrdersSync(ctx)
	s.runDailySync(ctx)
	s.runAdsSync(ctx)
	s.runPricingSync(ctx)
}

// runOrdersSync 订单增量同步
func (s *SyncScheduler) runOrdersSync(ctx context.Context) {
	accounts, err := s.listActiveAccounts()
	if err != nil || len(accounts) == 0 {
		return
	}

	for _, acc := range accounts {
		func() {
			creds, err := s.credsLoader(acc.id)
			if err != nil || creds["lwa_client_id"] == "" {
				return // 没配置凭证，跳过
			}
			tm := auth.NewLWATokenManager(creds["lwa_client_id"], creds["lwa_client_secret"], creds["refresh_token"])
			client := spapi.NewClient(tm, acc.region, false)
			svc := services.NewOrdersService(s.db, client)

			jobCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
			defer cancel()

			records, err := svc.SyncOrders(jobCtx, acc.id, acc.marketplaceIDs, false)
			if err != nil {
				slog.Error("订单同步失败", "account", acc.id, "error", err)
			} else {
				slog.Info("订单同步完成", "account", acc.id, "records", records)
			}
		}()
	}
}

// runDailySync Data Kiosk + 库存同步（低频）
func (s *SyncScheduler) runDailySync(ctx context.Context) {
	accounts, err := s.listActiveAccounts()
	if err != nil || len(accounts) == 0 {
		return
	}

	for _, acc := range accounts {
		for _, mktID := range acc.marketplaceIDs {
			func() {
				creds, err := s.credsLoader(acc.id)
				if err != nil || creds["lwa_client_id"] == "" {
					return
				}
				tm := auth.NewLWATokenManager(creds["lwa_client_id"], creds["lwa_client_secret"], creds["refresh_token"])

				// Data Kiosk 同步
				dkClient := datakiosk.NewClient(tm, acc.region, false)
				dkSvc := services.NewDataKioskService(s.db, dkClient)
				end := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
				start := time.Now().UTC().AddDate(0, 0, -32).Format("2006-01-02")

				jobCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
				defer cancel()

				_, err = dkSvc.SyncSalesTraffic(jobCtx, acc.id, mktID, start, end)
				if err != nil {
					slog.Error("Data Kiosk 同步失败", "account", acc.id, "mkt", mktID, "error", err)
				}

				// FBA 库存同步
				spClient := spapi.NewClient(tm, acc.region, false)
				invSvc := services.NewInventoryService(s.db, spClient)
				invCtx, invCancel := context.WithTimeout(ctx, 15*time.Minute)
				defer invCancel()

				_, err = invSvc.SyncFBAInventory(invCtx, acc.id, mktID)
				if err != nil {
					slog.Error("库存同步失败", "account", acc.id, "mkt", mktID, "error", err)
				}

				// 预警规则检查
				alertSvc := services.NewAlertsService(s.db)
				alertSvc.RunAlertChecks(acc.id, mktID)
			}()
		}
	}
}

// runAdsSync 广告数据定时同步（SP/SB/SD 广告报告）
// 数据延迟 T+3：同步窗口为 today-33天到 today-3天
func (s *SyncScheduler) runAdsSync(ctx context.Context) {
	accounts, err := s.listActiveAccounts()
	if err != nil || len(accounts) == 0 {
		return
	}

	// 算汀广告延迟 3 天后的安全窗口
	adEnd := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
	adStart := time.Now().UTC().AddDate(0, 0, -33).Format("2006-01-02")

	for _, acc := range accounts {
		for _, mktID := range acc.marketplaceIDs {
			func() {
				creds, err := s.credsLoader(acc.id)
				if err != nil || creds["ads_client_id"] == "" {
					return // 没有广告凭证配置，跳过
				}

				tm := auth.NewLWATokenManager(
					creds["lwa_client_id"], creds["lwa_client_secret"], creds["refresh_token"],
				)
				adsClient := advertising.NewClient(
					tm, acc.region,
					creds["ads_client_id"],
					creds["ads_profile_id"],
					creds["ads_account_id"],
				)

				adsSvc := services.NewAdsService(s.db, adsClient)

				jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
				defer cancel()

				// SP 广告性效
				if _, err := adsSvc.SyncAdPerformance(jobCtx, acc.id, mktID, adStart, adEnd); err != nil {
					slog.Error("SP 广告同步失败", "account", acc.id, "mkt", mktID, "error", err)
				} else {
					slog.Info("SP 广告同步完成", "account", acc.id, "range", adStart+"~"+adEnd)
				}
			}()
		}
	}
}

// accountInfo 简化的账户信息
type accountInfo struct {
	id             int64
	region         string
	marketplaceIDs []string
}

// listActiveAccounts 查询所有有效账户
func (s *SyncScheduler) listActiveAccounts() ([]accountInfo, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.region, GROUP_CONCAT(am.marketplace_id, ',')
		FROM accounts a
		LEFT JOIN account_marketplaces am ON a.id=am.account_id
		WHERE a.is_active=1
		GROUP BY a.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []accountInfo
	for rows.Next() {
		var acc accountInfo
		var mktStr *string
		if err := rows.Scan(&acc.id, &acc.region, &mktStr); err != nil { slog.Warn("扫描调度账户行失败", "error", err); continue }
		if mktStr != nil && *mktStr != "" {
			acc.marketplaceIDs = splitStr(*mktStr, ',')
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func splitStr(s string, sep byte) []string {
	if s == "" {
		return nil
	}
	var res []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			res = append(res, s[start:i])
			start = i + 1
		}
	}
	return append(res, s[start:])
}

// runPricingSync 竞品价格同步（每 4 小时）
func (s *SyncScheduler) runPricingSync(ctx context.Context) {
	accounts, err := s.listActiveAccounts()
	if err != nil || len(accounts) == 0 {
		return
	}

	for _, acc := range accounts {
		creds, err := s.credsLoader(acc.id)
		if err != nil {
			continue
		}

		clientID := creds["lwa_client_id"]
		clientSecret := creds["lwa_client_secret"]
		refreshToken := creds["refresh_token"]
		if clientID == "" || refreshToken == "" {
			continue
		}

		tokenMgr := auth.NewLWATokenManager(clientID, clientSecret, refreshToken)
		client := spapi.NewClient(tokenMgr, acc.region, false)
		pricingSvc := services.NewPricingService(s.db, client)

		for _, mktID := range acc.marketplaceIDs {
			if n, err := pricingSvc.SyncCompetitivePrices(ctx, acc.id, mktID); err != nil {
				slog.Error("竞品价格同步失败", "account", acc.id, "mkt", mktID, "error", err)
			} else {
				slog.Info("竞品价格同步完成", "account", acc.id, "mkt", mktID, "synced", n)
			}
		}
	}
}

// runCurrencySync 汇率同步（每 24 小时）
func (s *SyncScheduler) runCurrencySync(ctx context.Context) {
	currSvc := services.NewCurrencyService(s.db)
	if err := currSvc.SyncExchangeRates(ctx); err != nil {
		slog.Error("汇率同步失败", "error", err)
	} else {
		slog.Info("汇率同步完成")
	}
}

// runDataCleanup 过期数据清理（每月13 号执行一次）
func (s *SyncScheduler) runDataCleanup() {
	// 只在每月 1 号执行实际清理
	if time.Now().Day() != 1 {
		return
	}
	slog.Info("执行月度数据清理")
	s.alertsSvc.CleanStaleData()
}

// runAutoBackup 自动数据库备份（每周日凌晨2点，保留最近4份）
func (s *SyncScheduler) runAutoBackup() {
	now := time.Now()
	// 只在周日执行
	if now.Weekday() != time.Sunday {
		return
	}
	// 检查时间窗口：凌晨1:30 ~ 2:30 之间
	hour := now.Hour()
	if hour < 1 || hour > 2 {
		return
	}

	slog.Info("自动备份：开始执行")

	// 确定备份目录：基于数据库文件所在目录
	dbPath := s.db.Path()
	var dataDir string
	if dbPath != "" {
		dataDir = filepath.Dir(dbPath)
	} else {
		dataDir = "."
	}
	backupDir := filepath.Join(dataDir, "backups")
	os.MkdirAll(backupDir, 0755)

	// 备份文件名
	ts := now.Format("20060102_150405")
	targetPath := filepath.Join(backupDir, fmt.Sprintf("voyage_backup_%s.db", ts))

	_, err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(targetPath, "'", "''")))
	if err != nil {
		slog.Error("自动备份失败", "error", err)
		return
	}

	// 获取文件大小
	var sizeStr string
	if stat, err := os.Stat(targetPath); err == nil {
		sizeStr = fmt.Sprintf("%.2f MB", float64(stat.Size())/1024/1024)
	}
	slog.Info("自动备份完成", "path", targetPath, "size", sizeStr)

	// 清理旧备份：保留最近 4 份
	s.cleanOldBackups(backupDir, 4)
}

// cleanOldBackups 保留最新的 keep 份，删除更早的备份文件
func (s *SyncScheduler) cleanOldBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "voyage_backup_") && strings.HasSuffix(e.Name(), ".db") {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) <= keep {
		return
	}

	// 按文件名排序（日期包含在文件名中）
	sort.Strings(backups)

	// 删除最旧的
	toDelete := backups[:len(backups)-keep]
	for _, name := range toDelete {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil {
			slog.Warn("删除旧备份失败", "file", name, "error", err)
		} else {
			slog.Info("已删除旧备份", "file", name)
		}
	}
}
