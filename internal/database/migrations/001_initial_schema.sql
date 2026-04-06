-- 001_initial_schema.sql
-- Voyage 初始数据库 Schema
-- 注意：PRAGMA 语句由 Go 代码在数据库打开时设置，不在迁移事务中执行

-- ============================================================
-- Schema 版本跟踪
-- ============================================================
CREATE TABLE IF NOT EXISTS schema_version (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- 站点 Marketplace 参考表（预置所有已知站点）
-- ============================================================
CREATE TABLE IF NOT EXISTS marketplace (
    marketplace_id  TEXT PRIMARY KEY,
    country_code    TEXT NOT NULL,
    name            TEXT NOT NULL,
    currency_code   TEXT NOT NULL,
    region          TEXT NOT NULL,  -- na / eu / fe
    timezone        TEXT NOT NULL
);

-- ============================================================
-- 店铺账户表（支持多店铺）
-- ============================================================
CREATE TABLE IF NOT EXISTS accounts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,       -- 用户自定义别名
    seller_id   TEXT    NOT NULL,       -- Amazon Seller ID
    region      TEXT    NOT NULL,       -- na / eu / fe
    is_active   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- 账户与站点的关联关系（一个账户可运营多个站点）
CREATE TABLE IF NOT EXISTS account_marketplaces (
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id  TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    PRIMARY KEY (account_id, marketplace_id)
);

-- 账户 API 凭证（AES-256-GCM 加密后存储）
CREATE TABLE IF NOT EXISTS account_credentials (
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    credential_type TEXT    NOT NULL,   -- lwa_client_id / lwa_client_secret / refresh_token / ads_client_id / ads_client_secret / ads_profile_id
    encrypted_value BLOB    NOT NULL,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (account_id, credential_type)
);

-- ============================================================
-- 订单表（Orders API v2026-01-01）
-- ============================================================
CREATE TABLE IF NOT EXISTS orders (
    amazon_order_id         TEXT    PRIMARY KEY,
    account_id              INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id          TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    order_status            TEXT    NOT NULL,   -- Pending / Unshipped / Shipped / Canceled / etc.
    fulfillment_channel     TEXT,               -- AFN（FBA）/ MFN（自发货）
    sales_channel           TEXT,
    order_total             REAL,
    currency_code           TEXT,
    purchase_date           TEXT    NOT NULL,   -- UTC ISO8601
    last_update_date        TEXT,               -- UTC ISO8601
    ship_service_level      TEXT,
    item_count              INTEGER DEFAULT 0,
    is_business_order       INTEGER DEFAULT 0,
    is_prime                INTEGER DEFAULT 0,
    ship_city               TEXT,
    ship_state              TEXT,
    ship_country            TEXT,
    ship_postal_code        TEXT,
    synced_at               TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_orders_account_marketplace ON orders(account_id, marketplace_id);
CREATE INDEX IF NOT EXISTS idx_orders_purchase_date ON orders(purchase_date);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(order_status);

-- 订单明细行
CREATE TABLE IF NOT EXISTS order_items (
    order_item_id       TEXT    PRIMARY KEY,
    amazon_order_id     TEXT    NOT NULL REFERENCES orders(amazon_order_id) ON DELETE CASCADE,
    asin                TEXT,
    seller_sku          TEXT,
    title               TEXT,
    quantity_ordered    INTEGER NOT NULL DEFAULT 0,
    quantity_shipped    INTEGER DEFAULT 0,
    item_price          REAL    DEFAULT 0,
    item_tax            REAL    DEFAULT 0,
    shipping_price      REAL    DEFAULT 0,
    shipping_tax        REAL    DEFAULT 0,
    gift_wrap_price     REAL    DEFAULT 0,
    promotion_discount  REAL    DEFAULT 0,
    cod_fee             REAL    DEFAULT 0,
    condition_id        TEXT
);

CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(amazon_order_id);
CREATE INDEX IF NOT EXISTS idx_order_items_asin ON order_items(asin);

-- ============================================================
-- 商品/Listing 表（Catalog Items API v2022-04-01）
-- ============================================================
CREATE TABLE IF NOT EXISTS products (
    asin            TEXT    NOT NULL,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id  TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    seller_sku      TEXT,
    title           TEXT,
    brand           TEXT,
    category        TEXT,
    image_url       TEXT,
    your_price      REAL,
    currency_code   TEXT,
    listing_status  TEXT,   -- Active / Inactive / Incomplete
    fulfillment     TEXT,   -- AFN / MFN
    open_date       TEXT,
    synced_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (asin, account_id, marketplace_id)
);

CREATE INDEX IF NOT EXISTS idx_products_account ON products(account_id, marketplace_id);

-- ============================================================
-- FBA 库存快照（GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA 报告）
-- ============================================================
CREATE TABLE IF NOT EXISTS inventory_snapshots (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id      TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    seller_sku          TEXT    NOT NULL,
    asin                TEXT,
    fnsku               TEXT,
    condition_type      TEXT,
    fulfillable_qty     INTEGER DEFAULT 0,
    unsellable_qty      INTEGER DEFAULT 0,
    reserved_qty        INTEGER DEFAULT 0,
    inbound_qty         INTEGER DEFAULT 0,
    researching_qty     INTEGER DEFAULT 0,
    unfulfillable_qty   INTEGER DEFAULT 0,
    snapshot_date       TEXT    NOT NULL    -- UTC date YYYY-MM-DD
);

CREATE INDEX IF NOT EXISTS idx_inventory_account ON inventory_snapshots(account_id, marketplace_id, snapshot_date);
CREATE INDEX IF NOT EXISTS idx_inventory_sku ON inventory_snapshots(seller_sku, snapshot_date);

-- ============================================================
-- Data Kiosk - 日销售与流量（Analytics_SalesAndTraffic）
-- ============================================================
CREATE TABLE IF NOT EXISTS sales_traffic_daily (
    id                              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id                      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id                  TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    date                            TEXT    NOT NULL,   -- YYYY-MM-DD（店铺本地日期）
    ordered_product_sales           REAL    DEFAULT 0,
    ordered_product_sales_b2b       REAL    DEFAULT 0,
    units_ordered                   INTEGER DEFAULT 0,
    units_ordered_b2b               INTEGER DEFAULT 0,
    total_order_items               INTEGER DEFAULT 0,
    total_order_items_b2b           INTEGER DEFAULT 0,
    average_selling_price           REAL    DEFAULT 0,
    page_views                      INTEGER DEFAULT 0,
    page_views_b2b                  INTEGER DEFAULT 0,
    sessions                        INTEGER DEFAULT 0,
    sessions_b2b                    INTEGER DEFAULT 0,
    browser_sessions                INTEGER DEFAULT 0,
    mobile_app_sessions             INTEGER DEFAULT 0,
    unit_session_percentage         REAL    DEFAULT 0,
    order_item_session_percentage   REAL    DEFAULT 0,
    buy_box_percentage              REAL    DEFAULT 0,
    average_offer_count             REAL    DEFAULT 0,
    average_parent_items            REAL    DEFAULT 0,
    feedback_received               INTEGER DEFAULT 0,
    negative_feedback_received      INTEGER DEFAULT 0,
    received_negative_feedback_rate REAL    DEFAULT 0,
    UNIQUE(account_id, marketplace_id, date)
);

CREATE INDEX IF NOT EXISTS idx_stt_daily ON sales_traffic_daily(account_id, marketplace_id, date);

-- 按 ASIN 维度的销售与流量
CREATE TABLE IF NOT EXISTS sales_traffic_by_asin (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id                  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id              TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    asin                        TEXT    NOT NULL,
    date                        TEXT    NOT NULL,   -- YYYY-MM-DD
    ordered_product_sales       REAL    DEFAULT 0,
    units_ordered               INTEGER DEFAULT 0,
    total_order_items           INTEGER DEFAULT 0,
    page_views                  INTEGER DEFAULT 0,
    sessions                    INTEGER DEFAULT 0,
    unit_session_percentage     REAL    DEFAULT 0,
    buy_box_percentage          REAL    DEFAULT 0,
    UNIQUE(account_id, marketplace_id, asin, date)
);

CREATE INDEX IF NOT EXISTS idx_stt_asin ON sales_traffic_by_asin(account_id, marketplace_id, asin, date);

-- ============================================================
-- 财务事件（Finances API v2024-06-19）
-- ============================================================
CREATE TABLE IF NOT EXISTS financial_events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id      TEXT,
    amazon_order_id     TEXT,
    event_type          TEXT    NOT NULL,   -- Order / Refund / ServiceFee / Adjustment / etc.
    posted_date         TEXT    NOT NULL,   -- UTC ISO8601
    principal_amount    REAL    DEFAULT 0,
    tax_amount          REAL    DEFAULT 0,
    marketplace_fee     REAL    DEFAULT 0,  -- 平台佣金
    fba_fee             REAL    DEFAULT 0,  -- FBA 固定费用
    variable_closing    REAL    DEFAULT 0,
    other_fee           REAL    DEFAULT 0,
    total_amount        REAL    DEFAULT 0,
    currency_code       TEXT,
    description         TEXT,
    synced_at           TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_financial_account ON financial_events(account_id, posted_date);
CREATE INDEX IF NOT EXISTS idx_financial_order ON financial_events(amazon_order_id);

-- 结算报告（Settlement Reports）
CREATE TABLE IF NOT EXISTS settlement_reports (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id              INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    settlement_id           TEXT    NOT NULL,
    marketplace_id          TEXT,
    settlement_start_date   TEXT,
    settlement_end_date     TEXT,
    deposit_date            TEXT,
    total_amount            REAL    DEFAULT 0,
    currency_code           TEXT,
    report_id               TEXT,
    synced_at               TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, settlement_id)
);

CREATE TABLE IF NOT EXISTS settlement_items (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    settlement_report_id    INTEGER NOT NULL REFERENCES settlement_reports(id) ON DELETE CASCADE,
    settlement_id           TEXT,
    amazon_order_id         TEXT,
    merchant_order_id       TEXT,
    transaction_type        TEXT,   -- Order / Refund / Adjustment / Transfer / etc.
    order_item_code         TEXT,
    sku                     TEXT,
    quantity                INTEGER,
    marketplace_name        TEXT,
    amount_type             TEXT,
    amount_description      TEXT,
    amount                  REAL    DEFAULT 0,
    fulfillment_id          TEXT,
    posted_date             TEXT,
    merchant_order_item_id  TEXT
);

CREATE INDEX IF NOT EXISTS idx_settlement_items_report ON settlement_items(settlement_report_id);

-- ============================================================
-- 广告活动（Advertising API v3）
-- ============================================================
CREATE TABLE IF NOT EXISTS ad_campaigns (
    campaign_id         TEXT    NOT NULL,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ads_profile_id      TEXT    NOT NULL,   -- Amazon Advertising Profile ID
    marketplace_id      TEXT,
    name                TEXT    NOT NULL,
    campaign_type       TEXT    NOT NULL,   -- sponsoredProducts / sponsoredBrands / sponsoredDisplay
    targeting_type      TEXT,               -- manual / auto
    state               TEXT    NOT NULL,   -- enabled / paused / archived
    daily_budget        REAL    DEFAULT 0,
    budget_type         TEXT,               -- daily / monthly
    start_date          TEXT,
    end_date            TEXT,
    synced_at           TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (campaign_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_campaigns_account ON ad_campaigns(account_id);

-- 广告日效果数据
CREATE TABLE IF NOT EXISTS ad_performance_daily (
    id                              INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id                     TEXT    NOT NULL,
    account_id                      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date                            TEXT    NOT NULL,   -- YYYY-MM-DD
    impressions                     INTEGER DEFAULT 0,
    clicks                          INTEGER DEFAULT 0,
    cost                            REAL    DEFAULT 0,
    attributed_sales_7d             REAL    DEFAULT 0,
    attributed_sales_14d            REAL    DEFAULT 0,
    attributed_sales_30d            REAL    DEFAULT 0,
    attributed_conversions_7d       INTEGER DEFAULT 0,
    attributed_conversions_14d      INTEGER DEFAULT 0,
    attributed_conversions_30d      INTEGER DEFAULT 0,
    attributed_units_ordered_7d     INTEGER DEFAULT 0,
    click_through_rate              REAL    DEFAULT 0,
    cost_per_click                  REAL    DEFAULT 0,
    acos                            REAL    DEFAULT 0,  -- 广告成本销售比
    roas                            REAL    DEFAULT 0,  -- 广告投资回报率
    UNIQUE(campaign_id, account_id, date)
);

CREATE INDEX IF NOT EXISTS idx_ad_perf_account ON ad_performance_daily(account_id, date);
CREATE INDEX IF NOT EXISTS idx_ad_perf_campaign ON ad_performance_daily(campaign_id, date);

-- ============================================================
-- Data Kiosk - 经营经济数据（Analytics_Economics）
-- ============================================================
CREATE TABLE IF NOT EXISTS economics_daily (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id      TEXT    NOT NULL REFERENCES marketplace(marketplace_id),
    date                TEXT    NOT NULL,   -- YYYY-MM-DD
    ordered_revenue     REAL    DEFAULT 0,
    shipped_revenue     REAL    DEFAULT 0,
    advertising_spend   REAL    DEFAULT 0,
    fba_fees            REAL    DEFAULT 0,
    referral_fees       REAL    DEFAULT 0,
    other_fees          REAL    DEFAULT 0,
    refunds             REAL    DEFAULT 0,
    cogs                REAL    DEFAULT 0,  -- 商品成本（用户录入）
    net_proceeds        REAL    DEFAULT 0,  -- 净收益（自动计算）
    UNIQUE(account_id, marketplace_id, date)
);

CREATE INDEX IF NOT EXISTS idx_economics_account ON economics_daily(account_id, marketplace_id, date);

-- ============================================================
-- 商品成本表（用户手动录入/CSV 导入）
-- ============================================================
CREATE TABLE IF NOT EXISTS product_costs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    seller_sku      TEXT    NOT NULL,
    asin            TEXT,
    cost_currency   TEXT    NOT NULL DEFAULT 'USD',
    unit_cost       REAL    NOT NULL DEFAULT 0,      -- 单件采购成本
    effective_from  TEXT    NOT NULL DEFAULT (date('now')),
    notes           TEXT,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, seller_sku, effective_from)
);

-- ============================================================
-- 预警记录
-- ============================================================
CREATE TABLE IF NOT EXISTS alerts (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id              INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    alert_type              TEXT    NOT NULL,   -- low_inventory / high_acos / sales_drop / listing_inactive / etc.
    severity                TEXT    NOT NULL,   -- critical / warning / info
    title                   TEXT    NOT NULL,
    message                 TEXT,
    related_entity_type     TEXT,               -- order / product / campaign
    related_entity_id       TEXT,
    is_read                 INTEGER NOT NULL DEFAULT 0,
    is_dismissed            INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_alerts_account ON alerts(account_id, is_read, created_at);

-- ============================================================
-- 预警规则配置
-- ============================================================
CREATE TABLE IF NOT EXISTS alert_rules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    rule_type       TEXT    NOT NULL,   -- low_inventory / high_acos / sales_drop / etc.
    is_enabled      INTEGER NOT NULL DEFAULT 1,
    threshold_value REAL,               -- 阈值（如库存天数、ACoS 百分比）
    notification    INTEGER NOT NULL DEFAULT 1, -- 是否创建通知
    parameters      TEXT,               -- JSON 格式的附加参数
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, rule_type)
);

-- ============================================================
-- 数据同步日志
-- ============================================================
CREATE TABLE IF NOT EXISTS sync_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    marketplace_id  TEXT,
    sync_type       TEXT    NOT NULL,   -- orders / inventory / finances / data_kiosk / advertising / economics
    status          TEXT    NOT NULL,   -- running / success / failed
    started_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    completed_at    TEXT,
    records_synced  INTEGER DEFAULT 0,
    error_message   TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_log_account ON sync_log(account_id, sync_type, started_at);

-- ============================================================
-- 预置 Marketplace 数据（官方来源：https://developer-docs.amazon.com/sp-api/docs/marketplace-ids）
-- ============================================================
INSERT OR IGNORE INTO marketplace VALUES
    -- 北美
    ('ATVPDKIKX0DER', 'US', '美国', 'USD', 'na', 'America/New_York'),
    ('A2EUQ1WTGCTBG2', 'CA', '加拿大', 'CAD', 'na', 'America/Toronto'),
    ('A1AM78C64UM0Y8', 'MX', '墨西哥', 'MXN', 'na', 'America/Mexico_City'),
    ('A2Q3Y263D00KWC', 'BR', '巴西', 'BRL', 'na', 'America/Sao_Paulo'),
    -- 欧洲
    ('A1PA6795UKMFR9', 'DE', '德国', 'EUR', 'eu', 'Europe/Berlin'),
    ('A1F83G8C2ARO7P', 'GB', '英国', 'GBP', 'eu', 'Europe/London'),
    ('A13V1IB3VIYZZH', 'FR', '法国', 'EUR', 'eu', 'Europe/Paris'),
    ('APJ6JRA9NG5V4',  'IT', '意大利', 'EUR', 'eu', 'Europe/Rome'),
    ('A1RKKUPIHCS9HS', 'ES', '西班牙', 'EUR', 'eu', 'Europe/Madrid'),
    ('A1805IZSGTT6HS', 'NL', '荷兰', 'EUR', 'eu', 'Europe/Amsterdam'),
    ('A2NODRKZP88ZB9', 'SE', '瑞典', 'SEK', 'eu', 'Europe/Stockholm'),
    ('A1C3SOZRARQ6R3', 'PL', '波兰', 'PLN', 'eu', 'Europe/Warsaw'),
    ('A33AVAJ2PDY3EV', 'TR', '土耳其', 'TRY', 'eu', 'Europe/Istanbul'),
    ('A2VIGQ35RCS4UG', 'AE', '阿联酋', 'AED', 'eu', 'Asia/Dubai'),
    ('A17E79C6D8DWNP', 'SA', '沙特阿拉伯', 'SAR', 'eu', 'Asia/Riyadh'),
    ('AIDIKKJX355ZZ',  'EG', '埃及', 'EGP', 'eu', 'Africa/Cairo'),
    ('A21TJRUUN4KGV',  'IN', '印度', 'INR', 'eu', 'Asia/Kolkata'),
    ('A1ZL9W1JXURPQ5', 'BE', '比利时', 'EUR', 'eu', 'Europe/Brussels'),
    -- 远东
    ('A1VC38T7YXB528', 'JP', '日本', 'JPY', 'fe', 'Asia/Tokyo'),
    ('A39IBJ37TRP1C6', 'AU', '澳大利亚', 'AUD', 'fe', 'Australia/Sydney'),
    ('A19VAU5U5O7RUS', 'SG', '新加坡', 'SGD', 'fe', 'Asia/Singapore');

-- 写入初始 Schema 版本
INSERT OR IGNORE INTO schema_version(version) VALUES(1);
