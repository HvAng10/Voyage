-- 005_ad_keyword_target_tables.sql
-- 广告关键词/广告组/投放定向 Schema（ad_analytics.go 依赖）

-- ============================================================
-- 广告组表（Advertising API - spAdGroups）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_groups (
    ad_group_id     TEXT    NOT NULL,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    campaign_id     TEXT    NOT NULL,
    name            TEXT    NOT NULL DEFAULT '',
    state           TEXT    NOT NULL DEFAULT 'enabled',  -- enabled / paused / archived
    default_bid     REAL    DEFAULT 0,
    synced_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (ad_group_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_ad_groups_campaign ON ad_groups(campaign_id, account_id);

-- ============================================================
-- 广告关键词表（SP Keywords）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_keywords (
    keyword_id      TEXT    NOT NULL,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ad_group_id     TEXT    NOT NULL,
    keyword_text    TEXT    NOT NULL,
    match_type      TEXT    NOT NULL DEFAULT 'BROAD',    -- BROAD / PHRASE / EXACT
    state           TEXT    NOT NULL DEFAULT 'enabled',  -- enabled / paused / archived
    bid             REAL    DEFAULT 0,
    synced_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (keyword_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_ad_keywords_group ON ad_keywords(ad_group_id, account_id);

-- ============================================================
-- 关键词日效果数据（spKeywords 报告）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_keyword_performance (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword_id          TEXT    NOT NULL,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date                TEXT    NOT NULL,
    impressions         INTEGER DEFAULT 0,
    clicks              INTEGER DEFAULT 0,
    cost                REAL    DEFAULT 0,
    attributed_sales_7d REAL    DEFAULT 0,
    attributed_orders_7d INTEGER DEFAULT 0,
    UNIQUE(keyword_id, account_id, date)
);

CREATE INDEX IF NOT EXISTS idx_kw_perf_date ON ad_keyword_performance(account_id, date);

-- ============================================================
-- 商品投放定向表（SP Product Targeting）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_targets (
    target_id       TEXT    NOT NULL,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ad_group_id     TEXT    NOT NULL,
    target_type     TEXT    NOT NULL DEFAULT 'ASIN',     -- ASIN / Category
    target_value    TEXT    NOT NULL DEFAULT '',
    state           TEXT    NOT NULL DEFAULT 'enabled',  -- enabled / paused / archived
    bid             REAL    DEFAULT 0,
    synced_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (target_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_ad_targets_group ON ad_targets(ad_group_id, account_id);

-- ============================================================
-- 定向投放日效果数据（spTargets 报告）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_target_performance (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id           TEXT    NOT NULL,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date                TEXT    NOT NULL,
    impressions         INTEGER DEFAULT 0,
    clicks              INTEGER DEFAULT 0,
    cost                REAL    DEFAULT 0,
    attributed_sales_7d REAL    DEFAULT 0,
    UNIQUE(target_id, account_id, date)
);

CREATE INDEX IF NOT EXISTS idx_tgt_perf_date ON ad_target_performance(account_id, date);

INSERT OR IGNORE INTO schema_version(version) VALUES(5);
