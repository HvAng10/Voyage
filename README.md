<div align="center">
  <img src="banner.png" alt="Voyage Logo" width="128" />
  <h1>Voyage (远航) v2.4.0</h1>
  <p><b>✨ 基于 Wails 的生产级跨平台本地化亚马逊运营数据分析工具 ✨</b></p>
  <p>
    <img src="https://img.shields.io/badge/version-v2.4.0-7c3aed?style=flat-square" />
    <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" />
    <img src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react" />
    <img src="https://img.shields.io/badge/Wails-v2-8B5CF6?style=flat-square" />
    <img src="https://img.shields.io/badge/SQLite-WAL-003B57?style=flat-square&logo=sqlite" />
    <img src="https://img.shields.io/badge/Marketplaces-21-c9a84c?style=flat-square" />
    <img src="https://img.shields.io/badge/License-Private-gray?style=flat-square" />
  </p>
</div>

---

## 📌 项目简介

**Voyage (远航)** 是一款专为 5 人左右小型精品亚马逊跨境电商团队设计的 **私有化数据罗盘**。采用 **Go + React + Wails** 全栈桌面架构，原生对接 Amazon **Selling Partner API (SP-API)** 与 **Advertising API**，将分散在 Seller Central 的海量碎片化数据拉取到本地，清洗聚合为多维度可视化报表与智能预警。

**注：此项目为作者一时兴起的Vibe coding项目，并不能完全接轨真实的业务环境，且未经过真实的亚马逊 api 接口调用测试，故仅供参考，请以真实业务数据为准。使用者需取得私有/公共开发者权限，方可调用 SP-API。**

### 🛡️ 三大核心原则

| 原则 | 说明 |
|------|------|
| **绝对只读安全** | 全部 API 调用限定为只读，**无 PII 权限**。不具备修改 Listing / 价格 / 库存 / 广告的能力，杜绝误操作风险 |
| **数据本地私有** | 数据存储于本地 SQLite（WAL 模式），API 凭证采用 **AES-256-GCM** 加密，拒绝任何第三方云端上传 |
| **合规高效集成** | Token 自动刷新、429 Rate Limit 指数退避重试、增量/全量轮询调度、严格遵循亚马逊开发者协议 |

### 🌍 站点覆盖

开箱即支持 **21 个 Amazon 官方站点**，覆盖全球三大区域：

| 区域 | 站点 | 货币 |
|------|------|------|
| **北美 (NA)** | 🇺🇸 美国 · 🇨🇦 加拿大 · 🇲🇽 墨西哥 · 🇧🇷 巴西 | USD · CAD · MXN · BRL |
| **欧洲 (EU)** | 🇬🇧 英国 · 🇩🇪 德国 · 🇫🇷 法国 · 🇮🇹 意大利 · 🇪🇸 西班牙 · 🇳🇱 荷兰 · 🇸🇪 瑞典 · 🇵🇱 波兰 · 🇹🇷 土耳其 · 🇦🇪 阿联酋 · 🇸🇦 沙特 · 🇪🇬 埃及 · 🇮🇳 印度 · 🇧🇪 比利时 | GBP · EUR · SEK · PLN · TRY · AED · SAR · EGP · INR |
| **远东 (FE)** | 🇯🇵 日本 · 🇦🇺 澳大利亚 · 🇸🇬 新加坡 | JPY · AUD · SGD |

所有页面的 ASIN 链接、货币符号、汇率换算均根据当前活跃站点 **自动适配**，无需手动配置。

---

## 🛠️ 技术架构

```
┌────────────────────────────────────────────────────────┐
│                    Wails v2 桌面框架                     │
├──────────────────────┬─────────────────────────────────┤
│     Go 后端 (1.24+)   │      React 前端 (Vite + TS)    │
│                      │                                 │
│  ┌─ amazon/          │  ┌─ pages/ (×8)                 │
│  │  ├─ auth/         │  │  ├─ Dashboard    仪表盘      │
│  │  ├─ spapi/        │  │  ├─ Sales        销售分析     │
│  │  ├─ advertising/  │  │  ├─ Products     商品管理     │
│  │  └─ datakiosk/    │  │  ├─ Advertising  广告分析     │
│  │                   │  │  ├─ Inventory    库存管理     │
│  ├─ services/ (×21)  │  │  ├─ Finance      财务分析     │
│  ├─ scheduler/       │  │  ├─ Alerts       智能预警     │
│  ├─ database/        │  │  └─ Settings     系统设置     │
│  └─ config/          │  ├─ components/                 │
│                      │  ├─ stores/ (Zustand)           │
│  app.go (69 绑定)     │  ├─ utils/ (站点/货币工具)      │
│                      │  └─ styles/ (Premium CSS)      │
├──────────────────────┴─────────────────────────────────┤
│       SQLite (WAL + 读写分离 + 8 迁移 + 性能索引)        │
│       21 站点预置 · 17 种货币种子汇率 · AES 加密凭证       │
└────────────────────────────────────────────────────────┘
```

| 层级 | 技术栈 |
|------|--------|
| **前端** | React 18 + TypeScript + Vite + Ant Design 5.x + Apache ECharts |
| **后端** | Go 1.24+ · 21 个 Service 模块 · 69 个 Wails 绑定方法 |
| **存储** | SQLite (WAL 模式 + 读写分离连接池 + 8 份渐进式迁移 + 性能索引) |
| **国际化** | 21 站点域名 + 17 货币符号 · 公共工具模块 `marketplaceUtils.ts` |
| **安全** | AES-256-GCM 加密存储 · LWA OAuth2 · Mutex 并发安全 |
| **设计** | 深色侧边栏 + 明亮工作区 · Glassmorphism 毛玻璃质感 · 微交互动画 |

---

## 🆕 v2.4.0 更新内容

### 🌍 全球站点生产就绪审计
- **全面消除硬编码**：所有页面的货币符号（`$`/`£`/`€` 等）和 ASIN 链接域名（`amazon.com`/`.co.uk` 等）改为根据当前站点 **动态生成**
- **受影响页面**：Sales · Products · Advertising · Inventory（共 10+ 处修复）
- **公共工具模块 `marketplaceUtils.ts`**：统一维护 21 个站点域名映射 + 17 种货币符号映射，消除页面间重复代码
  - `getAmazonDomain(countryCode)` — 获取 Amazon 域名
  - `getCurrencySymbol(currencyCode)` — 获取货币符号
  - `formatAmount(value, currencyCode)` — 格式化金额

### 🔔 预警红点实时同步
- 修复清空/生成模拟数据后侧边栏预警红点残留问题
- `App.tsx` 新增 `activeAccountId` 监听：切换账户自动刷新未读预警计数
- 清空数据后主动 `setUnreadAlertCount(0)`；生成数据后重新查询真实计数

### 🔄 前端状态同步增强
- 模拟数据生成/清空后自动重载 `GetAccounts()` + `GetMarketplaces()`，右上角切换器即时更新
- `appStore.setActiveAccount()` 增加无效 ID 守卫：账户不存在时自动重置 `activeAccountId/Marketplace → null`

---

## 🚀 核心功能模块

### 1. 📊 仪表盘总览 (Dashboard)
- 5 大核心 KPI 卡片：销售额、订单量、广告花费、ACoS、净利润
- **环比趋势计算**：自动推算等长上期区间，实时显示各指标百分比变化
- 净利润口径统一：`销售额 - 广告费 - 平台费 - COGS`（与财务页一致）
- 近 30 天混合趋势图（ECharts）+ 库存健康卡片 + 广告概览卡片
- **📅 利润日历**：月度日历网格，每日净利润色块展示
- **📈 利润率趋势图**：日度利润率折线，按销售额比例智能均摊 COGS/费用
- **🏆 ASIN 利润排行**：Top N 最赚钱 + Top N 最亏钱双榜
- **📄 PDF 经营报表**：6 大专业模块（KPI 表 + 环比对比 + 趋势图 + 利润结构 + ASIN榜 + 广告 ROI）
- **🌐 全局模式**：多账户合并视图，全部 KPI 自动 CNY 换算
- **利润贡献饼图**：各账户/站点净利润占比 ECharts 环形图

### 2. 📈 销售分析 (Sales Analysis)
- 高级销售趋势图 + DataZoom 区间缩放
- "当期 vs 上期"一键对比（自动计算等长对比期）
- 6 维指标切换：销售额 / 订单数 / 销量 / 页面浏览 / 访客数 / 转化率
- ASIN Top 20 排行表（Data Kiosk 数据源）
- 动态货币符号 + 站点域名链接
- CSV 一键导出至 `exports/` 目录

### 3. 📦 商品管理 (Products)
- ASIN 维度销售排行表（90 天汇总）
- 商品详情抽屉：历史趋势折线图 + 实际费率信息
- **📊 竞品价格历史趋势图**：90 天 Buy Box / 落地价 / 卖家数
- **🔍 多 ASIN 叠加对比**：勾选 2-5 个，多系列指标对比
- 单品成本录入（支持 SKU 级 COGS 管理）+ **利润预估**
- Catalog 元数据异步补全（品牌 / 分类 / 高清主图）
- ASIN 链接根据站点自动跳转对应 Amazon 域名

### 4. 🎯 广告分析 (Advertising)
- 活动级汇总表：展示量、点击、花费、归因销售、ACoS、ROAS、CTR
- **搜索词分析 (Search Terms)**：投放漏斗、否定词建议
- **版位报告 (Placements)**：Top of Search / Product Pages / Rest of Search 对比
- **关键词 & ASIN 定向分析**：Top N 绩效排行
- 当期 vs 上期广告花费对比图
- **💰 竞价建议引擎**：基于历史 CVR/ASP 计算最优 CPC（货币自动适配）
- **📥 CSV 一键导出**

### 5. 🏭 库存管理 (Inventory)
- 库存健康分布：可售 / 在途 / 不可售饼图 + 明细表
- **⚡ 补货决策引擎**：日均流速 × 旺季系数 → 有效日均 → 可售天数 → Critical/Warning/Ok 三色分级
  - 用户可配置头程天数 / 安全天数 / 目标天数
  - **旺季系数配置面板**：Q1~Q4 季度滑块 + Prime Day 特殊系数 + 自动按季度应用开关
  - **批量应用**：一键将季度系数写入所有 SKU
- **库龄 & LTSF 预警**：按 0-90 / 91-180 / 181-270 / 271-365 / 365+ 天分段（LTSF 费用货币动态适配）
- **退货分析**：ASIN 退货率统计 + 退货明细 + **退货原因分布饼图**（15 种原因中文翻译）
- CSV 库存快照导出

### 6. 💰 财务分析 (Finance)
- 利润瀑布模型：销售毛收入 → 退款 → 平台佣金 → FBA 费 → 广告 → COGS → 净利润
- 结算报告列表（Settlement TSV 自动解析）
- 批量 CSV 成本导入（SKU / 采购单价 / 币种）
- **🇪🇺 VAT 税率分析**：含税/不含税自动拆分（EU 9 国）
- **📥 利润报表 CSV 导出**

### 7. 🔔 智能预警 (Smart Alerts)
- **7 大预警规则**（每次同步后自动检测）：

  | 预警类型 | 触发条件 | 等级 |
  |----------|----------|------|
  | 📦 低库存 | 可售天数 < 14 天 / 断货 | warning / critical |
  | 📢 高 ACoS | 7 天均值 > 30% | warning |
  | 📉 销售下滑 | 周环比 > 25% | warning |
  | 🚫 Listing 不可售 | 库存清零且有近期销售 | critical |
  | 💲 Buy Box 丢失 | 首次→warning，持续≥N天→critical | 自动升级 |
  | 📉 竞品价格大幅下降 | 日环比降幅超阈值 | warning |
  | 📊 退货率超标 | 30 天退货率 > 10% | warning |

- 24h / 7d 去重机制 · critical 级别 4h 高频告警 · 已读/忽略管理
- **侧边栏红点角标**：切换账户/操作数据后实时同步

### 8. 🏪 竞品价格监控 (Competitive Pricing)
- 定时获取在售 ASIN 的竞品价格（Product Pricing API）
- Buy Box 持有状态追踪 + 竞争卖家数
- **历史价格快照**：`prev_buy_box_price` + `price_change_pct` 日环比追踪
- **连续 Buy Box 丢失天数计算** + 严重程度自动升级
- 价格异动/丢失自动触发分级预警

### 9. ⚙️ 系统设置 (Settings)
- 多账户管理 + 区域选择（NA / EU / FE）
- API 凭证加密存储（LWA Client ID/Secret + Refresh Token + Ads Profile）
- 连接测试（实时调用 SP-API 验证凭证有效性）
- **🔄 数据同步面板**：6 种同步按钮 + 各类延迟说明
- **💾 数据库备份**：手动备份 + 自动每周日备份（保留 4 份）
- **🧪 开发者工具**：一键生成 / 清空模拟数据（操作后前端自动同步）
- 数据库信息面板（路径 / 大小 / 表数量 / 各分类目录）
- 同步历史记录 + 全模块 CSV 数据导出

---

## 🔌 API 集成矩阵

| API | 版本 | 用途 | 限流策略 |
|-----|------|------|----------|
| **Orders API** | v2026-01-01 | 订单及行项目同步 | Burst 20, Rate 0.0167/s |
| **Reports API** | v2021-06-30 | FBA 库存 / 库龄 / 退货 / 结算报表 | 异步轮询 |
| **Data Kiosk API** | v2023-11-15 | GraphQL 销售与流量（日级 + ASIN 级） | T+2 延迟 |
| **Advertising API** | v3 | SP/SB/SD 报表 + 搜索词 + 版位 | T+3 延迟 |
| **Product Pricing API** | v0 | 竞品价格 / Buy Box 状态 / 价格历史 | 0.5 req/s |
| **Catalog Items API** | v2022-04-01 | 商品元数据补全 | 5 req/s |
| **Finances API** | v0 | 财务事件（佣金 / FBA 费 / 退款） | 0.5 req/s |

**限流策略**：SP-API 客户端内置 [指数退避重试](internal/amazon/spapi/client.go)（3 次 + Retry-After 头优先），4xx 非限流错误不重试。

---

## 📁 项目结构

```
Voyage/
├── app.go                        # Wails 主应用 (69 个前端绑定方法)
├── main.go                       # 入口 + Wails 初始化
├── wails.json                    # Wails 配置
│
├── internal/
│   ├── amazon/                   # Amazon API 客户端层
│   │   ├── auth/lwa.go           #   LWA Token 管理 (Mutex 并发安全 + 自动刷新)
│   │   ├── spapi/client.go       #   SP-API 客户端 (指数退避 + 429 Retry-After)
│   │   ├── advertising/client.go #   Advertising API v3 客户端
│   │   └── datakiosk/client.go   #   Data Kiosk GraphQL 客户端
│   │
│   ├── services/                 # 业务服务层 (21 个模块)
│   │   ├── dashboard.go          #   仪表盘 KPI + 环比趋势 + 利润日历 + 利润率趋势 + ASIN 排行
│   │   ├── dashboard_overview.go #   库存/广告概览卡片
│   │   ├── orders.go             #   订单同步 + ASIN 销售排行
│   │   ├── inventory.go          #   FBA 库存同步与查询
│   │   ├── inventory_age.go      #   库龄分析 + LTSF 预警
│   │   ├── replenishment.go      #   补货决策引擎 (旺季系数 + 季度配置)
│   │   ├── returns.go            #   退货同步与分析 + 退货原因分布
│   │   ├── datakiosk_ads.go      #   Data Kiosk + 广告同步
│   │   ├── ad_analytics.go       #   关键词/定向分析
│   │   ├── ad_placement.go       #   版位报告
│   │   ├── ad_search_terms.go    #   搜索词分析
│   │   ├── finance_sync.go       #   财务同步 + CSV 导入导出
│   │   ├── finance_alerts.go     #   财务查询 + 预警引擎 + 同步日志
│   │   ├── pricing.go            #   竞品价格监控 + 价格异动预警
│   │   ├── catalog.go            #   Catalog 元数据补全
│   │   ├── currency.go           #   汇率同步 (双 API 降级) + 多账户合并
│   │   ├── report_pdf.go         #   PDF 周报/月报生成 (6 大模块)
│   │   ├── vat.go                #   欧洲站 VAT 税率管理
│   │   ├── bid_suggest.go        #   广告竞价建议引擎
│   │   ├── data_service.go       #   统一数据查询代理 (只读)
│   │   └── mockdata.go           #   模拟数据生成/清空服务
│   │
│   ├── scheduler/                # 后台定时调度器
│   │   └── scheduler.go          #   7 类定时任务 + 自动备份 + 数据清理
│   │
│   ├── database/                 # 数据库层 (读写分离)
│   │   ├── database.go           #   连接管理 + WAL + 读写分离连接池
│   │   └── migrations/           #   8 份渐进式 SQL 迁移
│   │       ├── 001_initial_schema.sql        # 核心表 + 21 站点预置
│   │       ├── 002_extend_schema.sql         # 库龄/退货/同步日志
│   │       ├── 003_pricing_replenishment.sql  # 竞价 + 补货 + 17 种汇率种子
│   │       ├── 004_placement_catalog.sql      # 版位 + 目录
│   │       ├── 005_ad_keyword_target_tables.sql # 关键词/定向
│   │       ├── 006_performance_indexes.sql    # 性能索引
│   │       ├── 007_vat_bid_tables.sql         # VAT + 竞价
│   │       └── 008_season_factor_price_alerts.sql # 旺季系数 + 价格预警
│   │
│   └── config/                   # 配置与安全
│       ├── config.go             #   AppConfig + 数据目录管理
│       └── crypto.go             #   AES-256-GCM 凭证加解密
│
├── frontend/                     # React 前端
│   ├── src/
│   │   ├── App.tsx               #   路由 + 初始化 + 预警计数同步
│   │   ├── pages/                #   8 个页面组件
│   │   ├── components/           #   Layout / CostImport / ErrorBoundary
│   │   ├── stores/appStore.ts    #   Zustand 全局状态 (账户/站点/同步/预警)
│   │   ├── utils/                #   公共工具
│   │   │   └── marketplaceUtils.ts #  21 站点域名 + 17 货币符号映射
│   │   ├── styles/               #   全局 CSS (Premium 设计系统)
│   │   └── main.tsx              #   Ant Design 主题配置
│   └── package.json
│
├── scripts/
│   └── mockdata.go               # 独立命令行模拟数据生成器 (开发调试用)
│
└── data/                         # 运行时数据目录 (%APPDATA%/Voyage/)
    ├── data/                     #   SQLite 数据库文件
    ├── reports/                  #   PDF 报告输出
    ├── exports/                  #   CSV 导出文件
    ├── backups/                  #   数据库备份 (自动保留 4 份)
    └── logs/                     #   日志文件
```

---

## 🔐 生产环境架构

### 数据安全

| 机制 | 说明 |
|------|------|
| **凭证加密** | API 密钥使用 AES-256-GCM 加密存储于 SQLite `account_credentials` 表，内存中解密，不写明文日志 |
| **Token 管理** | LWA access_token 自动刷新（提前 60s），`sync.Mutex` 保护并发安全 |
| **只读 API** | 全部 SP-API / Ads API 调用限定为 GET / 报告读取，无任何写入权限 |

### 数据库可靠性

| 机制 | 说明 |
|------|------|
| **WAL 模式** | 支持多读一写并发，`busy_timeout=5000` 防超时 |
| **读写分离** | 写连接 `MaxOpenConns=1` 保证单写者，读连接池 `MaxOpenConns=3` 支持并发查询 |
| **两阶段模式** | 所有「读游标 + 写入」组合统一采用先收集数据关闭游标再写入的模式，杜绝连接池死锁 |
| **渐进式迁移** | 8 份 SQL 迁移文件按序执行，自动兼容 SQLite `ALTER TABLE` 限制 |
| **SQL NULL 防护** | 全局 `COALESCE(SUM(...), 0)` + `*string/*float64` 可空类型接收 nullable 字段 |
| **性能索引** | 复合索引覆盖高频查询路径（KPI / 趋势图 / 预警去重） |
| **自动备份** | 每周日 02:00 `VACUUM INTO`，滚动保留 4 份 |

### API 容错

| 机制 | 说明 |
|------|------|
| **指数退避重试** | SP-API 请求失败自动重试（最多 3 次），初始 1s、最大 30s |
| **429 Retry-After** | 限流时优先读取 Amazon 返回的 `Retry-After` 头，精确等待 |
| **4xx 短路** | 非限流的 4xx 错误（401/403/404）不重试，直接返回 |
| **Context 超时** | 每个同步任务独立 Context + Timeout（15~30 分钟级别） |
| **汇率双降级** | 主 API `frankfurter.app` → 备用 `open.er-api.com` → DB 种子汇率兜底 |

### 前端健壮性

| 机制 | 说明 |
|------|------|
| **ErrorBoundary** | 每个页面和顶层 Layout 均有隔离，单页崩溃不影响全局 |
| **null 前置守卫** | 所有数据操作均有 `if (!activeAccountId) return` 守卫 |
| **ECharts 安全初始化** | `useCallback` ref callback 模式，兼容 React 条件渲染时序 |
| **预警实时同步** | `activeAccountId` 变化自动触发 `GetUnreadAlertCount`；清空数据后主动归零 |
| **国际化工具** | `marketplaceUtils.ts` 统一维护 21 站点域名 + 17 货币符号 |

---

## 💻 快速开始

### 环境要求

| 工具 | 版本 | 用途 |
|------|------|------|
| [Go](https://go.dev/dl/) | ≥ 1.24 | 后端编译 |
| [Node.js](https://nodejs.org/) | ≥ 18 | 前端构建 |
| [Wails CLI](https://wails.io/zh-Hans/docs/gettingstarted/installation) | v2 | 桌面框架 |

### 开发模式

```bash
# 克隆项目
git clone <repo-url> && cd Voyage

# 安装前端依赖
cd frontend && npm install && cd ..

# 启动开发服务 (热更新)
wails dev
```

前端修改即时热重载，可通过 `http://localhost:34115` 在浏览器中调试。

### 生产构建

```bash
wails build
```

生成的原生可执行文件位于 `build/bin/` 目录。

### 模拟数据

> **推荐方式（UI 操作）**：启动应用后前往 **设置** → **数据同步与备份** → **开发者工具** → 点击 **🚀 生成模拟数据**

也可通过命令行生成：

```bash
go run scripts/mockdata.go
```

模拟数据包含 8 款商品、90 天完整销售/订单/广告/库存/财务/退货/预警数据，覆盖 25+ 张业务表，自动根据账户地区生成对应货币数据。

---

## 🔧 生产环境配置

### 1. 创建账户

启动应用后，前往 **设置** → **店铺管理** 选项卡 → 新建账户并选择区域（NA / EU / FE）。

### 2. 配置 API 凭证

前往 **设置** → **API 凭证** 选项卡，填入以下信息：

| 凭证 | 来源 | 必填 |
|------|------|------|
| LWA Client ID | Amazon Developer Console → Security Profile | ✅ |
| LWA Client Secret | 同上 | ✅ |
| Refresh Token | SP-API 授权流程 | ✅ |
| Ads Client ID | 广告 API 应用（不需要广告分析可留空） | ❌ |
| Ads Profile ID | `GET /v2/profiles` 获取 | ❌ |

### 3. 连接测试

配置完成后点击 **测试连接** 按钮，系统会实时调用 SP-API 验证凭证有效性。

### 4. 自动同步时间表

设置就绪后，后台调度器将按以下频率自动拉取数据：

| 数据类型 | 同步频率 | 数据延迟 | 超时 |
|----------|----------|----------|------|
| 订单 | 每 30 分钟 | 近实时 | 25 分钟 |
| 销售与流量 (Data Kiosk) | 每 6 小时 | T+2 天 | 20 分钟 |
| FBA 库存 + 预警检测 | 每 6 小时 | 近实时 | 15 分钟 |
| 广告效果 | 每 6 小时 | T+3 天 | 30 分钟 |
| 竞品价格 + 异动检测 | 每 4 小时 | ≈15 分钟 | — |
| 汇率 | 每 24 小时 | — | 15 秒 |
| 数据库备份 | 每周日 02:00 | — | — |
| 过期数据清理 | 每月 1 号 | — | — |

也可在 **设置** → **数据同步与备份** 手动触发。

### 5. 数据存储目录

所有运行时数据按分类存储在 `%APPDATA%/Voyage/` 下：

| 目录 | 用途 | 示例文件 |
|------|------|----------|
| `data/` | SQLite 数据库 | `voyage.db` |
| `reports/` | PDF 经营报告 | `voyage_weekly_2026-04-01.pdf` |
| `exports/` | CSV 导出文件 | `voyage_sales_20260405_135500.csv` |
| `backups/` | 数据库备份 | `voyage_backup_20260406_020000.db` |
| `logs/` | 日志文件 | `voyage_20260406.log` |

---

## 📋 版本历史

| 版本 | 日期 | 主要变更 |
|------|------|----------|
| **v2.4.0** | 2026-04-07 | 🌍 全球 21 站点生产审计 · 货币/域名动态化 · 🔔 预警红点实时同步 · 🔄 前端状态同步增强 · 📦 公共工具模块 `marketplaceUtils.ts` |
| v2.3.0 | 2026-04-06 | 📄 PDF 报告增强版（6 大模块）· ⚡ 旺季系数补货引擎 · 📉 竞品价格异动预警 · 📊 利润率趋势 · 🏆 ASIN 排行 · 🔒 9 处死锁修复 |
| v2.2.0 | 2026-04-05 | ECharts 时序修复 · 文件导出统一架构 · 模拟数据 UI 集成 |
| v2.1.0 | 2026-04-05 | PDF 周报/月报 · EU VAT 税率 · 广告竞价建议引擎 |
| v2.0.0 | 2026-04-04 | 利润日历 · 竞品价格历史 · 退货原因分布 · ASIN 对比 · 自动备份 |
| v1.0.0 | 2026-04-02 | 生产级硬化：KPI 环比修复 · 库存重构 · 性能索引 · 文档完善 |
| v0.9.0 | 2026-03-30 | 初始版本：8 页面 · 16 服务 · 5 预警规则 · 7 API 集成 |

---

## 📄 许可及免责声明

本项目基于安全合规原则设计，严格遵循亚马逊开发者协议。所有 API 调用均为只读操作，不触及任何 PII（个人隐私信息）。相关工具逻辑不构成商业运营保证，使用者须自行承担由于亚马逊官方 API 策略调整、网络中断等外部因素导致的数据同步异常。
