-- 007: VAT 配置表 + 广告竞价建议历史表
-- 用于欧洲站 VAT 税率自定义覆盖和广告竞价建议存储

CREATE TABLE IF NOT EXISTS vat_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL,
    country_code TEXT NOT NULL,       -- DE/GB/FR/IT/ES/NL/SE/PL/BE
    custom_vat_rate REAL NOT NULL,    -- 用户自定义 VAT 税率（百分比）
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, country_code)
);

CREATE INDEX IF NOT EXISTS idx_vat_config_account ON vat_config(account_id);
