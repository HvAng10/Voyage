-- 006: 补充关键查询索引 + 性能优化
-- 这些索引显著加速 Dashboard、Finance 和 Alerts 模块的日期范围筛选

-- 销售流量日期索引（Dashboard KPI / 趋势图高频查询）
CREATE INDEX IF NOT EXISTS idx_sales_traffic_daily_date
    ON sales_traffic_daily(account_id, marketplace_id, date);

-- ASIN 级销售日期索引（商品趋势图 / 补货引擎）
CREATE INDEX IF NOT EXISTS idx_sales_traffic_by_asin_date
    ON sales_traffic_by_asin(account_id, marketplace_id, asin, date);

-- 订单日期索引（COGS 利润计算 / 财务对账）
CREATE INDEX IF NOT EXISTS idx_orders_purchase_date
    ON orders(account_id, marketplace_id, purchase_date);

-- 财务事件日期索引（净利润 / 费率计算）
CREATE INDEX IF NOT EXISTS idx_financial_events_posted_date
    ON financial_events(account_id, marketplace_id, posted_date);

-- 广告效果日期索引（ACoS 趋势 / 环比计算）
CREATE INDEX IF NOT EXISTS idx_ad_perf_daily_date
    ON ad_performance_daily(account_id, date);

-- 预警去重检索索引
CREATE INDEX IF NOT EXISTS idx_alerts_dedup
    ON alerts(account_id, alert_type, related_entity_id, is_dismissed, created_at);

-- 竞品价格快照日期索引
CREATE INDEX IF NOT EXISTS idx_competitive_prices_snapshot
    ON competitive_prices(account_id, marketplace_id, snapshot_date);
