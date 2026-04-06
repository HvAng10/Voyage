import { useState, useEffect, useCallback, useRef } from 'react'
import { Table, Tag, DatePicker, Button, Empty, Tabs, Tooltip, Select, Alert, message, InputNumber } from 'antd'
import { ReloadOutlined, DownloadOutlined, InfoCircleOutlined } from '@ant-design/icons'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetAdCampaigns, GetDailyTrend, GetAdKeywords, GetAdTargets, GetSearchTermStats, GetAdPlacementStats, GetPlacementSummary, ExportDataCSV, GetBidSuggestions, OpenFileInExplorer } from '../../wailsjs/go/main/App'

echarts.use([LineChart, BarChart, TitleComponent, TooltipComponent,
  GridComponent, LegendComponent, DataZoomComponent, CanvasRenderer])

const { RangePicker } = DatePicker

interface Campaign {
  campaignId: string; name: string; state: string; dailyBudget: number
  totalCost: number; totalSales: number; totalClicks: number
  impressions: number; acos: number; roas: number; ctr: number
}

interface KeywordRow {
  campaignName: string; adGroupName: string; keywordText: string
  matchType: string; state: string
  impressions: number; clicks: number; cost: number; sales: number
  orders: number; acos: number; ctr: number; cpc: number
}

export default function Advertising() {
  const { activeAccountId, activeMarketplaceId, marketplaces } = useAppStore()
  const mp = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currency = mp?.currencyCode ?? 'USD'

  const trendRef = useRef<HTMLDivElement>(null)
  const trendChart = useRef<echarts.ECharts | null>(null)
  const compRef = useRef<HTMLDivElement>(null)
  const compChart = useRef<echarts.ECharts | null>(null)

  const defaultDates: [dayjs.Dayjs, dayjs.Dayjs] = [dayjs().subtract(33, 'day'), dayjs().subtract(3, 'day')]
  const [dates, setDates] = useState<[dayjs.Dayjs, dayjs.Dayjs]>(defaultDates)
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [keywords, setKeywords] = useState<KeywordRow[]>([])
  const [adTrend, setAdTrend] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [exportLoading, setExportLoading] = useState(false)
  const [kwLoading, setKwLoading] = useState(false)
  const [searchTerms, setSearchTerms] = useState<any[]>([])
  const [stLoading, setStLoading] = useState(false)

  // 版位数据
  const [placementData, setPlacementData] = useState<any[]>([])
  const [placementSummary, setPlacementSummary] = useState<any[]>([])
  const [placementLoading, setPlacementLoading] = useState(false)

  // 初始化 ECharts
  useEffect(() => {
    if (trendRef.current && !trendChart.current) {
      trendChart.current = echarts.init(trendRef.current, undefined, { renderer: 'canvas' })
    }
    if (compRef.current && !compChart.current) {
      compChart.current = echarts.init(compRef.current, undefined, { renderer: 'canvas' })
    }
    const resizer = () => { trendChart.current?.resize(); compChart.current?.resize() }
    window.addEventListener('resize', resizer)
    return () => {
      window.removeEventListener('resize', resizer)
      trendChart.current?.dispose(); trendChart.current = null
      compChart.current?.dispose(); compChart.current = null
    }
  }, [])

  const updateTrendChart = useCallback((data: any[]) => {
    if (!trendChart.current || !data.length) return
    trendChart.current.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis', axisPointer: { type: 'cross' } },
      legend: { data: ['广告花费', 'ACoS (%)'], textStyle: { fontSize: 12 }, top: 4 },
      grid: { top: 36, right: 60, bottom: 40, left: 70 },
      dataZoom: [{ type: 'inside', start: 0, end: 100 }],
      xAxis: { type: 'category', data: data.map(d => d.date?.slice(5)),
        axisLabel: { fontSize: 11 }, axisLine: { lineStyle: { color: '#e5e7eb' } } },
      yAxis: [
        { type: 'value', name: `花费(${currency})`, nameTextStyle: { fontSize: 11 },
          axisLabel: { fontSize: 11 }, splitLine: { lineStyle: { color: '#f0f0f0' } } },
        { type: 'value', name: 'ACoS%', nameTextStyle: { fontSize: 11 },
          axisLabel: { fontSize: 11 }, splitLine: { show: false }, min: 0, max: 80 },
      ],
      series: [
        { name: '广告花费', type: 'bar', yAxisIndex: 0,
          data: data.map(d => +(d.adSpend ?? 0).toFixed(2)),
          itemStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [{ offset: 0, color: 'rgba(12,166,148,0.85)' }, { offset: 1, color: 'rgba(12,166,148,0.2)' }] },
            borderRadius: [2, 2, 0, 0] }, barMaxWidth: 20 },
        { name: 'ACoS (%)', type: 'line', smooth: true, yAxisIndex: 1,
          data: data.map(d => +(d.acos ?? 0).toFixed(1)),
          lineStyle: { color: '#c9a84c', width: 2 }, itemStyle: { color: '#c9a84c' }, symbol: 'circle', symbolSize: 3 },
      ],
    }, true)
  }, [currency])

  // 当期 vs 上期对比图
  const updateCompChart = useCallback((current: any[], prev: any[]) => {
    if (!compChart.current) return
    const dates = current.map(d => d.date?.slice(5))
    compChart.current.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis' },
      legend: { data: ['当期花费', '上期花费'], textStyle: { fontSize: 12 }, top: 4 },
      grid: { top: 36, right: 20, bottom: 40, left: 60 },
      dataZoom: [{ type: 'inside' }],
      xAxis: { type: 'category', data: dates, axisLabel: { fontSize: 11 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 11 } },
      series: [
        { name: '当期花费', type: 'line', smooth: true,
          data: current.map(d => +(d.adSpend ?? 0).toFixed(2)),
          lineStyle: { color: '#1a2744', width: 2 },
          areaStyle: { color: 'rgba(26,39,68,0.08)' }, symbolSize: 4 },
        { name: '上期花费', type: 'line', smooth: true,
          data: prev.map(d => +(d.adSpend ?? 0).toFixed(2)),
          lineStyle: { color: '#c9a84c', width: 2, type: 'dashed' },
          symbolSize: 4 },
      ],
    }, true)
  }, [])

  const fetchData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const start = dates[0].format('YYYY-MM-DD')
      const end = dates[1].format('YYYY-MM-DD')
      // 计算上期（相同天数）
      const dayDiff = dates[1].diff(dates[0], 'day')
      const prevEnd = dates[0].subtract(1, 'day').format('YYYY-MM-DD')
      const prevStart = dates[0].subtract(dayDiff + 1, 'day').format('YYYY-MM-DD')

      const [c, t, prevT] = await Promise.all([
        GetAdCampaigns(activeAccountId, activeMarketplaceId, start, end),
        GetDailyTrend(activeAccountId, activeMarketplaceId, start, end),
        GetDailyTrend(activeAccountId, activeMarketplaceId, prevStart, prevEnd),
      ])
      setCampaigns((c ?? []) as Campaign[])
      const trend = (t ?? []) as any[]
      const prevTrend = (prevT ?? []) as any[]
      setAdTrend(trend)
      // 更新图表
      updateTrendChart(trend)
      updateCompChart(trend, prevTrend)
    } catch { setCampaigns([]) } finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId, dates, updateTrendChart, updateCompChart])

  const fetchKeywords = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setKwLoading(true)
    try {
      const start = dates[0].format('YYYY-MM-DD')
      const end = dates[1].format('YYYY-MM-DD')
      const kw = await GetAdKeywords(activeAccountId, activeMarketplaceId, start, end, 100)
      setKeywords((kw ?? []) as unknown as KeywordRow[])
    } catch { setKeywords([]) } finally { setKwLoading(false) }
  }, [activeAccountId, activeMarketplaceId, dates])

  useEffect(() => { fetchData() }, [activeAccountId, activeMarketplaceId])

  // KPI 统计
  const totalSpend  = campaigns.reduce((s, c) => s + c.totalCost, 0)
  const totalSales  = campaigns.reduce((s, c) => s + c.totalSales, 0)
  const totalClicks = campaigns.reduce((s, c) => s + c.totalClicks, 0)
  const avgAcos     = totalSales > 0 ? (totalSpend / totalSales) * 100 : 0
  const avgRoas     = totalSpend > 0 ? totalSales / totalSpend : 0

  // 活动列表列
  const campaignCols: ColumnsType<Campaign> = [
    { title: '活动名称', dataIndex: 'name', ellipsis: true },
    { title: '状态', dataIndex: 'state', width: 80,
      render: v => <Tag color={v === 'enabled' ? 'green' : 'orange'}>{v === 'enabled' ? '投放中' : '已暂停'}</Tag> },
    { title: `日预算`, dataIndex: 'dailyBudget', render: v => `${currency} ${v.toFixed(2)}` },
    { title: '展示量', dataIndex: 'impressions', sorter: (a, b) => a.impressions - b.impressions,
      render: v => v.toLocaleString() },
    { title: '点击量', dataIndex: 'totalClicks', sorter: (a, b) => a.totalClicks - b.totalClicks,
      render: v => v.toLocaleString() },
    { title: '花费', dataIndex: 'totalCost', sorter: (a, b) => a.totalCost - b.totalCost, defaultSortOrder: 'descend',
      render: v => <span className="amount">{currency} {v.toFixed(2)}</span> },
    { title: '广告销售额', dataIndex: 'totalSales', sorter: (a, b) => a.totalSales - b.totalSales,
      render: v => <span className="amount">{currency} {v.toFixed(2)}</span> },
    { title: 'ACoS', dataIndex: 'acos', sorter: (a, b) => a.acos - b.acos,
      render: v => <span style={{ color: v > 30 ? '#ef4444' : v > 20 ? '#f59e0b' : '#10b981', fontWeight: 600 }}>{v.toFixed(1)}%</span> },
    { title: 'ROAS', dataIndex: 'roas', sorter: (a, b) => a.roas - b.roas,
      render: v => <span className={v >= 3 ? 'trend-up' : 'trend-down'}>{v.toFixed(2)}x</span> },
  ]

  // 关键词列表列
  const kwCols: ColumnsType<KeywordRow> = [
    { title: '关键词', dataIndex: 'keywordText', ellipsis: true,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: '匹配类型', dataIndex: 'matchType', width: 90,
      render: v => <Tag>{v === 'exact' ? '精准' : v === 'phrase' ? '词组' : '广泛'}</Tag> },
    { title: '状态', dataIndex: 'state', width: 80,
      render: v => <Tag color={v === 'enabled' ? 'green' : 'default'}>{v === 'enabled' ? '启用' : '暂停'}</Tag> },
    { title: '展示量', dataIndex: 'impressions', sorter: (a, b) => a.impressions - b.impressions,
      render: v => v.toLocaleString() },
    { title: '点击量', dataIndex: 'clicks', sorter: (a, b) => a.clicks - b.clicks,
      render: v => v.toLocaleString() },
    { title: 'CTR', dataIndex: 'ctr', render: v => `${v.toFixed(2)}%` },
    { title: '花费', dataIndex: 'cost', sorter: (a, b) => a.cost - b.cost, defaultSortOrder: 'descend',
      render: v => <span className="amount">{currency} {v.toFixed(2)}</span> },
    { title: 'CPC', dataIndex: 'cpc', render: v => `${currency} ${v.toFixed(3)}` },
    { title: '广告销售额', dataIndex: 'sales', sorter: (a, b) => a.sales - b.sales,
      render: v => <span className="amount">{currency} {v.toFixed(2)}</span> },
    { title: 'ACoS', dataIndex: 'acos', sorter: (a, b) => a.acos - b.acos,
      render: v => <span style={{ color: v > 30 ? '#ef4444' : v > 20 ? '#f59e0b' : '#10b981', fontWeight: 600 }}>
        {v > 0 ? `${v.toFixed(1)}%` : '-'}
      </span> },
    { title: '广告活动', dataIndex: 'campaignName', ellipsis: true,
      render: v => <Tooltip title={v}><span style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{v}</span></Tooltip> },
  ]

  const tabItems = [
    {
      key: 'trend',
      label: '📊 广告趋势',
      children: (
        <div style={{ padding: 16 }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                每日广告花费 + ACoS
              </div>
              <div ref={trendRef} style={{ height: 220 }} />
            </div>
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                当期 vs 上期广告花费对比
              </div>
              <div ref={compRef} style={{ height: 220 }} />
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'campaigns',
      label: `🎯 活动明细（${campaigns.length}）`,
      children: (
        <Table columns={campaignCols} dataSource={campaigns} rowKey="campaignId"
          loading={loading} size="small"
          pagination={{ pageSize: 20, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
          scroll={{ x: 1000 }}
          locale={{ emptyText: <Empty description="暂无广告数据" /> }}
        />
      ),
    },
    {
      key: 'keywords',
      label: '🔑 关键词分析',
      children: (
        <div>
          <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--color-border-light)' }}>
            <Button type="primary" size="small" onClick={fetchKeywords} loading={kwLoading}
              style={{ background: '#1a2744' }}>
              加载关键词数据
            </Button>
            <span style={{ marginLeft: 12, fontSize: 12, color: 'var(--color-text-muted)' }}>
              显示花费最高的 Top 100 关键词
            </span>
          </div>
          <Table columns={kwCols} dataSource={keywords} rowKey={r => r.campaignName + r.keywordText}
            loading={kwLoading} size="small"
            pagination={{ pageSize: 20, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
            scroll={{ x: 1100 }}
            locale={{ emptyText: <Empty description="点击「加载关键词数据」获取数据" /> }}
          />
        </div>
      ),
    },
    {
      key: 'searchterms',
      label: '🔍 搜索词报告',
      children: (
        <div>
          <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--color-border-light)', display: 'flex', alignItems: 'center', gap: 12 }}>
            <Button type="primary" size="small" onClick={async () => {
              if (!activeAccountId) return
              setStLoading(true)
              try {
                // T+3 延迟：结束日期为 4 天前
                const end = dates[1].subtract(3, 'day').format('YYYY-MM-DD')
                const start = dates[0].format('YYYY-MM-DD')
                const list = await GetSearchTermStats(activeAccountId, start, end)
                setSearchTerms(list || [])
              } catch { message.error('搜索词数据加载失败') } finally { setStLoading(false) }
            }} loading={stLoading} style={{ background: '#1a2744' }}>
              加载搜索词数据
            </Button>
            <Tooltip title="搜索词数据存在约 T+3 的固有延迟，查询最近 3 天内的数据可能尚不完整">
              <Tag color="blue" style={{ cursor: 'help', fontSize: 11 }}>⚠️ 数据延迟 T+3</Tag>
            </Tooltip>
            {searchTerms.length > 0 && (
              <span style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>
                共 {searchTerms.length} 条搜索词
                　<span style={{ color: '#ef4444' }}>🚫 {searchTerms.filter((s: any) => s.optTag === 'negate').length} 个否词建议</span>
                　<span style={{ color: '#c9a84c' }}>⭐ {searchTerms.filter((s: any) => s.optTag === 'exact').length} 个精准词建议</span>
              </span>
            )}
          </div>
          <Table
            columns={[
              { title: '建议', dataIndex: 'optTag', width: 90, fixed: 'left',
                render: (v: string) => {
                  if (v === 'negate') return <Tag color="red">否词</Tag>
                  if (v === 'exact') return <Tag color="gold">加精准</Tag>
                  return <Tag color="green">正常</Tag>
                }},
              { title: '买家搜索词', dataIndex: 'searchTerm', ellipsis: true,
                render: (v: string) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
              { title: '投放词', dataIndex: 'keywordText', ellipsis: true, width: 160,
                render: (v: string) => <span style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{v || '-'}</span> },
              { title: '匹配', dataIndex: 'matchType', width: 72,
                render: (v: string) => <Tag>{({BROAD:'广泛',PHRASE:'词组',EXACT:'精准'} as any)[v] || v}</Tag> },
              { title: '展示量', dataIndex: 'impressions', sorter: (a: any, b: any) => a.impressions - b.impressions,
                render: (v: number) => v.toLocaleString() },
              { title: '点击量', dataIndex: 'clicks', sorter: (a: any, b: any) => a.clicks - b.clicks,
                render: (v: number) => v.toLocaleString() },
              { title: 'CTR', dataIndex: 'ctr', render: (v: number) => `${v.toFixed(2)}%` },
              { title: '花费', dataIndex: 'cost', sorter: (a: any, b: any) => a.cost - b.cost, defaultSortOrder: 'descend',
                render: (v: number) => <span className="amount">{currency} {v.toFixed(2)}</span> },
              { title: 'CPC', dataIndex: 'cpc', render: (v: number) => `${currency} ${v.toFixed(3)}` },
              { title: '广告销售额', dataIndex: 'sales7d', sorter: (a: any, b: any) => a.sales7d - b.sales7d,
                render: (v: number) => <span className="amount">{currency} {v.toFixed(2)}</span> },
              { title: '转化率', dataIndex: 'cvr', render: (v: number) => `${v.toFixed(1)}%` },
              { title: 'ACoS', dataIndex: 'acos', sorter: (a: any, b: any) => a.acos - b.acos,
                render: (v: number) => <span style={{ color: v > 30 ? '#ef4444' : v > 20 ? '#f59e0b' : '#10b981', fontWeight: 600 }}>
                  {v > 0 ? `${v.toFixed(1)}%` : '-'}
                </span> },
            ]}
            dataSource={searchTerms}
            rowKey={(r: any) => r.searchTerm + r.keywordText}
            loading={stLoading}
            size="small"
            pagination={{ pageSize: 20, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
            scroll={{ x: 1300 }}
            locale={{ emptyText: <Empty description='点击「加载搜索词数据」，系统将查询 T+3 前历史数据' /> }}
            rowClassName={(r: any) => r.optTag === 'negate' ? 'row-critical' : r.optTag === 'exact' ? 'row-warning' : ''}
          />
        </div>
      ),
    },
    {
      key: 'placement',
      label: `🎯 版位效果`,
      children: (
        <div>
          {/* 版位汇总卡片 */}
          {placementSummary.length > 0 && (
            <div style={{ display: 'grid', gridTemplateColumns: `repeat(${placementSummary.length}, 1fr)`, gap: 12, marginBottom: 16, padding: '16px 16px 0' }}>
              {placementSummary.map((ps: any) => (
                <div key={ps.placement} className="voyage-card" style={{ padding: '12px 16px' }}>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                    {({'Top of Search': '🔝 搜索顶部', 'Product Pages': '📦 商品页面', 'Rest of Search': '🔍 其他位置'} as any)[ps.placement] || ps.placement}
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6, fontSize: 11 }}>
                    <div>花费 <strong style={{ fontFamily: 'var(--font-number)' }}>{currency} {(ps.cost || 0).toFixed(2)}</strong></div>
                    <div>销售额 <strong style={{ fontFamily: 'var(--font-number)' }}>{currency} {(ps.sales || 0).toFixed(2)}</strong></div>
                    <div>ACoS <strong style={{ color: (ps.acos || 0) > 30 ? '#ef4444' : (ps.acos || 0) > 20 ? '#f59e0b' : '#10b981', fontFamily: 'var(--font-number)' }}>{(ps.acos || 0).toFixed(1)}%</strong></div>
                    <div>CVR <strong style={{ fontFamily: 'var(--font-number)' }}>{(ps.cvr || 0).toFixed(1)}%</strong></div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* 版位明细表格 */}
          <Table
            columns={[
              { title: '版位', dataIndex: 'placement', width: 140,
                render: (v: string) => <Tag color={v === 'Top of Search' ? 'blue' : v === 'Product Pages' ? 'purple' : 'default'}>
                  {({'Top of Search': '搜索顶部', 'Product Pages': '商品页面', 'Rest of Search': '其他位置'} as any)[v] || v}
                </Tag> },
              { title: '广告活动', dataIndex: 'campaignName', ellipsis: true },
              { title: '展示量', dataIndex: 'impressions', sorter: (a: any, b: any) => a.impressions - b.impressions,
                render: (v: number) => v.toLocaleString() },
              { title: '点击量', dataIndex: 'clicks', sorter: (a: any, b: any) => a.clicks - b.clicks,
                render: (v: number) => v.toLocaleString() },
              { title: 'CTR', dataIndex: 'ctr', render: (v: number) => `${(v || 0).toFixed(2)}%` },
              { title: '花费', dataIndex: 'cost', sorter: (a: any, b: any) => a.cost - b.cost, defaultSortOrder: 'descend',
                render: (v: number) => <span className="amount">{currency} {(v || 0).toFixed(2)}</span> },
              { title: 'CPC', dataIndex: 'cpc', render: (v: number) => `${currency} ${(v || 0).toFixed(3)}` },
              { title: '广告销售', dataIndex: 'sales7d', sorter: (a: any, b: any) => a.sales7d - b.sales7d,
                render: (v: number) => <span className="amount">{currency} {(v || 0).toFixed(2)}</span> },
              { title: 'ACoS', dataIndex: 'acos', sorter: (a: any, b: any) => a.acos - b.acos,
                render: (v: number) => <span style={{ color: (v||0) > 30 ? '#ef4444' : (v||0) > 20 ? '#f59e0b' : '#10b981', fontWeight: 600 }}>
                  {(v||0) > 0 ? `${(v||0).toFixed(1)}%` : '-'}
                </span> },
              { title: 'CVR', dataIndex: 'cvr', render: (v: number) => `${(v || 0).toFixed(1)}%` },
            ]}
            dataSource={placementData}
            rowKey={(r: any) => r.campaignId + r.placement}
            loading={placementLoading}
            size="small"
            pagination={{ pageSize: 20, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
            scroll={{ x: 1200 }}
          />
        </div>
      ),
    },
    {
      key: 'bids',
      label: '💰 竞价建议',
      children: (
        <BidSuggestPanel accountId={activeAccountId} marketplaceId={activeMarketplaceId} dates={dates} currency={currency} />
      ),
    },
  ]

  return (
    <div>
      {/* ── 页头 ── */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="page-title">📢 广告分析</div>
          <div className="page-subtitle">Sponsored Products · 活动、关键词、趋势多维分析</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <RangePicker value={dates} onChange={v => v && setDates(v as any)} allowClear={false} />
          <Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={fetchData}
            style={{ background: '#1a2744' }}>刷新</Button>
          <Button icon={<DownloadOutlined />} loading={exportLoading} onClick={async () => {
            if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
            setExportLoading(true)
            try {
              const r: any = await ExportDataCSV(activeAccountId, activeMarketplaceId, 'advertising',
                dates[0].format('YYYY-MM-DD'), dates[1].format('YYYY-MM-DD'))
              if (r?.success) {
                message.success(`${r.message} (${r.fileSize})`)
                OpenFileInExplorer(r.path)
              } else {
                message.warning(r?.message ?? '导出失败')
              }
            } catch { message.error('导出失败') } finally { setExportLoading(false) }
          }}>导出 CSV</Button>
        </div>
      </div>

      {/* ── T+3 延迟提示 ── */}
      <Alert
        message={
          <span>
            <InfoCircleOutlined style={{ marginRight: 6 }} />
            广告数据延迟约 <strong>T+3</strong>（Advertising API 固有延迟），日期范围已自动排除近 3 天不完整数据。
          </span>
        }
        type="info" showIcon={false}
        style={{ marginBottom: 12, fontSize: 12, padding: '6px 12px' }}
      />

      {/* ── KPI 卡片 ── */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 12, marginBottom: 16 }}>
        {[
          { label: '总广告花费', value: `${currency} ${totalSpend.toFixed(2)}`, ok: undefined },
          { label: '广告销售额', value: `${currency} ${totalSales.toFixed(2)}`, ok: undefined },
          { label: '总点击量', value: `${totalClicks.toLocaleString()} 次`, ok: undefined },
          { label: '平均 ACoS', value: `${avgAcos.toFixed(1)}%`, ok: avgAcos < 25 && avgAcos > 0 },
          { label: '平均 ROAS', value: `${avgRoas.toFixed(2)}x`, ok: avgRoas >= 3 },
        ].map(card => (
          <div key={card.label} className="voyage-card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 4 }}>{card.label}</div>
            <div style={{ fontSize: 20, fontWeight: 700, fontFamily: 'var(--font-number)',
              color: card.ok === undefined ? 'var(--color-text)' : card.ok ? '#10b981' : '#ef4444' }}>
              {card.value}
            </div>
          </div>
        ))}
      </div>

      {/* ── 内容 Tab ── */}
      <div className="voyage-card" style={{ padding: 0 }}>
        <Tabs items={tabItems} size="middle" style={{ margin: 0 }}
          tabBarStyle={{ padding: '0 16px', marginBottom: 0 }}
          onTabClick={key => {
            if (key === 'trend') { setTimeout(() => { trendChart.current?.resize(); compChart.current?.resize() }, 100) }
            if (key === 'placement' && placementData.length === 0 && !placementLoading && activeAccountId) {
              setPlacementLoading(true)
              const start = dates[0].format('YYYY-MM-DD')
              const end = dates[1].format('YYYY-MM-DD')
              Promise.all([
                GetAdPlacementStats(activeAccountId, start, end),
                GetPlacementSummary(activeAccountId, start, end),
              ]).then(([data, summary]) => {
                setPlacementData(data || [])
                setPlacementSummary(summary || [])
              }).catch(() => {}).finally(() => setPlacementLoading(false))
            }
          }}
        />
      </div>
    </div>
  )
}

// ── 竞价建议面板 ──
function BidSuggestPanel({ accountId, marketplaceId, dates, currency }: {
  accountId: number | null; marketplaceId: string | null; dates: any[]; currency: string
}) {
  const [bids, setBids] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [targetAcos, setTargetAcos] = useState(25)

  const fetchBids = async () => {
    if (!accountId || !marketplaceId) return
    setLoading(true)
    try {
      const start = dates[0].format('YYYY-MM-DD')
      const end = dates[1].format('YYYY-MM-DD')
      const data = await GetBidSuggestions(accountId, marketplaceId, start, end, targetAcos)
      setBids((data ?? []) as any[])
    } catch { setBids([]) } finally { setLoading(false) }
  }

  const bidCols: ColumnsType<any> = [
    { title: '关键词', dataIndex: 'keywordText', width: 180, ellipsis: true },
    { title: '匹配', dataIndex: 'matchType', width: 70,
      render: (v: string) => <Tag color={v === 'exact' ? 'blue' : v === 'phrase' ? 'green' : 'orange'}>{v}</Tag> },
    { title: '展示', dataIndex: 'histImpressions', width: 70, sorter: (a: any, b: any) => a.histImpressions - b.histImpressions },
    { title: '点击', dataIndex: 'histClicks', width: 60 },
    { title: 'ACoS', dataIndex: 'histAcos', width: 70,
      render: (v: number) => <span style={{ color: v > targetAcos ? '#ef4444' : '#10b981' }}>{v.toFixed(0)}%</span> },
    { title: 'CVR', dataIndex: 'histCvr', width: 60,
      render: (v: number) => `${v.toFixed(1)}%` },
    { title: '当前出价', dataIndex: 'currentBid', width: 80,
      render: (v: number) => `${currency} ${v.toFixed(2)}` },
    { title: '建议出价', dataIndex: 'suggestedBid', width: 90,
      render: (v: number, r: any) => (
        <span style={{ fontWeight: 700, color: r.bidDelta > 0 ? '#10b981' : r.bidDelta < 0 ? '#ef4444' : '#64748b' }}>
          {currency} {v.toFixed(2)}
        </span>
      ) },
    { title: '差值', dataIndex: 'bidDelta', width: 70,
      render: (v: number) => (
        <span style={{ color: v > 0 ? '#10b981' : v < 0 ? '#ef4444' : '#64748b' }}>
          {v > 0 ? '+' : ''}{v.toFixed(2)}
        </span>
      ) },
    { title: '置信度', dataIndex: 'confidence', width: 70,
      render: (v: string) => <Tag color={v === 'high' ? 'green' : v === 'medium' ? 'orange' : 'default'}>{
        v === 'high' ? '高' : v === 'medium' ? '中' : '低'
      }</Tag> },
    { title: '建议原因', dataIndex: 'reason', width: 220, ellipsis: true },
  ]

  return (
    <div style={{ padding: 16 }}>
      <Alert type="info" showIcon style={{ marginBottom: 12 }}
        message="基于历史投放数据计算最优竞价：SuggestedBid = TargetACoS × CVR × ASP。仅供参考，实际应结合市场竞争力调整。" />
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 16 }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>目标 ACoS：</span>
        <InputNumber value={targetAcos} onChange={v => v && setTargetAcos(v)} min={5} max={80}
          suffix="%" style={{ width: 100 }} />
        <Button type="primary" loading={loading} onClick={fetchBids}
          style={{ background: '#1a2744' }}>计算竞价建议</Button>
        <span style={{ fontSize: 12, color: 'var(--color-text-muted)', marginLeft: 'auto' }}>
          {bids.length > 0 && `共 ${bids.length} 个关键词`}
        </span>
      </div>
      <Table
        columns={bidCols}
        dataSource={bids}
        rowKey={(r: any) => r.keywordText + r.matchType + r.campaignName}
        loading={loading}
        size="small"
        pagination={{ pageSize: 20, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
        scroll={{ x: 1100 }}
        locale={{ emptyText: <Empty description="点击「计算竞价建议」获取数据" /> }}
      />
    </div>
  )
}
