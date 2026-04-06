import { useState, useEffect, useRef, useCallback } from 'react'
import { DatePicker, Select, Button, Table, Tag, Spin, Empty, Alert, Tooltip } from 'antd'
import { DownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import * as echarts from 'echarts/core'
import { LineChart as ELineChart, BarChart as EBarChart } from 'echarts/charts'
import {
  TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { useAppStore } from '../stores/appStore'
import { message } from 'antd'

// @ts-ignore
import { GetDailyTrend, GetSalesByAsin, ExportDataCSV, OpenFileInExplorer } from '../../wailsjs/go/main/App'
import { getAmazonDomain, getCurrencySymbol } from '../utils/marketplaceUtils'

echarts.use([ELineChart, EBarChart, TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent, CanvasRenderer])

const { RangePicker } = DatePicker

interface DailyPoint {
  date: string; sales: number; orders: number; units: number
  pageViews: number; sessions: number; conversionRate: number
}

interface AsinRow {
  asin: string; title: string; sku: string
  sales: number; units: number; pageViews: number
  sessions: number; conversionRate: number; buyBoxPercentage: number
}

const METRIC_OPTIONS = [
  { value: 'sales', label: '销售额' },
  { value: 'orders', label: '订单数' },
  { value: 'units', label: '销量(件)' },
  { value: 'pageViews', label: '页面浏览' },
  { value: 'sessions', label: '访客数' },
  { value: 'conversionRate', label: '转化率%' },
]

export default function Sales() {
  const { activeAccountId, activeMarketplaceId, marketplaces } = useAppStore()
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)

  // 根据当前站点获取货币符号
  const activeMarketplace = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currencyCode = activeMarketplace?.currencyCode ?? 'USD'
  const sym = getCurrencySymbol(currencyCode)
  const domain = getAmazonDomain(activeMarketplace?.countryCode)

  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([
    dayjs().subtract(29, 'day'), dayjs().subtract(2, 'day'),
  ])
  const [metric, setMetric] = useState('sales')
  const [trend, setTrend] = useState<DailyPoint[]>([])
  const [prevTrend, setPrevTrend] = useState<DailyPoint[]>([])
  const [showCompare, setShowCompare] = useState(false)
  const [asinTop, setAsinTop] = useState<AsinRow[]>([])
  const [loading, setLoading] = useState(false)
  const [exportLoading, setExportLoading] = useState(false)

  // 清理 ECharts 实例
  useEffect(() => {
    return () => { chartInstance.current?.dispose(); chartInstance.current = null }
  }, [])

  const updateChart = useCallback((data: DailyPoint[], m: string, prev?: DailyPoint[], compare?: boolean) => {
    if (!chartInstance.current || !data.length) return
    const dates = data.map(d => d.date?.slice(5))
    const values = data.map(d => (d as any)[m] ?? 0)
    const prevValues = (prev ?? []).map(d => (d as any)[m] ?? 0)

    const formatVal = (v: number) =>
      m === 'sales' ? `${sym}${Number(v).toFixed(2)}`
      : m.includes('Rate') ? `${Number(v).toFixed(1)}%`
      : Number(v).toLocaleString()

    const series: any[] = [{
      name: '当期', type: 'line', data: values, smooth: true,
      lineStyle: { width: 2, color: '#1a2744' },
      areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
        colorStops: [{ offset: 0, color: 'rgba(26,39,68,0.15)' }, { offset: 1, color: 'rgba(26,39,68,0.01)' }] } },
      symbol: 'circle', symbolSize: 4, itemStyle: { color: '#1a2744' },
    }]

    if (compare && prevValues.length > 0) {
      series.push({
        name: '上期', type: 'line', data: prevValues, smooth: true,
        lineStyle: { width: 2, color: '#c9a84c', type: 'dashed' },
        symbol: 'circle', symbolSize: 3, itemStyle: { color: '#c9a84c' },
      })
    }

    chartInstance.current.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis', formatter: (params: any[]) =>
        params.map(p => `${p.marker} ${p.seriesName}: ${formatVal(Number(p.value))}`)
          .join('<br/>')
      },
      legend: compare ? { data: ['当期', '上期'], textStyle: { fontSize: 12 }, top: 4 } : { show: false },
      grid: { left: 60, right: 16, top: compare ? 32 : 10, bottom: 40 },
      xAxis: { type: 'category', data: dates, axisLabel: { fontSize: 11 }, axisLine: { lineStyle: { color: '#e5e7eb' } } },
      yAxis: { type: 'value', axisLabel: { fontSize: 11,
        formatter: (v: number) => m === 'sales' ? `${sym}${v.toLocaleString()}` : v.toLocaleString() } },
      dataZoom: [{ type: 'inside', start: 0, end: 100 }],
      series,
    }, true)
  }, [])

  const loadData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const start = dateRange[0].format('YYYY-MM-DD')
      const end   = dateRange[1].format('YYYY-MM-DD')
      const dayDiff   = dateRange[1].diff(dateRange[0], 'day')
      const prevEnd   = dateRange[0].subtract(1, 'day').format('YYYY-MM-DD')
      const prevStart = dateRange[0].subtract(dayDiff + 1, 'day').format('YYYY-MM-DD')

      const [trendData, asinData, prevData] = await Promise.all([
        GetDailyTrend(activeAccountId, activeMarketplaceId, start, end),
        GetSalesByAsin(activeAccountId, activeMarketplaceId, start, end, 20),
        GetDailyTrend(activeAccountId, activeMarketplaceId, prevStart, prevEnd),
      ])
      const t = (trendData ?? []) as unknown as DailyPoint[]
      const pv = (prevData ?? []) as unknown as DailyPoint[]
      setTrend(t)
      setPrevTrend(pv)
      setAsinTop((asinData ?? []) as unknown as AsinRow[])
      updateChart(t, metric, pv, showCompare)
    } finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId, dateRange, metric, updateChart, showCompare, sym])

  // ref callback：避免条件渲染导致 echarts.init 不执行
  const chartRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (node && !chartInstance.current) {
      chartInstance.current = echarts.init(node, undefined, { renderer: 'canvas' })
      // 如果趋势数据已加载，立即渲染
      if (trend.length > 0) { updateChart(trend, metric, prevTrend, showCompare) }
    }
  }, [trend, metric, prevTrend, showCompare, updateChart])

  useEffect(() => { loadData() }, [activeAccountId, activeMarketplaceId])
  useEffect(() => { updateChart(trend, metric, prevTrend, showCompare) }, [metric, trend, prevTrend, showCompare, updateChart])

  // 窗口 resize 时更新图表
  useEffect(() => {
    const resizeHandler = () => chartInstance.current?.resize()
    window.addEventListener('resize', resizeHandler)
    return () => window.removeEventListener('resize', resizeHandler)
  }, [])

  // 汇总统计
  const totalSales = trend.reduce((s, d) => s + d.sales, 0)
  const totalOrders = trend.reduce((s, d) => s + d.orders, 0)
  const avgConv = trend.length ? trend.reduce((s, d) => s + d.conversionRate, 0) / trend.length : 0

  const handleExport = async () => {
    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
    setExportLoading(true)
    try {
      const r: any = await ExportDataCSV(activeAccountId, activeMarketplaceId, 'sales',
        dateRange[0].format('YYYY-MM-DD'), dateRange[1].format('YYYY-MM-DD'))
      if (r?.success) {
        message.success(`${r.message} (${r.fileSize})`)
        OpenFileInExplorer(r.path)
      } else {
        message.warning(r?.message ?? '导出失败')
      }
    } catch { message.error('导出失败') } finally { setExportLoading(false) }
  }

  const asinColumns: ColumnsType<AsinRow> = [
    { title: 'ASIN', dataIndex: 'asin', width: 115,
      render: v => <a href={`https://${domain}/dp/${v}`} target="_blank" rel="noopener noreferrer"
        style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</a> },
    { title: '商品名称', dataIndex: 'title', ellipsis: true },
    { title: `销售额(${currencyCode})`, dataIndex: 'sales', sorter: (a, b) => a.sales - b.sales, defaultSortOrder: 'descend',
      render: v => <span className="amount">{sym}{v.toFixed(2)}</span> },
    { title: '销量', dataIndex: 'units', sorter: (a, b) => a.units - b.units,
      render: v => <span className="num">{v}</span> },
    { title: '页面浏览', dataIndex: 'pageViews', sorter: (a, b) => a.pageViews - b.pageViews,
      render: v => <span className="num">{v.toLocaleString()}</span> },
    { title: '访客数', dataIndex: 'sessions', sorter: (a, b) => a.sessions - b.sessions,
      render: v => <span className="num">{v.toLocaleString()}</span> },
    { title: '转化率', dataIndex: 'conversionRate',
      render: v => `${(v * 100).toFixed(1)}%` },
    { title: 'Buy Box%', dataIndex: 'buyBoxPercentage',
      render: v => {
        const pct = v * 100
        const color = pct >= 90 ? '#10b981' : pct >= 70 ? '#f59e0b' : '#ef4444'
        return <span style={{ color, fontWeight: 600 }}>{pct.toFixed(0)}%</span>
      }},
  ]

  return (
    <div>
      {/* ── 页面标题 + 工具栏 ── */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="page-title">📈 销售分析</div>
          <div className="page-subtitle">
            {activeMarketplaceId ? `站点: ${activeMarketplaceId}` : '请选择站点'}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <Tag color="orange" icon={<span>⏰</span>}> Data Kiosk 数据 T+2 延迟</Tag>
          <RangePicker
            value={dateRange}
            onChange={(v) => v && setDateRange(v as any)}
            disabledDate={d => d.isAfter(dayjs().subtract(2, 'day'))}
            allowClear={false}
            style={{ width: 240 }}
          />
          <Select value={metric} onChange={setMetric} style={{ width: 130 }} options={METRIC_OPTIONS} />
          <Tooltip title={showCompare ? '关闭对比' : '与上期对比'}>
            <Button
              onClick={() => setShowCompare(c => !c)}
              type={showCompare ? 'primary' : 'default'}
              style={showCompare ? { background: '#c9a84c', borderColor: '#c9a84c' } : {}}
            >
              ↔ 对比上期
            </Button>
          </Tooltip>
          <Tooltip title="导出 CSV">
            <Button icon={<DownloadOutlined />} loading={exportLoading} onClick={handleExport} />
          </Tooltip>
          <Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={loadData}
            style={{ background: '#1a2744' }}>刷新</Button>
        </div>
      </div>

      {/* ── 汇总 KPI ── */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 16 }}>
        {[
          { label: '期间销售额', value: `${sym}${totalSales.toFixed(2)}`, gradient: 'var(--gradient-sales)' },
          { label: '期间订单量', value: `${totalOrders} 单`, gradient: 'var(--gradient-orders)' },
          { label: '平均转化率', value: `${(avgConv * 100).toFixed(2)}%`, gradient: 'var(--gradient-ads)' },
        ].map(item => (
          <div key={item.label} className="kpi-card" style={{ background: item.gradient }}>
            <div className="kpi-card-label">{item.label}</div>
            <div className="kpi-card-value">{item.value}</div>
          </div>
        ))}
      </div>

      {/* ── 趋势图 ── */}
      <div className="voyage-card" style={{ marginBottom: 16 }}>
        <div className="voyage-card-header">
          <span className="voyage-card-title">📊 趋势图</span>
        </div>
        <div className="voyage-card-body">
          {loading ? <div className="voyage-loading"><Spin /> 加载数据...</div>
            : !activeAccountId ? (
              <div className="no-data-hint">
                <div className="no-data-hint-icon">🏪</div>
                <div className="no-data-hint-text">尚未配置店铺账户</div>
                <div className="no-data-hint-sub">请前往系统设置 → 添加您的亚马逊卖家账户</div>
              </div>
            ) : trend.length === 0 ? (
              <div className="no-data-hint">
                <div className="no-data-hint-icon">📭</div>
                <div className="no-data-hint-text">暂无数据，请同步后查看</div>
              </div>
            ) : <div ref={chartRefCallback} style={{ height: 280, width: '100%' }} />}
        </div>
      </div>

      {/* ── ASIN 排行 ── */}
      <div className="voyage-card">
        <div className="voyage-card-header">
          <span className="voyage-card-title">🏆 ASIN 销售排行 Top {asinTop.length}</span>
        </div>
        <div className="voyage-card-body" style={{ padding: 0 }}>
          <Table
            columns={asinColumns}
            dataSource={asinTop}
            rowKey="asin"
            size="small"
            loading={loading}
            locale={{ emptyText: <Empty description="暂无数据" /> }}
            pagination={{ pageSize: 10, showSizeChanger: false }}
            scroll={{ x: 900 }}
          />
        </div>
      </div>
    </div>
  )
}
