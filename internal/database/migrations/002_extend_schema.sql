-- 002_extend_schema.sql
-- 扩展 Schema：SB/SD 广告、退货数据、库龄数据、搜索词报告

-- ============================================================
-- FBA 客户退货数据（GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA）
-- 注：数据延迟约 T+1
-- ============================================================
CREATE TABLE IF NOT EXISTS fba_returns (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id      TEXT    NOT NULL,
    return_date         TEXT    NOT NULL,   -- YYYY-MM-DD
    order_id            TEXT,
    sku                 TEXT    NOT NULL,
    asin                TEXT,
    fnsku               TEXT,
    product_name        TEXT,
    quantity            INTEGER DEFAULT 1,
    fulfillment_center  TEXT,
    detailed_disposition TEXT,  -- SELLABLE / DAMAGED / CUSTOMER_DAMAGED 等
    reason              TEXT,   -- 买家退货原因代码
    status              TEXT,   -- Reimbursed / Unit returned to inventory 等
    license_plate_number TEXT,
    customer_comments   TEXT,   -- 买家备注（可能含关键词但无 PII）
    synced_at           TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_fba_returns_account ON fba_returns(account_id, marketplace_id, return_date);
CREATE INDEX IF NOT EXISTS idx_fba_returns_sku ON fba_returns(account_id, sku, return_date);

-- ============================================================
-- FBA 库龄快照（GET_FBA_INVENTORY_AGED_DATA）
-- 注：每月 15 日更新，T+1
-- ============================================================
CREATE TABLE IF NOT EXISTS fba_inventory_age (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id      TEXT    NOT NULL,
    snapshot_date       TEXT    NOT NULL,   -- 快照日期（每月）
    sku                 TEXT    NOT NULL,
    asin                TEXT,
    product_name        TEXT,
    condition           TEXT,
    qty_0_90_days       INTEGER DEFAULT 0,  -- 0-90 天库存
    qty_91_180_days     INTEGER DEFAULT 0,  -- 91-180 天库存
    qty_181_270_days    INTEGER DEFAULT 0,  -- 181-270 天库存
    qty_271_365_days    INTEGER DEFAULT 0,  -- 271-365 天库存
    qty_over_365_days   INTEGER DEFAULT 0,  -- 365 天以上库存（必征长期仓储费）
    est_ltsf            REAL    DEFAULT 0,  -- 预计长期仓储费（美元）
    currency            TEXT    DEFAULT 'USD',
    fnsku               TEXT,
    UNIQUE(account_id, marketplace_id, snapshot_date, sku)
);

CREATE INDEX IF NOT EXISTS idx_inv_age_account ON fba_inventory_age(account_id, marketplace_id, snapshot_date);
CREATE INDEX IF NOT EXISTS idx_inv_age_sku ON fba_inventory_age(account_id, sku);

-- ============================================================
-- 广告搜索词效果（SP Search Term Report）
-- 注：数据延迟约 T+3
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_search_terms (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id                  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date                        TEXT    NOT NULL,   -- 报告日期 YYYY-MM-DD
    campaign_id                 TEXT    NOT NULL,
    ad_group_id                 TEXT,
    keyword_id                  TEXT,
    keyword_text                TEXT,               -- 卖家自设关键词（投放词）
    search_term                 TEXT    NOT NULL,   -- 买家实际搜索词
    match_type                  TEXT,               -- BROAD / PHRASE / EXACT
    impressions                 INTEGER DEFAULT 0,
    clicks                      INTEGER DEFAULT 0,
    cost                        REAL    DEFAULT 0,
    purchases_7d                INTEGER DEFAULT 0,
    sales_7d                    REAL    DEFAULT 0,
    click_through_rate          REAL    DEFAULT 0,
    cost_per_click              REAL    DEFAULT 0,
    conversion_rate             REAL    DEFAULT 0,
    acos                        REAL    DEFAULT 0,
    UNIQUE(account_id, date, campaign_id, search_term, keyword_id)
);

CREATE INDEX IF NOT EXISTS idx_search_terms_account ON ad_search_terms(account_id, date);
CREATE INDEX IF NOT EXISTS idx_search_terms_term ON ad_search_terms(account_id, search_term);

-- 写入扩展 Schema 版本
INSERT OR IGNORE INTO schema_version(version) VALUES(2);
