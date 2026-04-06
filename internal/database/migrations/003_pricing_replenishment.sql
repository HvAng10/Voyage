-- 003_pricing_replenishment.sql
-- 竞品价格监控 + 补货引擎 + 汇率扩展

-- ============================================================
-- 竞品价格快照（Competitive Pricing API，只读）
-- ============================================================
CREATE TABLE IF NOT EXISTS competitive_prices (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id  TEXT    NOT NULL,
    asin            TEXT    NOT NULL,
    condition_type  TEXT    DEFAULT 'New',
    listing_price   REAL,           -- 上架价
    shipping_price  REAL    DEFAULT 0,
    landed_price    REAL,           -- 落地价 = listing + shipping
    buy_box_price   REAL,           -- Buy Box 价格
    is_buy_box_winner INTEGER DEFAULT 0,  -- 是否为 Buy Box 持有者
    number_of_offers INTEGER DEFAULT 0,   -- 竞争卖家数量
    snapshot_date   TEXT    NOT NULL,
    UNIQUE(account_id, marketplace_id, asin, snapshot_date)
);

CREATE INDEX IF NOT EXISTS idx_competitive_prices_asin ON competitive_prices(account_id, marketplace_id, asin, snapshot_date);

-- ============================================================
-- 补货参数配置（用户可手动设置头程周期等）
-- ============================================================
CREATE TABLE IF NOT EXISTS replenishment_config (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    seller_sku      TEXT    NOT NULL,
    lead_time_days  INTEGER NOT NULL DEFAULT 30,   -- 头程周期（天），默认 30
    safety_days     INTEGER NOT NULL DEFAULT 7,    -- 安全余量天数
    target_days     INTEGER NOT NULL DEFAULT 60,   -- 目标库存天数
    moq             INTEGER DEFAULT 0,             -- 最小起订量
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, seller_sku)
);

-- ============================================================
-- 汇率表（免费 API 每日自动更新，基准货币 CNY）
-- ============================================================
CREATE TABLE IF NOT EXISTS currency_rates (
    currency_code   TEXT    PRIMARY KEY,
    rate_to_cny     REAL    NOT NULL,       -- 1 单位该货币 = ? CNY
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- 预置主要货币汇率（初始值，会被 API 覆盖）
INSERT OR IGNORE INTO currency_rates VALUES ('CNY', 1.0, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('USD', 7.25, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('EUR', 7.90, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('GBP', 9.20, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('JPY', 0.048, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('CAD', 5.30, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('AUD', 4.70, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('MXN', 0.42, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('BRL', 1.40, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('INR', 0.087, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('SGD', 5.40, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('AED', 1.97, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('SAR', 1.93, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('SEK', 0.70, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('PLN', 1.80, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('TRY', 0.22, datetime('now'));
INSERT OR IGNORE INTO currency_rates VALUES ('EGP', 0.15, datetime('now'));

-- 扩展 sales_traffic_by_asin：增加 parent_asin 字段（变体分析用）
-- 注意：SQLite ALTER TABLE ADD COLUMN 只能加到末尾且不能加 UNIQUE 约束
-- 若表已存在则忽略错误
ALTER TABLE sales_traffic_by_asin ADD COLUMN parent_asin TEXT DEFAULT '';

INSERT OR IGNORE INTO schema_version(version) VALUES(3);
