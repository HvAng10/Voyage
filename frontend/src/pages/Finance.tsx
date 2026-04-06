import { useState, useEffect, useCallback, useRef } from 'react'
import { DatePicker, Table, Empty, Descriptions, Tag, Button, message, Tabs, Alert } from 'antd'
import { DownloadOutlined, InfoCircleOutlined } from '@ant-design/icons'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent, TooltipComponent, LegendComponent,
  TitleComponent, DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetFinanceSummary, GetSettlements, ExportDataCSV, GetFinanceVATBreakdown, OpenFileInExplorer } from '../../wailsjs/go/main/App'

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent,
  LegendComponent, TitleComponent, DataZoomComponent, CanvasRenderer])

interface FinanceSummary {
  totalRevenue: number; totalRefunds: number; totalFees: number
  totalAdSpend: number; totalCogs: number
  grossProfit: number; netProfit: number; profitMargin: number
  currency: string; dateStart: string; dateEnd: string
}
interface Settlement {
  settlementId: string; startDate: string; endDate: string
  depositDate: string; totalAmount: number; currency: string
}

const { RangePicker } = DatePicker

export default function Finance() {
  const { activeAccountId, activeMarketplaceId, marketplaces } = useAppStore()
  const mp = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currency = mp?.currencyCode ?? 'USD'

  const waterfallRef = useRef<HTMLDivElement>(null)
  const waterfallChart = useRef<echarts.ECharts | null>(null)

  const defaultDates: [dayjs.Dayjs, dayjs.Dayjs] = [dayjs().subtract(30, 'day'), dayjs()]
  const [dates, setDates] = useState<[dayjs.Dayjs, dayjs.Dayjs]>(defaultDates)
  const [summary, setSummary] = useState<FinanceSummary | null>(null)
  const [settlements, setSettlements] = useState<Settlement[]>([])
  const [loading, setLoading] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [vatInfo, setVatInfo] = useState<any>(null)

  // 初始化图表（resize 监听）
  useEffect(() => {
    const resizer = () => waterfallChart.current?.resize()
    window.addEventListener('resize', resizer)
    return () => {
      window.removeEventListener('resize', resizer)
      waterfallChart.current?.dispose(); waterfallChart.current = null
    }
  }, [])

  const updateWaterfallChart = useCallback((s: FinanceSummary) => {
    if (!waterfallChart.current) return
    const items = [
      { name: '销售收入', value: s.totalRevenue,    pos: true  },
      { name: '退款',     value: -s.totalRefunds,   pos: false },
      { name: '平台费用', value: -s.totalFees,       pos: false },
      { name: '广告花费', value: -s.totalAdSpend,    pos: false },
      { name: '商品成本', value: -s.totalCogs,       pos: false },
      { name: '净利润',   value: s.netProfit,        pos: s.netProfit >= 0 },
    ]
    const formatCur = (v: number) => `${currency} ${Math.abs(v).toFixed(0)}`

    waterfallChart.current.setOption({
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        formatter: (params: any[]) => {
          const p = params[0]
          const item = items[p.dataIndex]
          return `<b>${item.name}</b><br/>${formatCur(item.value)}`
        }
      },
      grid: { top: 36, right: 16, bottom: 36, left: 90 },
      xAxis: {
        type: 'category',
        data: items.map(d => d.name),
        axisLabel: { fontSize: 12 },
        axisLine: { lineStyle: { color: '#e5e7eb' } },
      },
      yAxis: {
        type: 'value', name: currency,
        axisLabel: { fontSize: 11, formatter: (v: number) => `${v.toLocaleString()}` },
        splitLine: { lineStyle: { color: '#f0f0f0' } },
      },
      series: [{
        type: 'bar',
        data: items.map((d, i) => ({
          value: Math.abs(d.value),
          itemStyle: {
            color: i === 0 || i === 5 ? (d.pos ? '#10b981' : '#ef4444') : (d.pos ? '#1a2744' : '#ef4444'),
            borderRadius: [4, 4, 0, 0],
          },
        })),
        barWidth: 48,
        label: {
          show: true, position: 'top', fontSize: 11,
          formatter: (p: any) => {
            const item = items[p.dataIndex]
            return (item.value < 0 ? '-' : '') + formatCur(item.value)
          }
        },
      }],
    }, true)
  }, [currency])

  const fetchData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const start = dates[0].format('YYYY-MM-DD')
      const end   = dates[1].format('YYYY-MM-DD')
      const [s, sl] = await Promise.all([
        GetFinanceSummary(activeAccountId, activeMarketplaceId, start, end),
        GetSettlements(activeAccountId),
      ])
      setSummary(s)
      setSettlements((sl ?? []) as unknown as Settlement[])
      if (s) updateWaterfallChart(s as FinanceSummary)
      // VAT 拆分（EU 站）
      try {
        const vat = await GetFinanceVATBreakdown(activeAccountId, activeMarketplaceId, start, end)
        setVatInfo(vat)
      } catch { setVatInfo(null) }
    } catch { setSummary(null); setSettlements([]) } finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId, dates, updateWaterfallChart])

  // 瀑布图 ref callback：避免条件渲染导致 echarts.init 不执行
  const waterfallRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (node && !waterfallChart.current) {
      waterfallChart.current = echarts.init(node, undefined, { renderer: 'canvas' })
      if (summary) { updateWaterfallChart(summary) }
    }
  }, [summary, updateWaterfallChart])

  useEffect(() => { fetchData() }, [activeAccountId, activeMarketplaceId])

  // 导出结算报告 CSV
  const handleExportSettlements = () => {
    if (!settlements.length) { message.warning('暂无结算数据'); return }
    const header = '结算ID,开始日期,结束日期,到账日期,金额,货币\n'
    const rows = settlements.map(s =>
      `"${s.settlementId}","${s.startDate}","${s.endDate}","${s.depositDate}",${s.totalAmount},"${s.currency}"`
    ).join('\n')
    const blob = new Blob(['\uFEFF' + header + rows], { type: 'text/csv;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `voyage_settlements_${dayjs().format('YYYY-MM-DD')}.csv`
    a.click()
    message.success('结算报告导出成功')
  }

  const fmt  = (v: number) => `${currency} ${v.toLocaleString('en-US', { minimumFractionDigits: 2 })}`
  const pct  = (v: number) => `${v.toFixed(1)}%`

  const settlementCols: ColumnsType<Settlement> = [
    { title: '结算 ID', dataIndex: 'settlementId', width: 160, ellipsis: true,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: '结算开始', dataIndex: 'startDate', width: 110 },
    { title: '结算结束', dataIndex: 'endDate', width: 110 },
    { title: '到账日期', dataIndex: 'depositDate', width: 110 },
    { title: '金额', dataIndex: 'totalAmount', sorter: (a, b) => a.totalAmount - b.totalAmount,
      defaultSortOrder: 'descend',
      render: (v: number, r: Settlement) => (
        <span className="amount" style={{ color: v >= 0 ? '#10b981' : '#ef4444', fontWeight: 600 }}>
          {r.currency} {v.toLocaleString('en-US', { minimumFractionDigits: 2 })}
        </span>
      ) },
  ]

  const tabItems = [
    {
      key: 'waterfall',
      label: '💹 利润分解',
      children: (
        <div style={{ padding: 16 }}>
          {!summary
            ? <Empty description="暂无财务数据，同步财务事件后显示" style={{ padding: '60px 0' }} />
            : <div ref={waterfallRefCallback} style={{ height: 300 }} />}
        </div>
      ),
    },
    {
      key: 'summary',
      label: '📋 财务摘要',
      children: summary ? (
        <div style={{ padding: 16 }}>
          {/* KPI 卡片行 */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 20 }}>
            {[
              { label: '销售收入', value: fmt(summary.totalRevenue), color: '#1a2744' },
              { label: '毛利润', value: fmt(summary.grossProfit), color: '#3b82f6' },
              { label: '净利润', value: fmt(summary.netProfit), color: summary.netProfit >= 0 ? '#10b981' : '#ef4444' },
              { label: '净利率', value: pct(summary.profitMargin),
                color: summary.profitMargin >= 15 ? '#10b981' : summary.profitMargin >= 5 ? '#f59e0b' : '#ef4444' },
            ].map(c => (
              <div key={c.label} className="voyage-card" style={{ padding: '14px 18px' }}>
                <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 4 }}>{c.label}</div>
                <div style={{ fontSize: 22, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
              </div>
            ))}
          </div>
          <Descriptions bordered size="small" column={3}>
            <Descriptions.Item label="销售收入">{fmt(summary.totalRevenue)}</Descriptions.Item>
            <Descriptions.Item label="退款金额">
              <span style={{ color: '#ef4444' }}>-{fmt(summary.totalRefunds)}</span>
            </Descriptions.Item>
            <Descriptions.Item label="毛利润">{fmt(summary.grossProfit)}</Descriptions.Item>
            <Descriptions.Item label="平台费用">
              <span style={{ color: '#f59e0b' }}>-{fmt(summary.totalFees)}</span>
            </Descriptions.Item>
            <Descriptions.Item label="广告花费">
              <span style={{ color: '#3b82f6' }}>-{fmt(summary.totalAdSpend)}</span>
            </Descriptions.Item>
            <Descriptions.Item label="商品成本">
              <span style={{ color: '#8b5cf6' }}>-{fmt(summary.totalCogs)}</span>
              {summary.totalCogs === 0 && <Tag color="orange" style={{ marginLeft: 8 }}>未录入成本</Tag>}
            </Descriptions.Item>
            <Descriptions.Item label="净利润" span={2}>
              <span style={{ fontSize: 18, fontWeight: 700,
                color: summary.netProfit >= 0 ? '#10b981' : '#ef4444' }}>
                {fmt(summary.netProfit)}
              </span>
            </Descriptions.Item>
            <Descriptions.Item label="净利率">
              <span style={{ fontSize: 16, fontWeight: 700,
                color: summary.profitMargin >= 15 ? '#10b981' : summary.profitMargin >= 5 ? '#f59e0b' : '#ef4444' }}>
                {pct(summary.profitMargin)}
              </span>
            </Descriptions.Item>
          </Descriptions>
        </div>
      ) : <Empty description="暂无财务摘要" style={{ padding: '60px 0' }} />,
    },
    {
      key: 'settlements',
      label: `📄 结算报告（${settlements.length}）`,
      children: (
        <div>
          <div style={{ padding: '8px 16px', borderBottom: '1px solid var(--color-border-light)',
            display: 'flex', justifyContent: 'flex-end' }}>
            <Button icon={<DownloadOutlined />} size="small" onClick={handleExportSettlements}>
              导出结算 CSV
            </Button>
          </div>
          <Table
            columns={settlementCols}
            dataSource={settlements}
            rowKey="settlementId"
            loading={loading}
            size="small"
            pagination={{ pageSize: 15, showSizeChanger: false, showTotal: t => `共 ${t} 期` }}
          />
        </div>
      ),
    },
    {
      key: 'vat',
      label: '🇪🇺 VAT 税率分析',
      children: (
        <div style={{ padding: 16 }}>
          <Alert type="info" showIcon style={{ marginBottom: 16 }}
            message="欧洲站 VAT 拆分分析：自动识别当前站点所属国家，显示含税/不含税销售拆分。" />
          {vatInfo?.isEU ? (
            <div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 16 }}>
                {[
                  { label: `含税销售额 (${currency})`, value: vatInfo.totalSalesInclVAT?.toFixed(2) ?? '-', color: '#1a2744' },
                  { label: `不含税销售额 (${currency})`, value: vatInfo.totalSalesExclVAT?.toFixed(2) ?? '-', color: '#3b82f6' },
                  { label: `VAT 税额 (${currency})`, value: vatInfo.vatAmount?.toFixed(2) ?? '-', color: '#f59e0b' },
                  { label: `VAT 税率 (${vatInfo.countryCode})`, value: `${vatInfo.vatRate}%`, color: '#8b5cf6' },
                ].map(c => (
                  <div key={c.label} className="voyage-card" style={{ padding: '14px 18px' }}>
                    <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 4 }}>{c.label}</div>
                    <div style={{ fontSize: 22, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
                  </div>
                ))}
              </div>
              <Alert type="warning" showIcon message={`当前站点 ${vatInfo.countryCode} 适用标准 VAT 税率 ${vatInfo.vatRate}%。Amazon 欧洲站销售额默认为含税价格（VAT Inclusive）。`} />
            </div>
          ) : (
            <Empty description="当前站点非欧洲站，无 VAT 拆分数据" style={{ padding: '60px 0' }} />
          )}
        </div>
      ),
    },
  ]

  return (
    <div>
      {/* ── 页头 ── */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="page-title">💰 财务报表</div>
          <div className="page-subtitle">{mp?.name ?? '请选择站点'} · 利润分析 · 结算报告</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <RangePicker value={dates} onChange={v => v && setDates(v as any)} allowClear={false} />
          <Button type="primary" loading={loading} onClick={fetchData} style={{ background: '#1a2744' }}>
            刷新
          </Button>
          <Button icon={<DownloadOutlined />} loading={exporting} onClick={async () => {
            if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
            setExporting(true)
            try {
              const r: any = await ExportDataCSV(activeAccountId, activeMarketplaceId, 'finance',
                dates[0].format('YYYY-MM-DD'), dates[1].format('YYYY-MM-DD'))
              if (r?.success) {
                message.success(`${r.message} (${r.fileSize})`)
                OpenFileInExplorer(r.path)
              } else {
                message.warning(r?.message ?? '导出失败')
              }
            } catch { message.error('导出失败') } finally { setExporting(false) }
          }}>导出利润报表</Button>
        </div>
      </div>

      {/* ── 数据延迟提示 ── */}
      <Alert
        message={
          <span>
            <InfoCircleOutlined style={{ marginRight: 6 }} />
            财务事件数据延迟约 <strong>T+1 ~ T+3</strong>（Amazon Financial Events API 固有延迟），近 3 天的数据可能不完整。
          </span>
        }
        type="info" showIcon={false}
        style={{ marginBottom: 12, fontSize: 12, padding: '6px 12px' }}
      />

      {/* ── 内容 Tab ── */}
      <div className="voyage-card" style={{ padding: 0 }}>
        <Tabs items={tabItems} size="middle" style={{ margin: 0 }}
          tabBarStyle={{ padding: '0 16px', marginBottom: 0 }}
          onTabClick={key => { if (key === 'waterfall') { setTimeout(() => waterfallChart.current?.resize(), 100) } }}
        />
      </div>
    </div>
  )
}
