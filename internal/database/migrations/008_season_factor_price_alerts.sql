-- 008_season_factor_price_alerts.sql
-- 季节性补货系数 + 竞品价格历史快照（用于异动检测）

-- ============================================================
-- 1. 补货配置扩展：旺季系数
-- ============================================================
-- season_factor: 旺季系数（如 Q4 旺季设为 1.5，平时保持 1.0）
-- 补货建议中的日均销量会乘以该系数，从而提升建议量和预警阈值
ALTER TABLE replenishment_config ADD COLUMN season_factor REAL NOT NULL DEFAULT 1.0;

-- ============================================================
-- 2. 竞品价格历史快照（保留多天，用于计算波动幅度）
-- 原 competitive_prices 已有 snapshot_date 唯一约束，支持多天快照
-- 这里扩展两个字段用于异动检测
-- ============================================================
ALTER TABLE competitive_prices ADD COLUMN prev_buy_box_price REAL DEFAULT NULL;
ALTER TABLE competitive_prices ADD COLUMN price_change_pct REAL DEFAULT NULL;

-- ============================================================
-- 3. 统一旺季系数配置（账户级全局旺季系数，可被 SKU 级覆盖）
-- ============================================================
CREATE TABLE IF NOT EXISTS season_config (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    -- 季度系数（Q1~Q4）
    q1_factor       REAL    NOT NULL DEFAULT 1.0,
    q2_factor       REAL    NOT NULL DEFAULT 1.0,
    q3_factor       REAL    NOT NULL DEFAULT 1.1,  -- 小旺季
    q4_factor       REAL    NOT NULL DEFAULT 1.5,  -- Q4 旺季（Black Friday / 圣诞）
    -- 特殊促销系数（Prime Day 等）
    prime_day_factor REAL   NOT NULL DEFAULT 1.3,
    -- 是否启用自动季节系数（根据当前月份自动选取）
    auto_apply      INTEGER NOT NULL DEFAULT 0,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id)
);

-- ============================================================
-- 4. 价格预警配置（每账户配置异动阈值）
-- ============================================================
CREATE TABLE IF NOT EXISTS price_alert_config (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id              INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    -- 价格波动预警阈值（百分比）
    price_drop_threshold    REAL    NOT NULL DEFAULT 10.0,  -- 降价超过 10% 触发 warning
    price_surge_threshold   REAL    NOT NULL DEFAULT 15.0,  -- 涨价超过 15% 触发 info
    -- Buy Box 丢失持续升级阈值
    buybox_critical_hours   INTEGER NOT NULL DEFAULT 24,   -- 丢失 Buy Box 超过 N 小时升级为 critical
    updated_at              TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id)
);

-- 预置默认配置（在 Go 代码中按需 INSERT OR IGNORE）

INSERT OR IGNORE INTO schema_version(version) VALUES(8);
