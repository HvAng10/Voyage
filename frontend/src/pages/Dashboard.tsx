import { useEffect, useState, useCallback } from 'react'
import { Empty, Spin, Button, message, Dropdown, Tooltip, Tag, Switch } from 'antd'
import {
  ArrowUpOutlined, ArrowDownOutlined,
  DollarCircleOutlined, ShoppingCartOutlined,
  FundOutlined, RiseOutlined, ReloadOutlined, InfoCircleOutlined, FilePdfOutlined,
  TrophyOutlined, FrownOutlined, GlobalOutlined,
} from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import dayjs from 'dayjs'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetDashboardKPI, GetDailyTrend, GetProfitMarginTrend, GetAsinProfitRank, TriggerSync, GetInventoryOverview, GetAdOverview, GetDailyProfitCalendar, GeneratePDFReport, OpenFileInExplorer, GetCrossAccountKPI } from '../../wailsjs/go/main/App'

interface KPI {
  totalSales: number; salesTrend: number; currency: string
  totalOrders: number; ordersTrend: number
  adSpend: number; adSpendTrend: number
  acos: number; acosTrend: number
  netProfit: number; netProfitTrend: number
  dataLatencyDays: number
}
interface DailyPoint {
  date: string; sales: number; orders: number; adSpend: number; acos: number
}
interface InvOverview {
  totalSku: number; criticalCount: number; warningCount: number; okCount: number; totalFulfillable: number
}
interface AdOverview {
  totalCampaigns: number; totalSpend: number; totalSales: number; avgAcos: number; avgRoas: number
}
interface MarginPoint {
  date: string; sales: number; netProfit: number; margin: number; adSpend: number; fees: number; cogs: number
}
interface AsinProfitItem {
  asin: string; title: string; sku: string
  totalSales: number; totalCogs: number; netProfit: number
  margin: number; unitsSold: number; unitCost: number
  dataLatencyNote: string
}

// 全局模式下的多账户汇总数据类型
interface AccountBreakdownItem {
  accountId: number; accountName: string
  marketplaceId: string; marketplaceName: string; originalCurrency: string
  sales: number; salesCny: number; orders: number
  adSpend: number; adSpendCny: number
  fees: number; feesCny: number; cogs: number; cogsCny: number
  netProfit: number; netProfitCny: number
  profitMargin: number; profitSharePct: number
}
interface CrossAccountData {
  totalSalesCny: number; totalOrderCount: number; totalAdSpendCny: number
  totalFeesCny: number; totalCogsCny: number; totalNetProfitCny: number; totalProfitMargin: number
  accountBreakdown: AccountBreakdownItem[]; baseCurrency: string; rateUpdatedAt: string
}

export default function Dashboard() {
  const { activeAccountId, activeMarketplaceId, marketplaces, accounts, setSyncState } = useAppStore()
  const mp = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currency = mp?.currencyCode ?? 'USD'

  const [kpi, setKpi] = useState<KPI | null>(null)
  const [trend, setTrend] = useState<DailyPoint[]>([])
  const [invOverview, setInvOverview] = useState<InvOverview | null>(null)
  const [adOverview, setAdOverview] = useState<AdOverview | null>(null)
  const [marginTrend, setMarginTrend] = useState<MarginPoint[]>([])
  const [asinRank, setAsinRank] = useState<{ best: AsinProfitItem[]; worst: AsinProfitItem[] }>({ best: [], worst: [] })
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)

  // 全局模式状态
  const [globalMode, setGlobalMode] = useState(false)
  const [crossData, setCrossData] = useState<CrossAccountData | null>(null)
  const [globalLoading, setGlobalLoading] = useState(false)

  // 利润日历
  const [calendarMonth, setCalendarMonth] = useState(dayjs().format('YYYY-MM'))
  const [profitCells, setProfitCells] = useState<{date:string;sales:number;adSpend:number;fees:number;cogs:number;netProfit:number;level:string}[]>([])

  // 默认日期：近 30 天（Data Kiosk T+2 延迟）
  const dateEnd   = dayjs().subtract(2, 'day').format('YYYY-MM-DD')
  const dateStart = dayjs().subtract(31, 'day').format('YYYY-MM-DD')
  const prevEnd   = dayjs().subtract(32, 'day').format('YYYY-MM-DD')
  const prevStart = dayjs().subtract(62, 'day').format('YYYY-MM-DD')

  // 全局模式数据获取
  const fetchGlobalData = useCallback(async () => {
    if (accounts.length === 0) return
    setGlobalLoading(true)
    try {
      const data: any = await GetCrossAccountKPI(dateStart, dateEnd)
      setCrossData(data as CrossAccountData)
    } catch { setCrossData(null) }
    finally { setGlobalLoading(false) }
  }, [accounts.length, dateStart, dateEnd])

  useEffect(() => {
    if (globalMode) { fetchGlobalData() }
  }, [globalMode, fetchGlobalData])

  const fetchData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const [k, t, inv, ad, mt, ar] = await Promise.all([
        GetDashboardKPI(activeAccountId, activeMarketplaceId, dateStart, dateEnd),
        GetDailyTrend(activeAccountId, activeMarketplaceId, dateStart, dateEnd),
        GetInventoryOverview(activeAccountId, activeMarketplaceId),
        GetAdOverview(activeAccountId, activeMarketplaceId, dateStart, dateEnd),
        GetProfitMarginTrend(activeAccountId, activeMarketplaceId, dateStart, dateEnd),
        GetAsinProfitRank(activeAccountId, activeMarketplaceId, dateStart, dateEnd, 5),
      ])
      setKpi(k)
      setTrend(t ?? [])
      setInvOverview(inv as unknown as InvOverview)
      setAdOverview(ad as unknown as AdOverview)
      setMarginTrend((mt as any) ?? [])
      setAsinRank((ar as any) ?? { best: [], worst: [] })
    } catch { setKpi(null); setTrend([]) }
    finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId])

  useEffect(() => { fetchData() }, [fetchData])

  // 利润日历数据
  useEffect(() => {
    if (!activeAccountId || !activeMarketplaceId) return
    GetDailyProfitCalendar(activeAccountId, activeMarketplaceId, calendarMonth)
      .then((d: any) => setProfitCells(d ?? []))
      .catch(() => setProfitCells([]))
  }, [activeAccountId, activeMarketplaceId, calendarMonth])

  // 手动同步（订单 + Data Kiosk）
  const handleSync = async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setSyncing(true)
    setSyncState({ status: 'syncing', message: '正在同步数据...', progress: 10 })
    try {
      // 并发同步订单和 Data Kiosk
      const results = await Promise.allSettled([
        TriggerSync(activeAccountId, activeMarketplaceId, 'orders'),
        TriggerSync(activeAccountId, activeMarketplaceId, 'datakiosk'),
      ])
      const allOk = results.every(r => r.status === 'fulfilled' && (r.value as any)?.success)
      if (allOk) {
        message.success('数据同步完成')
        setSyncState({ status: 'success', message: '同步成功', lastSyncTime: new Date().toLocaleString('zh-CN'), progress: 100 })
      } else {
        const errMsg = results.find(r => r.status === 'rejected' || !(r as any).value?.success)
        message.warning('部分数据同步失败，请检查 API 凭证配置')
        setSyncState({ status: 'error', message: '同步部分失败' })
      }
      await fetchData()
    } finally { setSyncing(false) }
  }

  const chartOption = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#1a2744', borderColor: '#2d4078',
      textStyle: { color: '#e8e8e8', fontSize: 12 },
    },
    legend: { data: [`销售额 (${currency})`, '订单量', 'ACoS (%)'], textStyle: { color: '#6b7280', fontSize: 12 }, top: 4, left: 'center', itemGap: 24 },
    grid: { top: 36, right: 60, bottom: 24, left: 70 },
    xAxis: {
      type: 'category', data: trend.map(d => d.date.slice(5)),
      axisLabel: { color: '#9ca3af', fontSize: 11 }, axisLine: { lineStyle: { color: '#e5e7eb' } }, splitLine: { show: false },
    },
    yAxis: [
      { type: 'value', name: currency, nameTextStyle: { color: '#9ca3af', fontSize: 11 },
        axisLabel: { color: '#9ca3af', fontSize: 11 }, splitLine: { lineStyle: { color: '#f0f0f0' } } },
      { type: 'value', name: 'ACoS %', nameTextStyle: { color: '#9ca3af', fontSize: 11 },
        axisLabel: { color: '#9ca3af', fontSize: 11 }, splitLine: { show: false }, min: 0, max: 100 },
    ],
    series: [
      {
        name: `销售额 (${currency})`, type: 'line', smooth: true, yAxisIndex: 0,
        data: trend.map(d => +d.sales.toFixed(2)),
        lineStyle: { color: '#1a2744', width: 2.5 }, itemStyle: { color: '#1a2744' },
        areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [{ offset: 0, color: 'rgba(26,39,68,0.15)' }, { offset: 1, color: 'rgba(26,39,68,0.01)' }] } },
      },
      {
        name: '订单量', type: 'bar', yAxisIndex: 0,
        data: trend.map(d => d.orders),
        itemStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [{ offset: 0, color: 'rgba(201,168,76,0.8)' }, { offset: 1, color: 'rgba(201,168,76,0.2)' }] }, borderRadius: [2,2,0,0] },
        barMaxWidth: 18,
      },
      {
        name: 'ACoS (%)', type: 'line', smooth: true, yAxisIndex: 1, symbol: 'none',
        data: trend.map(d => +d.acos.toFixed(1)),
        lineStyle: { color: '#c9a84c', width: 2, type: 'dashed' }, itemStyle: { color: '#c9a84c' },
      },
    ],
  }

  if (!activeAccountId) {
    return (
      <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <div className="no-data-hint">
          <div className="no-data-hint-icon">🏪</div>
          <div className="no-data-hint-text">尚未配置店铺账户</div>
          <div className="no-data-hint-sub">请前往系统设置 → 添加您的亚马逊卖家账户</div>
        </div>
      </div>
    )
  }

  return (
    <div>
      {/* 页面标题 */}
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div>
            <div className="page-title">仪表盘总览</div>
            <div className="page-subtitle">
              {globalMode
                ? <span><GlobalOutlined style={{ marginRight: 4 }} />全局模式 · 全部账户合并 · 近 30 天 · CNY</span>
                : <>{mp ? `${mp.name} · 近 30 天` : '请选择站点'}&emsp;<span className="latency-warning"><InfoCircleOutlined style={{ marginRight: 4 }} />Data Kiosk T+2 延迟</span></>
              }
            </div>
          </div>
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center' }}>
            {/* 全局模式切换 */}
            <Tooltip title={globalMode ? '切换到单账户视图' : '切换到全局总览（合并所有账户）'}>
              <div style={{
                display: 'flex', alignItems: 'center', gap: 6,
                padding: '4px 12px', borderRadius: 8,
                background: globalMode ? 'linear-gradient(135deg, #1a2744, #2d4078)' : '#f0f2f5',
                transition: 'all 0.3s',
              }}>
                <GlobalOutlined style={{ fontSize: 13, color: globalMode ? '#c9a84c' : '#8c8c8c' }} />
                <span style={{ fontSize: 12, color: globalMode ? '#fff' : '#595959', fontWeight: 500 }}>全局</span>
                <Switch
                  size="small"
                  checked={globalMode}
                  onChange={setGlobalMode}
                />
              </div>
            </Tooltip>
            {!globalMode && (
              <>
                <Dropdown menu={{ items: [
                  { key: 'weekly', label: '📊 生成周报', onClick: async () => {
                    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择店铺账户'); return }
                    const hide = message.loading('正在生成周报 PDF...', 0)
                    try {
                      const r: any = await GeneratePDFReport(activeAccountId, activeMarketplaceId, 'weekly')
                      hide()
                      if (r?.success) {
                        message.success(`${r.message} (${r.fileSize})`)
                        OpenFileInExplorer(r.path)
                      } else {
                        message.error(r?.message ?? '生成失败，请查看后台日志')
                      }
                    } catch (e: any) {
                      hide()
                      console.error('PDF 生成异常:', e)
                      message.error(`PDF 生成异常: ${e?.message ?? String(e)}`)
                    }
                  }},
                  { key: 'monthly', label: '📊 生成月报', onClick: async () => {
                    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择店铺账户'); return }
                    const hide = message.loading('正在生成月报 PDF...', 0)
                    try {
                      const r: any = await GeneratePDFReport(activeAccountId, activeMarketplaceId, 'monthly')
                      hide()
                      if (r?.success) {
                        message.success(`${r.message} (${r.fileSize})`)
                        OpenFileInExplorer(r.path)
                      } else {
                        message.error(r?.message ?? '生成失败，请查看后台日志')
                      }
                    } catch (e: any) {
                      hide()
                      console.error('PDF 生成异常:', e)
                      message.error(`PDF 生成异常: ${e?.message ?? String(e)}`)
                    }
                  }},
                ] }}>
                  <Button icon={<FilePdfOutlined />}>PDF 报告</Button>
                </Dropdown>
                <Button icon={<ReloadOutlined />} loading={syncing} onClick={handleSync}>立即同步</Button>
              </>
            )}
          </div>
        </div>
      </div>

      {/* 全局模式 → 多账户总览 */}
      {globalMode ? (
        globalLoading ? (
          <div className="voyage-loading"><Spin size="large" /></div>
        ) : crossData ? (
          <GlobalOverview data={crossData} />
        ) : (
          <div className="voyage-card" style={{ marginBottom: 20, padding: '40px 20px' }}>
            <Empty description="暂无多账户数据，请先配置并同步至少一个账户" />
          </div>
        )
      ) : (
      <>
      {/* KPI 卡片 */}
      {loading ? (
        <div className="voyage-loading"><Spin size="large" /></div>
      ) : !kpi || kpi.totalOrders === 0 ? (
        <div className="voyage-card" style={{ marginBottom: 20, padding: '40px 20px' }}>
          <Empty description={
            <span>暂无数据 · <Button type="link" onClick={handleSync} loading={syncing}>点击同步数据</Button>或在设置页面配置 API 凭证</span>
          } />
        </div>
      ) : (
        <div className="kpi-grid">
          <KPICard label="期间销售额" value={`${currency} ${kpi.totalSales.toLocaleString('en-US', { minimumFractionDigits: 0 })}`}
            trend={kpi.salesTrend} icon={<DollarCircleOutlined />} className="kpi-sales" />
          <KPICard label="期间订单数" value={`${kpi.totalOrders.toLocaleString()} 单`}
            trend={kpi.ordersTrend} icon={<ShoppingCartOutlined />} className="kpi-orders" />
          <KPICard label="广告花费" value={`${currency} ${kpi.adSpend.toLocaleString('en-US', { minimumFractionDigits: 0 })}`}
            trend={kpi.adSpendTrend} trendReverse icon={<FundOutlined />} className="kpi-ads" />
          <KPICard label="净利润" value={`${currency} ${kpi.netProfit.toLocaleString('en-US', { minimumFractionDigits: 0 })}`}
            trend={kpi.netProfitTrend} icon={<RiseOutlined />} className="kpi-profit" />
        </div>
      )}

      {/* 趋势图 */}
      <div className="voyage-card" style={{ marginBottom: 16 }}>
        <div className="voyage-card-header">
          <span className="voyage-card-title">📈 销售趋势（近 30 天）</span>
          <span className="data-delay-badge"><InfoCircleOutlined /> T+2 延迟</span>
        </div>
        <div style={{ padding: '12px 16px' }}>
          {trend.length === 0
            ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="同步数据后显示" style={{ padding: 40 }} />
            : <ReactECharts option={chartOption} style={{ height: 280 }} opts={{ renderer: 'canvas' }} />
          }
        </div>
      </div>

      {/* ── 利润率趋势图 + ASIN 利润排行 双栏 ── */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>

        {/* 利润率趋势折线图 */}
        <div className="voyage-card">
          <div className="voyage-card-header">
            <span className="voyage-card-title">💹 利润率趋势（近 30 天）</span>
            <Tooltip title="利润率 = (销售额 - 广告费 - 平台费 - 采购成本) / 销售额 × 100%，基于已录入 COGS 和财务数据估算">
              <span className="data-delay-badge" style={{ cursor: 'help' }}><InfoCircleOutlined /> 估算数据</span>
            </Tooltip>
          </div>
          <div style={{ padding: '0 16px 12px' }}>
            {marginTrend.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="录入商品成本后显示" style={{ padding: 32 }} />
            ) : (
              <ReactECharts
                option={{
                  backgroundColor: 'transparent',
                  tooltip: {
                    trigger: 'axis',
                    backgroundColor: '#1a2744', borderColor: '#2d4078',
                    textStyle: { color: '#e8e8e8', fontSize: 12 },
                    formatter: (params: any[]) => {
                      const d = params[0]?.axisValue ?? ''
                      const marginP = params.find((p: any) => p.seriesName === '利润率 %')
                      const profitP = params.find((p: any) => p.seriesName === `净利润 (${currency})`)
                      const salesP  = params.find((p: any) => p.seriesName === `销售额 (${currency})`)
                      return `<div style="font-size:12px;line-height:1.8">
                        <b>${d}</b><br/>
                        ${salesP  ? `🟦 销售额：${currency} ${(salesP.value  as number).toFixed(0)}<br/>` : ''}
                        ${profitP ? `🟩 净利润：${currency} ${(profitP.value as number).toFixed(0)}<br/>` : ''}
                        ${marginP ? `📊 利润率：<b style="color:${(marginP.value as number) >= 0 ? '#10b981' : '#ef4444'}">${(marginP.value as number).toFixed(1)}%</b>` : ''}
                      </div>`
                    },
                  },
                  legend: {
                    data: [`销售额 (${currency})`, `净利润 (${currency})`, '利润率 %'],
                    textStyle: { color: '#6b7280', fontSize: 11 }, top: 4, left: 'center', itemGap: 16,
                  },
                  grid: { top: 36, right: 56, bottom: 24, left: 70 },
                  xAxis: {
                    type: 'category',
                    data: marginTrend.map(d => d.date.slice(5)),
                    axisLabel: { color: '#9ca3af', fontSize: 10 },
                    axisLine: { lineStyle: { color: '#e5e7eb' } },
                    splitLine: { show: false },
                  },
                  yAxis: [
                    {
                      type: 'value', name: currency,
                      nameTextStyle: { color: '#9ca3af', fontSize: 10 },
                      axisLabel: { color: '#9ca3af', fontSize: 10 },
                      splitLine: { lineStyle: { color: '#f0f0f0', type: 'dashed' } },
                    },
                    {
                      type: 'value', name: '利润率 %', min: -30, max: 80,
                      nameTextStyle: { color: '#9ca3af', fontSize: 10 },
                      axisLabel: {
                        color: '#9ca3af', fontSize: 10,
                        formatter: (v: number) => `${v}%`,
                      },
                      splitLine: { show: false },
                    },
                  ],
                  series: [
                    {
                      name: `销售额 (${currency})`,
                      type: 'bar', yAxisIndex: 0,
                      data: marginTrend.map(d => +d.sales.toFixed(2)),
                      itemStyle: {
                        color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
                          colorStops: [
                            { offset: 0, color: 'rgba(26,39,68,0.6)' },
                            { offset: 1, color: 'rgba(26,39,68,0.15)' },
                          ] },
                        borderRadius: [2, 2, 0, 0],
                      },
                      barMaxWidth: 14,
                    },
                    {
                      name: `净利润 (${currency})`,
                      type: 'bar', yAxisIndex: 0,
                      data: marginTrend.map(d => +d.netProfit.toFixed(2)),
                      itemStyle: {
                        color: (params: any) => {
                          const v = params.value as number
                          return v >= 0
                            ? { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
                                colorStops: [{ offset: 0, color: 'rgba(16,185,129,0.8)' }, { offset: 1, color: 'rgba(16,185,129,0.2)' }] }
                            : { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
                                colorStops: [{ offset: 0, color: 'rgba(239,68,68,0.8)' }, { offset: 1, color: 'rgba(239,68,68,0.2)' }] }
                        },
                        borderRadius: [2, 2, 0, 0],
                      },
                      barMaxWidth: 14,
                    },
                    {
                      name: '利润率 %',
                      type: 'line', smooth: true, yAxisIndex: 1,
                      data: marginTrend.map(d => +d.margin.toFixed(1)),
                      lineStyle: { color: '#f59e0b', width: 2.5 },
                      itemStyle: { color: '#f59e0b' },
                      symbol: 'circle', symbolSize: 5,
                      // 正负区域着色
                      markArea: {
                        silent: true,
                        data: [[
                          { yAxis: 0, itemStyle: { color: 'rgba(16,185,129,0.04)' } },
                          { yAxis: 80 },
                        ]],
                      },
                    },
                  ],
                }}
                style={{ height: 280 }}
                opts={{ renderer: 'canvas' }}
              />
            )}
          </div>
        </div>

        {/* ASIN 利润率排行 */}
        <div className="voyage-card">
          <div className="voyage-card-header">
            <span className="voyage-card-title">🏆 ASIN 利润率排行（毛利）</span>
            <Tooltip title="基于 COGS（采购成本）计算毛利率，不含平台费和广告费。需先在设置中录入商品成本。">
              <span className="data-delay-badge" style={{ cursor: 'help' }}><InfoCircleOutlined /> 仅含 COGS</span>
            </Tooltip>
          </div>
          <div style={{ padding: '0 16px 12px' }}>
            {asinRank.best.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="录入商品成本后显示" style={{ padding: 32 }} />
            ) : (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>

                {/* 最赚钱 Top 5 */}
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
                    <TrophyOutlined style={{ color: '#c9a84c', fontSize: 14 }} />
                    <span style={{ fontSize: 12, fontWeight: 600, color: '#10b981' }}>最赚钱 Top 5</span>
                  </div>
                  {asinRank.best.map((item, i) => (
                    <AsinRankRow key={item.asin} item={item} rank={i + 1} currency={currency} good />
                  ))}
                </div>

                {/* 最亏钱 Top 5 */}
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
                    <FrownOutlined style={{ color: '#ef4444', fontSize: 14 }} />
                    <span style={{ fontSize: 12, fontWeight: 600, color: '#ef4444' }}>最亏钱 Top 5</span>
                  </div>
                  {asinRank.worst.map((item, i) => (
                    <AsinRankRow key={item.asin} item={item} rank={i + 1} currency={currency} good={false} />
                  ))}
                </div>

              </div>
            )}
          </div>
        </div>
      </div>

      {/* 底部双栏 */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* 库存概览 */}
        <div className="voyage-card">
          <div className="voyage-card-header"><span className="voyage-card-title">📦 库存概览</span></div>
          <div className="voyage-card-body">
            {!invOverview || invOverview.totalSku === 0
              ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="同步 FBA 库存后显示" />
              : (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  {[
                    { label: '总 SKU 数',    value: invOverview.totalSku,       color: 'var(--color-primary)' },
                    { label: '总可售库存', value: `${invOverview.totalFulfillable.toLocaleString()} 件`, color: '#1a2744' },
                    { label: '🔴 断货/紧急', value: invOverview.criticalCount, color: '#ef4444' },
                    { label: '🟡 库存不足', value: invOverview.warningCount,  color: '#f59e0b' },
                  ].map(c => (
                    <div key={c.label} style={{ padding: '10px 12px', background: '#f8fafc', borderRadius: 8 }}>
                      <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 2 }}>{c.label}</div>
                      <div style={{ fontSize: 20, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
                    </div>
                  ))}
                </div>
              )
            }
          </div>
        </div>
        {/* 广告概览 */}
        <div className="voyage-card">
          <div className="voyage-card-header"><span className="voyage-card-title">📢 广告概览</span></div>
          <div className="voyage-card-body">
            {!adOverview || adOverview.totalCampaigns === 0
              ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="同步广告数据后显示" />
              : (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  {[
                    { label: '活动活动数', value: `${adOverview.totalCampaigns} 个`,              color: 'var(--color-primary)' },
                    { label: `广告花费`, value: `${currency} ${adOverview.totalSpend.toFixed(0)}`, color: '#1a2744' },
                    { label: '平均 ACoS',  value: `${adOverview.avgAcos.toFixed(1)}%`,
                      color: adOverview.avgAcos < 25 ? '#10b981' : adOverview.avgAcos < 35 ? '#f59e0b' : '#ef4444' },
                    { label: '平均 ROAS',  value: `${adOverview.avgRoas.toFixed(2)}x`,
                      color: adOverview.avgRoas >= 3 ? '#10b981' : '#f59e0b' },
                  ].map(c => (
                    <div key={c.label} style={{ padding: '10px 12px', background: '#f8fafc', borderRadius: 8 }}>
                      <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 2 }}>{c.label}</div>
                      <div style={{ fontSize: 20, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
                    </div>
                  ))}
                </div>
              )
            }
          </div>
        </div>
      </div>

      {/* ── 利润日历 ── */}
      <div className="voyage-card" style={{ padding: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <div style={{ fontWeight: 600, fontSize: 15 }}>📅 利润日历</div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <button onClick={() => setCalendarMonth(dayjs(calendarMonth).subtract(1, 'month').format('YYYY-MM'))}
              style={{ border: '1px solid #e5e7eb', borderRadius: 6, padding: '2px 10px', cursor: 'pointer', background: '#fff' }}>◀</button>
            <span style={{ fontWeight: 600, fontFamily: 'var(--font-number)', minWidth: 80, textAlign: 'center' }}>{calendarMonth}</span>
            <button onClick={() => setCalendarMonth(dayjs(calendarMonth).add(1, 'month').format('YYYY-MM'))}
              style={{ border: '1px solid #e5e7eb', borderRadius: 6, padding: '2px 10px', cursor: 'pointer', background: '#fff' }}>▶</button>
            <span style={{ fontSize: 11, color: 'var(--color-text-muted)', marginLeft: 8 }}>
              🟢盈利 🟡低利(&lt;5%) 🔴亏损
            </span>
          </div>
        </div>
        {profitCells.length === 0 ? (
          <div style={{ textAlign: 'center', color: 'var(--color-text-muted)', padding: '30px 0', fontSize: 13 }}>
            该月暂无数据，同步后显示
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 4 }}>
            {['日', '一', '二', '三', '四', '五', '六'].map(d => (
              <div key={d} style={{ textAlign: 'center', fontSize: 11, color: 'var(--color-text-muted)', padding: 4 }}>{d}</div>
            ))}
            {/* 填充月初空白 */}
            {Array.from({ length: dayjs(calendarMonth + '-01').day() }).map((_, i) => (
              <div key={`e${i}`} />
            ))}
            {profitCells.map(cell => {
              const bg = cell.level === 'profit' ? '#d1fae5' : cell.level === 'low' ? '#fef3c7' : '#fee2e2'
              const border = cell.level === 'profit' ? '#6ee7b7' : cell.level === 'low' ? '#fcd34d' : '#fca5a5'
              const day = parseInt(cell.date.slice(8))
              return (
                <div key={cell.date} title={`${cell.date}\n销售: ${currency} ${cell.sales.toFixed(0)}\n广告: ${currency} ${cell.adSpend.toFixed(0)}\n费用: ${currency} ${cell.fees.toFixed(0)}\nCOGS: ${currency} ${cell.cogs.toFixed(0)}\n净利润: ${currency} ${cell.netProfit.toFixed(0)}`}
                  style={{
                    background: bg, border: `1.5px solid ${border}`, borderRadius: 8,
                    padding: '6px 4px', textAlign: 'center', cursor: 'default',
                    transition: 'transform 0.15s', minHeight: 52,
                  }}
                  onMouseEnter={e => (e.currentTarget.style.transform = 'scale(1.08)')}
                  onMouseLeave={e => (e.currentTarget.style.transform = 'scale(1)')}
                >
                  <div style={{ fontSize: 11, color: '#6b7280', marginBottom: 2 }}>{day}</div>
                  <div style={{ fontSize: 13, fontWeight: 700, fontFamily: 'var(--font-number)',
                    color: cell.netProfit >= 0 ? '#047857' : '#dc2626' }}>
                    {cell.netProfit >= 0 ? '+' : ''}{cell.netProfit.toFixed(0)}
                  </div>
                  <div style={{ fontSize: 9, color: '#9ca3af' }}>{currency} {cell.sales.toFixed(0)}</div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </>
    )}
    </div>
  )
}

interface KPICardProps {
  label: string; value: string; trend: number
  icon: React.ReactNode; className: string; trendReverse?: boolean
}

function KPICard({ label, value, trend, icon, className, trendReverse = false }: KPICardProps) {
  const isGood = trendReverse ? trend < 0 : trend > 0
  const isBad  = trendReverse ? trend > 0 : trend < 0
  return (
    <div className={`kpi-card ${className}`}>
      <div className="kpi-card-label">{label}</div>
      <div className="kpi-card-value">{value}</div>
      <div className="kpi-card-trend">
        {trend > 0 && <ArrowUpOutlined />}
        {trend < 0 && <ArrowDownOutlined />}
        <span style={{ color: isGood ? '#6ee7b7' : isBad ? '#fca5a5' : 'inherit' }}>
          {trend !== 0 ? `${Math.abs(trend).toFixed(1)}% 较上期` : '与上期持平'}
        </span>
      </div>
      <div className="kpi-card-icon">{icon}</div>
    </div>
  )
}

// ASIN 利润率排行行组件
interface AsinRankRowProps {
  item: AsinProfitItem
  rank: number
  currency: string
  good: boolean
}

function AsinRankRow({ item, rank, currency, good }: AsinRankRowProps) {
  const marginColor = item.margin >= 30 ? '#10b981'
    : item.margin >= 15 ? '#f59e0b'
    : item.margin >= 0  ? '#ef8c22'
    : '#ef4444'

  // 进度条宽度（按 0~80% 利润率区间归一化，亏损用红色反向）
  const barPct = Math.min(100, Math.max(0, Math.abs(item.margin) / 80 * 100))
  const barColor = item.margin >= 0
    ? (good ? '#10b981' : '#f59e0b')
    : '#ef4444'

  // 截断长标题
  const shortTitle = item.title.length > 18 ? item.title.slice(0, 18) + '…' : item.title

  return (
    <Tooltip
      title={
        <div style={{ fontSize: 12, lineHeight: 1.8 }}>
          <div><b>{item.asin}</b> {item.sku ? `(${item.sku})` : ''}</div>
          <div>销售额：{currency} {item.totalSales.toFixed(0)}</div>
          <div>COGS：{currency} {item.totalCogs.toFixed(0)}</div>
          <div>毛利润：{currency} {item.netProfit.toFixed(0)}</div>
          <div>已售件数：{item.unitsSold} 件</div>
          <div style={{ color: '#fbbf24', fontSize: 11 }}>{item.dataLatencyNote}</div>
        </div>
      }
      placement="left"
    >
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '5px 0', borderBottom: '1px solid #f3f4f6', cursor: 'default',
        transition: 'background 0.15s',
      }}
        onMouseEnter={e => (e.currentTarget.style.background = '#f8fafc')}
        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
      >
        {/* 排名 */}
        <div style={{
          width: 20, height: 20, borderRadius: '50%', flexShrink: 0,
          background: rank === 1 ? '#c9a84c' : rank === 2 ? '#9ca3af' : rank === 3 ? '#c97a4c' : '#e5e7eb',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 10, fontWeight: 700, color: rank <= 3 ? '#fff' : '#6b7280',
        }}>
          {rank}
        </div>

        {/* 商品信息 + 进度条 */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 11, fontWeight: 600, color: '#1f2937', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            {shortTitle}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginTop: 2 }}>
            <div style={{
              flex: 1, height: 4, background: '#f3f4f6', borderRadius: 2, overflow: 'hidden',
            }}>
              <div style={{
                height: '100%', width: `${barPct}%`,
                background: barColor, borderRadius: 2,
                transition: 'width 0.4s ease',
              }} />
            </div>
            <Tag
              style={{
                margin: 0, padding: '0 4px', fontSize: 10, lineHeight: '16px',
                borderRadius: 3, border: 'none',
                background: item.margin >= 0 ? '#dcfce7' : '#fee2e2',
                color: marginColor, fontWeight: 700, fontFamily: 'var(--font-number)',
                flexShrink: 0,
              }}
            >
              {item.margin >= 0 ? '+' : ''}{item.margin.toFixed(1)}%
            </Tag>
          </div>
        </div>
      </div>
    </Tooltip>
  )
}

// ══════════════════════════════════════════════════════
// 全局总览组件（多账户对比矩阵 + 利润贡献饼图）
// ══════════════════════════════════════════════════════

function GlobalOverview({ data }: { data: CrossAccountData }) {
  const fmt = (v: number) => v.toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 0 })
  const fmtPct = (v: number) => `${v >= 0 ? '+' : ''}${v.toFixed(1)}%`

  // 汇总 KPI 卡片颜色
  const profitColor = data.totalNetProfitCny >= 0 ? '#10b981' : '#ef4444'

  // 饼图数据 — ECharts pie 不支持负数 value，亏损账户用绝对值 + 红色区分
  const pieData = (data.accountBreakdown ?? [])
    .filter(a => a.salesCny > 0)
    .map((a, i) => ({
      name: `${a.accountName}\n${a.marketplaceName}`,
      value: Math.abs(Math.round(a.netProfitCny)),
      isLoss: a.netProfitCny < 0,
      // 亏损账户使用红色系
      itemStyle: a.netProfitCny < 0 ? { color: '#ef4444' } : undefined,
    }))

  // 饼图颜色（仅盈利账户使用）
  const pieColors = ['#1a2744', '#c9a84c', '#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4']

  return (
    <div>
      {/* 全局 KPI 汇总卡片 */}
      <div style={{
        display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 16,
      }}>
        {[
          { label: '总销售额 (¥)', value: `¥ ${fmt(data.totalSalesCny)}`, color: '#1a2744', icon: '💰' },
          { label: '总订单数', value: `${data.totalOrderCount.toLocaleString()} 单`, color: '#c9a84c', icon: '📦' },
          { label: '总广告花费 (¥)', value: `¥ ${fmt(data.totalAdSpendCny)}`, color: '#3b82f6', icon: '📢' },
          { label: '总净利润 (¥)', value: `¥ ${fmt(data.totalNetProfitCny)}`, color: profitColor, icon: '📈' },
        ].map(c => (
          <div key={c.label} className="voyage-card" style={{ padding: '16px 20px', position: 'relative', overflow: 'hidden' }}>
            <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginBottom: 4 }}>{c.label}</div>
            <div style={{ fontSize: 24, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
            <div style={{ position: 'absolute', right: 16, top: '50%', transform: 'translateY(-50%)', fontSize: 28, opacity: 0.15 }}>{c.icon}</div>
          </div>
        ))}
      </div>

      {/* 利润率 + 汇率信息 */}
      <div className="voyage-card" style={{ padding: '10px 20px', marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', gap: 24, fontSize: 13 }}>
          <span>📊 总利润率 <b style={{ color: profitColor, fontFamily: 'var(--font-number)' }}>{data.totalProfitMargin.toFixed(1)}%</b></span>
          <span>💸 总 COGS <b style={{ fontFamily: 'var(--font-number)' }}>¥ {fmt(data.totalCogsCny)}</b></span>
          <span>🏦 总平台费 <b style={{ fontFamily: 'var(--font-number)' }}>¥ {fmt(data.totalFeesCny)}</b></span>
        </div>
        <span style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>
          汇率更新：{data.rateUpdatedAt?.slice(0, 10) ?? '—'} · 基准货币 CNY
        </span>
      </div>

      {/* 双栏：对比矩阵 + 利润贡献饼图 */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: 16 }}>

        {/* 多账户对比矩阵表 */}
        <div className="voyage-card">
          <div className="voyage-card-header">
            <span className="voyage-card-title">📊 多账户 KPI 对比矩阵</span>
          </div>
          <div style={{ padding: '0 16px 16px', overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ background: '#1a2744', color: '#fff' }}>
                  <th style={thStyle}>账户 / 站点</th>
                  <th style={thStyle}>销售额</th>
                  <th style={thStyle}>订单</th>
                  <th style={thStyle}>广告费</th>
                  <th style={thStyle}>净利润</th>
                  <th style={thStyle}>利润率</th>
                  <th style={thStyle}>贡献占比</th>
                </tr>
              </thead>
              <tbody>
                {(data.accountBreakdown ?? []).map((a, i) => (
                  <tr key={`${a.accountId}-${a.marketplaceId}`}
                      style={{ background: i % 2 === 0 ? '#f8fafc' : '#fff', transition: 'background 0.15s' }}
                      onMouseEnter={e => (e.currentTarget.style.background = '#eef2ff')}
                      onMouseLeave={e => (e.currentTarget.style.background = i % 2 === 0 ? '#f8fafc' : '#fff')}
                  >
                    <td style={tdStyle}>
                      <div style={{ fontWeight: 600, color: '#1f2937' }}>{a.accountName}</div>
                      <div style={{ fontSize: 10, color: '#9ca3af' }}>{a.marketplaceName} · {a.originalCurrency}</div>
                    </td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-number)' }}>
                      <div>¥ {fmt(a.salesCny)}</div>
                      <div style={{ fontSize: 10, color: '#9ca3af' }}>{a.originalCurrency} {fmt(a.sales)}</div>
                    </td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-number)', textAlign: 'center' }}>{a.orders}</td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-number)' }}>¥ {fmt(a.adSpendCny)}</td>
                    <td style={{ ...tdStyle, fontFamily: 'var(--font-number)', color: a.netProfitCny >= 0 ? '#10b981' : '#ef4444', fontWeight: 600 }}>
                      ¥ {fmt(a.netProfitCny)}
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'center' }}>
                      <Tag style={{
                        margin: 0, border: 'none', fontWeight: 700, fontSize: 11, fontFamily: 'var(--font-number)',
                        background: a.profitMargin >= 15 ? '#dcfce7' : a.profitMargin >= 0 ? '#fef9c3' : '#fee2e2',
                        color: a.profitMargin >= 15 ? '#10b981' : a.profitMargin >= 0 ? '#d97706' : '#ef4444',
                      }}>
                        {fmtPct(a.profitMargin)}
                      </Tag>
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'center' }}>
                      {/* 贡献条 */}
                      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                        <div style={{ flex: 1, height: 6, background: '#f3f4f6', borderRadius: 3, overflow: 'hidden' }}>
                          <div style={{
                            height: '100%', borderRadius: 3, transition: 'width 0.4s',
                            width: `${Math.min(100, Math.max(0, a.profitSharePct))}%`,
                            background: a.netProfitCny >= 0
                              ? 'linear-gradient(90deg, #10b981, #34d399)'
                              : 'linear-gradient(90deg, #ef4444, #f87171)',
                          }} />
                        </div>
                        <span style={{ fontSize: 10, fontFamily: 'var(--font-number)', color: '#6b7280', minWidth: 32 }}>
                          {a.profitSharePct.toFixed(0)}%
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
                {/* 合计行 */}
                <tr style={{ background: '#1a2744', color: '#fff', fontWeight: 700 }}>
                  <td style={{ ...tdStyle, color: '#fff' }}>合计</td>
                  <td style={{ ...tdStyle, color: '#fff', fontFamily: 'var(--font-number)' }}>¥ {fmt(data.totalSalesCny)}</td>
                  <td style={{ ...tdStyle, color: '#fff', fontFamily: 'var(--font-number)', textAlign: 'center' }}>{data.totalOrderCount}</td>
                  <td style={{ ...tdStyle, color: '#fff', fontFamily: 'var(--font-number)' }}>¥ {fmt(data.totalAdSpendCny)}</td>
                  <td style={{ ...tdStyle, color: data.totalNetProfitCny >= 0 ? '#6ee7b7' : '#fca5a5', fontFamily: 'var(--font-number)' }}>¥ {fmt(data.totalNetProfitCny)}</td>
                  <td style={{ ...tdStyle, color: '#c9a84c', textAlign: 'center', fontFamily: 'var(--font-number)' }}>{data.totalProfitMargin.toFixed(1)}%</td>
                  <td style={{ ...tdStyle, color: '#fff', textAlign: 'center' }}>100%</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        {/* 利润贡献饼图 */}
        <div className="voyage-card">
          <div className="voyage-card-header">
            <span className="voyage-card-title">🏆 利润贡献构成</span>
            <Tooltip title="各账户/站点的净利润占总净利润的百分比（以 CNY 计）">
              <span className="data-delay-badge" style={{ cursor: 'help' }}><InfoCircleOutlined /> CNY 换算</span>
            </Tooltip>
          </div>
          <div style={{ padding: '0 8px 12px' }}>
            {pieData.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无利润数据" style={{ padding: 40 }} />
            ) : (
              <ReactECharts
                option={{
                  backgroundColor: 'transparent',
                  tooltip: {
                    trigger: 'item',
                    backgroundColor: '#1a2744', borderColor: '#2d4078',
                    textStyle: { color: '#e8e8e8', fontSize: 12 },
                    formatter: (p: any) => {
                      const item = pieData[p.dataIndex]
                      const prefix = item?.isLoss ? '<span style="color:#ef4444">亏损</span>' : '盈利'
                      const sign = item?.isLoss ? '-' : ''
                      return `<b>${p.name.replace('\n', ' · ')}</b><br/>${prefix}：${sign}¥ ${p.value.toLocaleString()}<br/>占比：${p.percent}%`
                    },
                  },
                  legend: {
                    orient: 'vertical', right: 10, top: 'center',
                    textStyle: { color: '#6b7280', fontSize: 11 },
                    formatter: (name: string) => name.replace('\n', ' '),
                  },
                  series: [{
                    type: 'pie', radius: ['40%', '70%'], center: ['35%', '50%'],
                    avoidLabelOverlap: true,
                    itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
                    label: {
                      show: true, fontSize: 11, color: '#6b7280',
                      formatter: '{d}%',
                    },
                    emphasis: {
                      label: { show: true, fontSize: 14, fontWeight: 'bold' },
                      itemStyle: { shadowBlur: 10, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.2)' },
                    },
                    data: pieData,
                    color: pieColors,
                  }],
                }}
                style={{ height: 320 }}
                opts={{ renderer: 'canvas' }}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// 表格通用样式
const thStyle: React.CSSProperties = {
  padding: '8px 10px', textAlign: 'left', fontSize: 11, fontWeight: 600,
  whiteSpace: 'nowrap', borderBottom: '2px solid #c9a84c',
}
const tdStyle: React.CSSProperties = {
  padding: '8px 10px', borderBottom: '1px solid #f0f0f0',
  fontSize: 12, whiteSpace: 'nowrap',
}
