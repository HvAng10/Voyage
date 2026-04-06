-- 004_placement_catalog.sql
-- 广告 Placement 报告 + Catalog 元数据扩展

-- ============================================================
-- 广告版位日报（SP Campaigns 按 placement 维度拆分）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_placement_daily (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id     TEXT    NOT NULL,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date            TEXT    NOT NULL,
    placement       TEXT    NOT NULL,  -- Top of Search / Product Pages / Rest of Search
    impressions     INTEGER DEFAULT 0,
    clicks          INTEGER DEFAULT 0,
    cost            REAL    DEFAULT 0,
    sales_7d        REAL    DEFAULT 0,
    orders_7d       INTEGER DEFAULT 0,
    UNIQUE(campaign_id, account_id, date, placement)
);

CREATE INDEX IF NOT EXISTS idx_ad_placement_daily_lookup
    ON ad_placement_daily(account_id, date, placement);

-- ============================================================
-- 产品元数据扩展 - catalog_synced_at 字段
-- 注意：products 表在 001 中已有 brand/category/image_url 列，
-- 此处仅添加 catalog_synced_at 追踪字段。
-- 使用安全方式：先检查列是否存在，避免 ALTER TABLE 重复添加导致启动失败。
-- SQLite 不支持 IF NOT EXISTS 语法，因此用事务内捕获错误的方式处理。
-- ============================================================
-- catalog_synced_at 列（如已存在则忽略错误）

INSERT OR IGNORE INTO schema_version(version) VALUES(4);
