import { useState, useEffect, useCallback, useRef } from 'react'
import { Table, Tag, Progress, Tooltip, Select, Button, message, Tabs, Alert, Badge, InputNumber, Spin, Slider, Switch, Popover } from 'antd'
import { ReloadOutlined, DownloadOutlined, ExclamationCircleOutlined, WarningOutlined, CheckCircleOutlined, InfoCircleOutlined, SendOutlined, SettingOutlined, ThunderboltOutlined } from '@ant-design/icons'
import * as echarts from 'echarts/core'
import { PieChart, BarChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ColumnsType } from 'antd/es/table'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetInventoryItems, TriggerSync, ExportDataCSV, GetInventoryAge, GetReturnRateByASIN, GetReturnDetails, GetReplenishmentAdvice, UpdateReplenishmentConfig, GetReturnReasonDistribution, OpenFileInExplorer, GetSeasonConfig, SaveSeasonConfig, ApplyGlobalSeasonFactor } from '../../wailsjs/go/main/App'
import { getAmazonDomain } from '../utils/marketplaceUtils'

echarts.use([PieChart, BarChart, TitleComponent, TooltipComponent, LegendComponent, GridComponent, CanvasRenderer])

interface InventoryItem {
  sellerSku: string; asin: string; title: string
  fulfillableQty: number; inboundQty: number; unsellableQty: number
  reservedQty: number; totalQty: number
  dailySalesAvg: number; estDaysLeft: number
  alertLevel: string; snapshotDate: string
}

interface InventoryAgeItem {
  sku: string; asin: string; productName: string
  qty0to90: number; qty91to180: number; qty181to270: number; qty271to365: number; qtyOver365: number
  totalQty: number; estLtsf: number; agingRatio: number; riskLevel: string
}

interface InventoryAgeSummary {
  snapshotDate: string; totalSkus: number; totalQty: number
  qtyAtRisk: number; qtyOver365: number; estTotalLtsf: number; dataLatencyNote: string
}

interface ReturnRateItem {
  asin: string; sku: string; title: string
  totalReturns: number; totalSold: number; returnRate: number
  topReason: string; dataLatencyNote: string
}

// 数据延迟说明组件
function LatencyBadge({ note }: { note: string }) {
  return (
    <Tooltip title={note}>
      <Tag icon={<InfoCircleOutlined />} color="blue" style={{ fontSize: 11, cursor: 'help' }}>
        含延迟数据
      </Tag>
    </Tooltip>
  )
}

export default function Inventory() {
  const { activeAccountId, activeMarketplaceId, marketplaces, setSyncState } = useAppStore()
  const mp = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currency = mp?.currencyCode ?? 'USD'

  // 根据当前站点生成 Amazon 链接域名
  const domain = getAmazonDomain(mp?.countryCode)

  const pieRef = useRef<HTMLDivElement>(null)
  const pieChart = useRef<echarts.ECharts | null>(null)
  const ageBarRef = useRef<HTMLDivElement>(null)
  const ageBarChart = useRef<echarts.ECharts | null>(null)

  const [activeTab, setActiveTab] = useState('inventory')
  const [items, setItems] = useState<InventoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [filter, setFilter] = useState<'all' | 'warning' | 'critical'>('all')

  // 库龄状态
  const [ageItems, setAgeItems] = useState<InventoryAgeItem[]>([])
  const [ageSummary, setAgeSummary] = useState<InventoryAgeSummary | null>(null)
  const [ageLoading, setAgeLoading] = useState(false)

  // 退货分析状态
  const [returnItems, setReturnItems] = useState<ReturnRateItem[]>([])
  const [returnLoading, setReturnLoading] = useState(false)

  // 补货建议状态
  const [replenishItems, setReplenishItems] = useState<any[]>([])
  const [replenishLoading, setReplenishLoading] = useState(false)
  const [leadTimeDays, setLeadTimeDays] = useState(30)
  // 旺季系数状态
  const [seasonCfg, setSeasonCfg] = useState<any>({ q1Factor: 1.0, q2Factor: 1.0, q3Factor: 1.1, q4Factor: 1.5, primeDayFactor: 1.3, autoApply: false })
  const [savingSeasonCfg, setSavingSeasonCfg] = useState(false)
  const [applyingFactor, setApplyingFactor] = useState(false)

  useEffect(() => {
    const resizer = () => { pieChart.current?.resize(); ageBarChart.current?.resize() }
    window.addEventListener('resize', resizer)
    return () => {
      window.removeEventListener('resize', resizer)
      pieChart.current?.dispose(); pieChart.current = null
      ageBarChart.current?.dispose(); ageBarChart.current = null
    }
  }, [])

  // 初始化库龄堆叠柱图
  useEffect(() => {
    if (activeTab === 'age' && ageBarRef.current && !ageBarChart.current) {
      ageBarChart.current = echarts.init(ageBarRef.current, undefined, { renderer: 'canvas' })
    }
    if (ageBarChart.current && ageItems.length > 0) {
      const top15 = ageItems.slice(0, 15)
      ageBarChart.current.setOption({
        backgroundColor: 'transparent',
        tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
        legend: { data: ['0-90天', '91-180天', '181-270天', '271-365天', '>365天'], bottom: 0, textStyle: { fontSize: 11 } },
        grid: { top: 10, left: 50, right: 20, bottom: 60 },
        xAxis: { type: 'value', axisLabel: { fontSize: 10 } },
        yAxis: { type: 'category', data: top15.map(i => i.sku.slice(0, 18)), axisLabel: { fontSize: 10 } },
        series: [
          { name: '0-90天', type: 'bar', stack: 'age', data: top15.map(i => i.qty0to90), itemStyle: { color: '#10b981' } },
          { name: '91-180天', type: 'bar', stack: 'age', data: top15.map(i => i.qty91to180), itemStyle: { color: '#f59e0b' } },
          { name: '181-270天', type: 'bar', stack: 'age', data: top15.map(i => i.qty181to270), itemStyle: { color: '#f97316' } },
          { name: '271-365天', type: 'bar', stack: 'age', data: top15.map(i => i.qty271to365), itemStyle: { color: '#ef4444' } },
          { name: '>365天', type: 'bar', stack: 'age', data: top15.map(i => i.qtyOver365), itemStyle: { color: '#7f1d1d' } },
        ],
      })
    }
  }, [activeTab, ageItems])

  const updatePieChart = useCallback((data: InventoryItem[]) => {
    if (!pieChart.current) return
    const ok       = data.filter(i => i.alertLevel === 'ok').length
    const warning  = data.filter(i => i.alertLevel === 'warning').length
    const critical = data.filter(i => i.alertLevel === 'critical').length

    pieChart.current.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'item', formatter: '{b}: {c} SKU ({d}%)' },
      legend: { bottom: 0, textStyle: { fontSize: 11 } },
      series: [{
        type: 'pie', radius: ['40%', '70%'],
        data: [
          { value: ok, name: '库存充足', itemStyle: { color: '#10b981' } },
          { value: warning, name: '库存不足', itemStyle: { color: '#f59e0b' } },
          { value: critical, name: '断货 / 紧急', itemStyle: { color: '#ef4444' } },
        ],
        label: { show: false },
        emphasis: { itemStyle: { shadowBlur: 10 } },
      }],
    })
  }, [])

  // 饼图初始化：使用 ref callback，避免条件渲染导致的时序 bug
  const pieRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (node && !pieChart.current) {
      pieChart.current = echarts.init(node, undefined, { renderer: 'canvas' })
      // 如果此时 items 已有数据，立即渲染饼图
      if (items.length > 0) { updatePieChart(items) }
    }
  }, [items, updatePieChart])

  const fetchData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const list: InventoryItem[] = await GetInventoryItems(activeAccountId, activeMarketplaceId) || []
      setItems(list)
      updatePieChart(list)
    } catch { setItems([]) } finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId, updatePieChart])

  const fetchAgeData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setAgeLoading(true)
    try {
      const result = await GetInventoryAge(activeAccountId, activeMarketplaceId)
      setAgeItems(result?.items || [])
      setAgeSummary(result?.summary || null)
    } catch { setAgeItems([]) } finally { setAgeLoading(false) }
  }, [activeAccountId, activeMarketplaceId])

  const fetchReturnData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setReturnLoading(true)
    try {
      const list: ReturnRateItem[] = await GetReturnRateByASIN(activeAccountId, activeMarketplaceId, 30) || []
      setReturnItems(list)
    } catch { setReturnItems([]) } finally { setReturnLoading(false) }
  }, [activeAccountId, activeMarketplaceId])

  const fetchReplenishData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setReplenishLoading(true)
    try {
      const list = await GetReplenishmentAdvice(activeAccountId, activeMarketplaceId, leadTimeDays) || []
      setReplenishItems(list)
    } catch { setReplenishItems([]) } finally { setReplenishLoading(false) }
  }, [activeAccountId, activeMarketplaceId, leadTimeDays])

  // 加载旺季系数配置
  const fetchSeasonCfg = useCallback(async () => {
    if (!activeAccountId) return
    try {
      const cfg = await GetSeasonConfig(activeAccountId)
      if (cfg) setSeasonCfg(cfg)
    } catch {}
  }, [activeAccountId])

  useEffect(() => { fetchData() }, [fetchData])

  useEffect(() => {
    if (activeTab === 'age') fetchAgeData()
    if (activeTab === 'returns') fetchReturnData()
    if (activeTab === 'replenish') { fetchReplenishData(); fetchSeasonCfg() }
  }, [activeTab, fetchAgeData, fetchReturnData, fetchReplenishData, fetchSeasonCfg])

  const handleSync = async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setSyncing(true)
    setSyncState({ status: 'syncing', message: '正在同步 FBA 库存...' })
    try {
      const result = await TriggerSync(activeAccountId, activeMarketplaceId, 'inventory')
      if (result?.success) {
        message.success(result.message)
        setSyncState({ status: 'success', message: result.message, lastSyncTime: new Date().toLocaleString('zh-CN') })
        fetchData()
      } else {
        message.warning(result?.message ?? '同步失败')
        setSyncState({ status: 'error', message: result?.message ?? '同步失败' })
      }
    } finally { setSyncing(false) }
  }

  const handleExport = async () => {
    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
    setExporting(true)
    try {
      const r: any = await ExportDataCSV(activeAccountId, activeMarketplaceId, 'inventory', '', '')
      if (r?.success) {
        message.success(`${r.message} (${r.fileSize})`)
        OpenFileInExplorer(r.path)
      } else {
        message.warning(r?.message ?? '导出失败')
      }
    } catch { message.error('导出失败') } finally { setExporting(false) }
  }

  const filteredItems = items.filter(i => filter === 'all' ? true : i.alertLevel === filter)
  const criticalCount = items.filter(i => i.alertLevel === 'critical').length
  const warningCount  = items.filter(i => i.alertLevel === 'warning').length
  const totalFulfillable = items.reduce((s, i) => s + i.fulfillableQty, 0)

  const alertIcon = (level: string) => {
    if (level === 'critical') return <ExclamationCircleOutlined style={{ color: '#ef4444' }} />
    if (level === 'warning')  return <WarningOutlined style={{ color: '#f59e0b' }} />
    return <CheckCircleOutlined style={{ color: '#10b981' }} />
  }

  const columns: ColumnsType<InventoryItem> = [
    { title: '状态', dataIndex: 'alertLevel', width: 56,
      render: v => <Tooltip title={({ ok: '库存充足（≥14天）', warning: '库存不足（7-14天）', critical: '紧急（<7天或断货）' } as Record<string, string>)[v]}>{alertIcon(v)}</Tooltip> },
    { title: 'SKU', dataIndex: 'sellerSku', width: 160,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: 'ASIN', dataIndex: 'asin', width: 110,
      render: v => <a href={`https://${domain}/dp/${v}`} target="_blank" rel="noopener noreferrer"
        style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</a> },
    { title: '商品名称', dataIndex: 'title', ellipsis: true },
    { title: '可售库存', dataIndex: 'fulfillableQty', sorter: (a, b) => a.fulfillableQty - b.fulfillableQty,
      render: (v, r) => (
        <span style={{ color: r.alertLevel === 'critical' ? '#ef4444' : r.alertLevel === 'warning' ? '#f59e0b' : '#10b981', fontWeight: 600 }}>
          {v.toLocaleString()}
        </span>
      ) },
    { title: '在途库存', dataIndex: 'inboundQty',
      render: v => <span className="num">{v.toLocaleString()}</span> },
    { title: '不可售', dataIndex: 'unsellableQty',
      render: v => v > 0 ? <span style={{ color: '#ef4444' }}>{v}</span> : <span className="num">0</span> },
    { title: '日均销量', dataIndex: 'dailySalesAvg',
      render: v => <span className="num">{v > 0 ? v.toFixed(1) : '-'}</span> },
    { title: '预计库存天数', dataIndex: 'estDaysLeft', sorter: (a, b) => a.estDaysLeft - b.estDaysLeft,
      render: (v, r) => {
        if (v >= 999) return <span className="num">∞</span>
        const color = v < 7 ? '#ef4444' : v < 14 ? '#f59e0b' : '#10b981'
        return (
          <Tooltip title={`约 ${v.toFixed(0)} 天`}>
            <div style={{ minWidth: 90 }}>
              <Progress percent={(Math.min(v, 60) / 60) * 100} size="small"
                strokeColor={color} showInfo={false} />
              <span style={{ fontSize: 11, color }}>{v.toFixed(0)} 天</span>
            </div>
          </Tooltip>
        )
      } },
    { title: '快照日期', dataIndex: 'snapshotDate', width: 105,
      render: v => <span style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{v}</span> },
  ]

  // 库龄表格列
  const ageColumns: ColumnsType<InventoryAgeItem> = [
    { title: '风险', dataIndex: 'riskLevel', width: 64,
      render: v => {
        if (v === 'critical') return <Tag color="red">高危</Tag>
        if (v === 'warning') return <Tag color="orange">注意</Tag>
        return <Tag color="green">正常</Tag>
      }},
    { title: 'SKU', dataIndex: 'sku', width: 160,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: 'ASIN', dataIndex: 'asin', width: 110,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: '商品名称', dataIndex: 'productName', ellipsis: true },
    { title: '0-90天', dataIndex: 'qty0to90', render: v => <span className="num" style={{ color: '#10b981' }}>{v}</span> },
    { title: '91-180天', dataIndex: 'qty91to180', render: v => <span className="num" style={{ color: '#f59e0b' }}>{v}</span> },
    { title: '181-270天', dataIndex: 'qty181to270', render: v => v > 0 ? <span style={{ color: '#f97316', fontWeight: 600 }}>{v}</span> : <span className="num">0</span> },
    { title: '271-365天', dataIndex: 'qty271to365', render: v => v > 0 ? <span style={{ color: '#ef4444', fontWeight: 600 }}>{v}</span> : <span className="num">0</span> },
    { title: '>365天(高危)', dataIndex: 'qtyOver365',
      render: v => v > 0 ? <Tag color="red">{v}</Tag> : <span className="num">0</span> },
    { title: '滞龄占比', dataIndex: 'agingRatio',
      sorter: (a, b) => a.agingRatio - b.agingRatio,
      render: v => <Progress percent={Math.min(v, 100)} size="small"
        strokeColor={v > 60 ? '#ef4444' : v > 30 ? '#f59e0b' : '#10b981'}
        format={p => `${p?.toFixed(0)}%`} style={{ minWidth: 100 }} /> },
    { title: '预计LTSF', dataIndex: 'estLtsf', sorter: (a, b) => a.estLtsf - b.estLtsf,
      render: v => v > 0 ? <span style={{ color: '#ef4444', fontWeight: 600 }}>{currency} {v.toFixed(2)}</span> : <span className="num">{currency} 0</span> },
  ]

  // 退货率表格列
  const returnColumns: ColumnsType<ReturnRateItem> = [
    { title: 'ASIN', dataIndex: 'asin', width: 110,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: 'SKU', dataIndex: 'sku', width: 140,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: '商品', dataIndex: 'title', ellipsis: true },
    { title: '退货件数', dataIndex: 'totalReturns',
      sorter: (a, b) => a.totalReturns - b.totalReturns,
      render: v => <span style={{ color: '#ef4444', fontWeight: 600 }}>{v}</span> },
    { title: '已售件数', dataIndex: 'totalSold', render: v => <span className="num">{v}</span> },
    { title: '退货率', dataIndex: 'returnRate',
      sorter: (a, b) => a.returnRate - b.returnRate,
      render: v => {
        const color = v > 20 ? '#ef4444' : v > 10 ? '#f59e0b' : '#10b981'
        return (
          <span style={{ color, fontWeight: 700, fontFamily: 'var(--font-number)' }}>
            {v.toFixed(1)}%
          </span>
        )
      }},
    { title: '主要退货原因', dataIndex: 'topReason',
      render: v => v ? <Tag color="default">{v}</Tag> : <span style={{ color: 'var(--color-text-muted)' }}>-</span> },
  ]

  return (
    <div>
      {/* ── 页头 ── */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="page-title">📦 库存管理</div>
          <div className="page-subtitle">{mp?.name ?? '请选择站点'} · FBA 库存与售后</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {activeTab === 'inventory' && <>
            <Select value={filter} onChange={setFilter} style={{ width: 135 }}>
              <Select.Option value="all">全部 ({items.length})</Select.Option>
              <Select.Option value="critical">🔴 紧急 ({criticalCount})</Select.Option>
              <Select.Option value="warning">🟡 预警 ({warningCount})</Select.Option>
            </Select>
            <Tooltip title="导出库存 CSV"><Button icon={<DownloadOutlined />} loading={exporting} onClick={handleExport} /></Tooltip>
            <Button icon={<ReloadOutlined />} loading={syncing} onClick={handleSync}>同步库存</Button>
          </>}
        </div>
      </div>

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'inventory',
            label: '库存快照',
            children: (
              <>
                {/* 统计卡片 + 饼图 */}
                <div style={{ display: 'flex', gap: 16, marginBottom: 16 }}>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, flex: 1 }}>
                    {[
                      { label: '总 SKU 数', value: items.length, color: 'var(--color-primary)' },
                      { label: '总可售库存', value: totalFulfillable.toLocaleString(), color: '#1a2744', suffix: '件' },
                      { label: '🔴 断货/紧急', value: criticalCount, color: '#ef4444' },
                      { label: '🟡 库存不足', value: warningCount, color: '#f59e0b' },
                    ].map(card => (
                      <div key={card.label} className="voyage-card" style={{ padding: '14px 18px' }}>
                        <div style={{ fontSize: 12, color: 'var(--color-text-secondary)', marginBottom: 4 }}>{card.label}</div>
                        <div style={{ fontSize: 26, fontWeight: 700, color: card.color, fontFamily: 'var(--font-number)' }}>
                          {card.value}{card.suffix ? <span style={{ fontSize: 13 }}> {card.suffix}</span> : ''}
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="voyage-card" style={{ padding: '14px 18px', width: 260, flexShrink: 0 }}>
                    <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 4, color: 'var(--color-text-secondary)' }}>库存健康分布</div>
                    {items.length > 0
                      ? <div ref={pieRefCallback} style={{ height: 170 }} />
                      : <div style={{ height: 170, display: 'flex', alignItems: 'center', justifyContent: 'center',
                          color: 'var(--color-text-muted)', fontSize: 13 }}>暂无数据</div>}
                  </div>
                </div>
                <div className="voyage-card">
                  <Table columns={columns} dataSource={filteredItems} rowKey="sellerSku"
                    loading={loading} size="small"
                    pagination={{ pageSize: 30, showSizeChanger: true, showTotal: t => `共 ${t} 条` }}
                    scroll={{ x: 1050 }}
                    rowClassName={(r: InventoryItem) => r.alertLevel === 'critical' ? 'row-critical' : r.alertLevel === 'warning' ? 'row-warning' : ''} />
                </div>
              </>
            )
          },
          {
            key: 'age',
            label: (
              <span>
                库龄分析
                {ageSummary && ageSummary.qtyOver365 > 0 && (
                  <Badge count={ageSummary.qtyOver365} size="small" style={{ marginLeft: 6, background: '#ef4444' }} />
                )}
              </span>
            ),
            children: (
              <>
                {ageSummary && (
                  <Alert
                    message={
                      <span>
                        ⏱️ 数据更新时间：<strong>{ageSummary.snapshotDate}</strong>　
                        <LatencyBadge note={ageSummary.dataLatencyNote} />
                      </span>
                    }
                    description={ageSummary.qtyOver365 > 0
                      ? `检测到 ${ageSummary.qtyOver365} 件库存超过 365 天，预计本月长期仓储费合计 ${currency} ${ageSummary.estTotalLtsf.toFixed(2)}，建议尽快清货处置。`
                      : '暂未发现超过 365 天的库存。'}
                    type={ageSummary.qtyOver365 > 0 ? 'error' : 'success'}
                    showIcon style={{ marginBottom: 16 }}
                  />
                )}

                {/* 汇总卡片 */}
                {ageSummary && (
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 16 }}>
                    {[
                      { label: 'SKU 品类', value: ageSummary.totalSkus },
                      { label: '>180天滞龄库存', value: ageSummary.qtyAtRisk, color: '#f59e0b' },
                      { label: '>365天高危库存', value: ageSummary.qtyOver365, color: '#ef4444' },
                      { label: '预估 LTSF 合计', value: `${currency} ${ageSummary.estTotalLtsf.toFixed(2)}`, color: '#ef4444' },
                    ].map(c => (
                      <div key={c.label} className="voyage-card" style={{ padding: '12px 16px' }}>
                        <div style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{c.label}</div>
                        <div style={{ fontSize: 22, fontWeight: 700, color: c.color || 'var(--color-primary)', fontFamily: 'var(--font-number)' }}>{c.value}</div>
                      </div>
                    ))}
                  </div>
                )}

                {/* 库龄堆积柱图 */}
                {ageItems.length > 0 && (
                  <div className="voyage-card" style={{ padding: '14px 18px', marginBottom: 16 }}>
                    <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                      Top 15 SKU 库龄分布（件数堆叠）
                    </div>
                    <div ref={ageBarRef} style={{ height: 340 }} />
                  </div>
                )}

                <div className="voyage-card">
                  <Table columns={ageColumns} dataSource={ageItems} rowKey="sku"
                    loading={ageLoading} size="small"
                    pagination={{ pageSize: 25, showTotal: t => `共 ${t} 条` }}
                    scroll={{ x: 1200 }}
                    rowClassName={(r: InventoryAgeItem) => r.riskLevel === 'critical' ? 'row-critical' : r.riskLevel === 'warning' ? 'row-warning' : ''} />
                </div>
              </>
            )
          },
          {
            key: 'returns',
            label: '退货分析',
            children: (
              <>
                <Alert
                  message={
                    <span>
                      ⚠️ 退货数据延迟约 T+1（FBA 退货次日可查）
                      <LatencyBadge note="退货原因数据通常在实际退货后 1-2 个工作日内出现，建议搭配 FBA 同步使用" />
                    </span>
                  }
                  type="info" showIcon style={{ marginBottom: 16 }}
                />

                {returnItems.length > 0 && (
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 16 }}>
                    {[
                      { label: '统计 ASIN 数', value: returnItems.length },
                      { label: '退货率 >10% ASIN', value: returnItems.filter(r => r.returnRate > 10).length, color: '#f59e0b' },
                      { label: '退货率 >20% ASIN', value: returnItems.filter(r => r.returnRate > 20).length, color: '#ef4444' },
                    ].map(c => (
                      <div key={c.label} className="voyage-card" style={{ padding: '12px 16px' }}>
                        <div style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{c.label}</div>
                        <div style={{ fontSize: 22, fontWeight: 700, color: c.color || 'var(--color-primary)', fontFamily: 'var(--font-number)' }}>{c.value}</div>
                      </div>
                    ))}
                  </div>
                )}

                <div className="voyage-card">
                  <Table columns={returnColumns} dataSource={returnItems} rowKey="asin"
                    loading={returnLoading} size="small"
                    pagination={{ pageSize: 25, showTotal: t => `共 ${t} 条` }}
                    scroll={{ x: 900 }}
                    rowClassName={(r: ReturnRateItem) => r.returnRate > 20 ? 'row-critical' : r.returnRate > 10 ? 'row-warning' : ''} />
                </div>

                {/* 退货原因分布饼图 */}
                <ReasonPieChart accountId={activeAccountId} marketplaceId={activeMarketplaceId} />
              </>
            )
          },
          {
            key: 'replenish',
            label: (
              <span>
                🚚 补货建议
                {replenishItems.filter((r: any) => r.urgency === 'critical').length > 0 && (
                  <Badge count={replenishItems.filter((r: any) => r.urgency === 'critical').length}
                    size="small" style={{ marginLeft: 6, background: '#ef4444' }} />
                )}
              </span>
            ),
            children: (
              <>
                <Alert
                  message={
                    <span>
                      💡 补货建议基于近 30 天日均销量 + FBA 库存快照（T+1）自动计算。
                      <strong> 默认头程周期：</strong>
                      <InputNumber
                        min={1} max={180} value={leadTimeDays}
                        onChange={(v) => { if (v) setLeadTimeDays(v) }}
                        size="small" style={{ width: 60, margin: '0 4px' }}
                        suffix="天"
                      />
                      <Button size="small" type="link" onClick={fetchReplenishData}>
                        重新计算
                      </Button>
                    </span>
                  }
                  type="info" showIcon style={{ marginBottom: 12 }}
                />

                {/* ── 旺季系数配置面板 ── */}
                <div className="voyage-card" style={{ marginBottom: 16, padding: '14px 18px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
                    <ThunderboltOutlined style={{ color: '#f59e0b', fontSize: 16 }} />
                    <span style={{ fontWeight: 600, fontSize: 14 }}>旺季系数配置</span>
                    <Tag color="orange" style={{ fontSize: 11, marginLeft: 4 }}>季节性库存预警</Tag>
                    <Tooltip title="旺季系数会乘以近 30 天日均销量，提升建议补货量和预警阈值。Q4（10-12月）建议设为 1.3~1.8 应对 Black Friday 和圣诞旺季。">
                      <InfoCircleOutlined style={{ color: '#9ca3af', cursor: 'help' }} />
                    </Tooltip>
                    <span style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--color-text-muted)' }}>
                      自动按季度应用：<Switch
                        size="small"
                        checked={seasonCfg.autoApply}
                        onChange={(v) => setSeasonCfg({ ...seasonCfg, autoApply: v })}
                        style={{ marginLeft: 4 }}
                      />
                    </span>
                  </div>

                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 16 }}>
                    {([
                      { key: 'q1Factor', label: 'Q1（1-3月）', color: '#3b82f6', note: '年后淡季' },
                      { key: 'q2Factor', label: 'Q2（4-6月）', color: '#10b981', note: '平稳期' },
                      { key: 'q3Factor', label: 'Q3（7-9月）', color: '#f59e0b', note: '小旺季/Prime Day' },
                      { key: 'q4Factor', label: 'Q4（10-12月）', color: '#ef4444', note: '旺季' },
                      { key: 'primeDayFactor', label: 'Prime Day（7月）', color: '#8b5cf6', note: '特殊促销' },
                    ] as { key: string; label: string; color: string; note: string }[]).map(({ key, label, color, note }) => (
                      <div key={key}>
                        <div style={{ fontSize: 12, fontWeight: 600, color, marginBottom: 4 }}>{label}</div>
                        <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 6 }}>{note}</div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <Slider
                            min={0.5} max={3.0} step={0.1}
                            value={seasonCfg[key] as number}
                            onChange={(v) => setSeasonCfg({ ...seasonCfg, [key]: v })}
                            style={{ flex: 1 }}
                            tooltip={{ formatter: (v) => `×${v}` }}
                            trackStyle={{ background: color }}
                            handleStyle={{ borderColor: color }}
                          />
                          <InputNumber
                            min={0.5} max={3.0} step={0.1} precision={1}
                            value={seasonCfg[key] as number}
                            onChange={(v) => { if (v) setSeasonCfg({ ...seasonCfg, [key]: v }) }}
                            size="small" style={{ width: 60 }}
                          />
                        </div>
                        <div style={{ textAlign: 'center', fontSize: 11,
                          color: (seasonCfg[key] as number) > 1.2 ? color : '#9ca3af',
                          fontWeight: (seasonCfg[key] as number) > 1.2 ? 700 : 400,
                        }}>
                          {(seasonCfg[key] as number) === 1.0 ? '正常' : `×${(seasonCfg[key] as number).toFixed(1)}`}
                        </div>
                      </div>
                    ))}
                  </div>

                  <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'flex-end' }}>
                    <Popover
                      content={
                        <div style={{ fontSize: 12 }}>
                          <div style={{ marginBottom: 8, color: '#ef4444', fontWeight: 600 }}>⚠️ 此操作会覆盖所有 SKU 的旺季系数</div>
                          <div>请先保存季度配置，再选择要应用的季度系数批量写入。</div>
                          <div style={{ marginTop: 8, display: 'flex', gap: 8 }}>
                            {['Q1', 'Q2', 'Q3', 'Q4'].map((q, i) => (
                              <Button key={q} size="small" danger
                                loading={applyingFactor}
                                onClick={async () => {
                                  if (!activeAccountId || !activeMarketplaceId) return
                                  setApplyingFactor(true)
                                  const factor = [seasonCfg.q1Factor, seasonCfg.q2Factor, seasonCfg.q3Factor, seasonCfg.q4Factor][i]
                                  try {
                                    await ApplyGlobalSeasonFactor(activeAccountId, activeMarketplaceId, factor)
                                    message.success(`已将 ${q} 系数(×${factor}) 批量应用到所有 SKU`)
                                    fetchReplenishData()
                                  } catch { message.error('应用失败') } finally { setApplyingFactor(false) }
                                }}
                              >{q} ×{[seasonCfg.q1Factor, seasonCfg.q2Factor, seasonCfg.q3Factor, seasonCfg.q4Factor][i]}</Button>
                            ))}
                          </div>
                        </div>
                      }
                      title="批量应用旺季系数"
                      trigger="click"
                    >
                      <Button size="small" icon={<SettingOutlined />} danger>批量应用系数</Button>
                    </Popover>
                    <Button
                      size="small" type="primary"
                      loading={savingSeasonCfg}
                      onClick={async () => {
                        if (!activeAccountId) return
                        setSavingSeasonCfg(true)
                        try {
                          await SaveSeasonConfig({ ...seasonCfg, accountId: activeAccountId })
                          message.success('旺季系数配置已保存')
                          fetchReplenishData()
                        } catch { message.error('保存失败') } finally { setSavingSeasonCfg(false) }
                      }}
                    >
                      保存配置
                    </Button>
                  </div>
                </div>

                {replenishItems.length > 0 && (
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 16 }}>
                    {[
                      { label: 'SKU 总计', value: replenishItems.length, color: 'var(--color-primary)' },
                      { label: '🔴 紧急补货', value: replenishItems.filter((r: any) => r.urgency === 'critical').length, color: '#ef4444' },
                      { label: '🟡 建议补货', value: replenishItems.filter((r: any) => r.urgency === 'warning').length, color: '#f59e0b' },
                      { label: '预估采购总成本', value: `¥${replenishItems.reduce((s: number, r: any) => s + (r.estCost || 0), 0).toLocaleString(undefined, { maximumFractionDigits: 0 })}`, color: '#7c3aed' },
                    ].map(c => (
                      <div key={c.label} className="voyage-card" style={{ padding: '12px 16px' }}>
                        <div style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{c.label}</div>
                        <div style={{ fontSize: 22, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
                      </div>
                    ))}
                  </div>
                )}

                <div className="voyage-card">
                  <Table
                    columns={[
                      { title: '紧急度', dataIndex: 'urgency', width: 80,
                        filters: [
                          { text: '🔴 紧急', value: 'critical' },
                          { text: '🟡 建议', value: 'warning' },
                          { text: '✅ 充足', value: 'ok' },
                        ],
                        onFilter: (v, r) => r.urgency === v,
                        render: (v: string) => {
                          if (v === 'critical') return <Tag color="red">🔴 紧急</Tag>
                          if (v === 'warning') return <Tag color="orange">🟡 建议</Tag>
                          return <Tag color="green">✅ 充足</Tag>
                        }
                      },
                      { title: 'SKU', dataIndex: 'sku', width: 150,
                        render: (v: string) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
                      { title: 'ASIN', dataIndex: 'asin', width: 110,
                        render: (v: string) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
                      { title: '商品', dataIndex: 'title', ellipsis: true },
                      { title: '可售', dataIndex: 'currentStock', width: 70,
                        render: (v: number) => <span className="num">{v}</span> },
                      { title: '在途', dataIndex: 'inboundQty', width: 70,
                        render: (v: number) => <span className="num">{v || 0}</span> },
                      { title: '日均销量', dataIndex: 'dailyAvgSales', width: 80,
                        sorter: (a: any, b: any) => a.dailyAvgSales - b.dailyAvgSales,
                        render: (v: number) => <span className="num">{v > 0 ? v.toFixed(1) : '-'}</span> },
                      { title: '旺季系数', dataIndex: 'seasonFactor', width: 80,
                        render: (v: number) => {
                          const isActive = v > 1.0
                          return (
                            <Tag
                              style={{ margin: 0, fontWeight: 700, fontFamily: 'var(--font-number)', fontSize: 11,
                                background: isActive ? '#fef3c7' : '#f3f4f6',
                                color: isActive ? '#d97706' : '#6b7280',
                                border: isActive ? '1px solid #fcd34d' : '1px solid #e5e7eb',
                              }}
                            >
                              ×{v ? v.toFixed(1) : '1.0'}
                            </Tag>
                          )
                        }
                      },
                      { title: '有效日均', dataIndex: 'effectiveDailyAvg', width: 90,
                        sorter: (a: any, b: any) => (a.effectiveDailyAvg||0) - (b.effectiveDailyAvg||0),
                        render: (v: number, r: any) => {
                          const isAdjusted = r.seasonFactor > 1.0
                          return (
                            <Tooltip title={isAdjusted ? `原始日均 ${r.dailyAvgSales?.toFixed(1)} × 旺季系数 ${r.seasonFactor?.toFixed(1)}` : '与日均相同'}>
                              <span style={{ fontFamily: 'var(--font-number)', fontSize: 12,
                                color: isAdjusted ? '#d97706' : undefined,
                                fontWeight: isAdjusted ? 700 : undefined,
                              }}>
                                {(v || 0) > 0 ? (v || 0).toFixed(1) : '-'}
                                {isAdjusted && <ThunderboltOutlined style={{ fontSize: 10, marginLeft: 2, color: '#f59e0b' }} />}
                              </span>
                            </Tooltip>
                          )
                        }
                      },
                      { title: '可售天数', dataIndex: 'daysOfStock', width: 100,
                        sorter: (a: any, b: any) => a.daysOfStock - b.daysOfStock,
                        render: (v: number, r: any) => {
                          if (v >= 999) return <span className="num">∞</span>
                          const color = r.urgency === 'critical' ? '#ef4444' : r.urgency === 'warning' ? '#f59e0b' : '#10b981'
                          return (
                            <div style={{ minWidth: 80 }}>
                              <Progress percent={Math.min(v / (leadTimeDays * 2) * 100, 100)} size="small" strokeColor={color} showInfo={false} />
                              <span style={{ fontSize: 11, color, fontWeight: 600 }}>{v.toFixed(0)}天</span>
                            </div>
                          )
                        }
                      },
                      { title: '建议补货量', dataIndex: 'suggestedQty', width: 100,
                        sorter: (a: any, b: any) => a.suggestedQty - b.suggestedQty,
                        render: (v: number, r: any) => v > 0
                          ? <span style={{ color: '#7c3aed', fontWeight: 700, fontFamily: 'var(--font-number)' }}>{v.toLocaleString()}</span>
                          : <span className="num">-</span>
                      },
                      { title: '预估成本', dataIndex: 'estCost', width: 100,
                        render: (v: number) => v > 0
                          ? <span style={{ fontFamily: 'var(--font-number)' }}>¥{v.toLocaleString(undefined, { maximumFractionDigits: 0 })}</span>
                          : <span className="num">-</span>
                      },
                      { title: '头程(天)', dataIndex: 'leadTimeDays', width: 80,
                        render: (v: number) => <span className="num">{v}</span> },
                    ] as ColumnsType<any>}
                    dataSource={replenishItems}
                    rowKey="sku"
                    loading={replenishLoading}
                    size="small"
                    pagination={{ pageSize: 30, showTotal: t => `共 ${t} 条` }}
                    scroll={{ x: 1200 }}
                    rowClassName={(r: any) => r.urgency === 'critical' ? 'row-critical' : r.urgency === 'warning' ? 'row-warning' : ''}
                  />
                </div>
              </>
            )
          }
        ]}
      />
    </div>
  )
}

// ── 退货原因分布饼图组件 ──
function ReasonPieChart({ accountId, marketplaceId }: { accountId: number | null; marketplaceId: string | null }) {
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInst = useRef<echarts.ECharts | null>(null)
  const [data, setData] = useState<{reason:string;reasonDesc:string;count:number;percentage:number}[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!accountId || !marketplaceId) return
    setLoading(true)
    GetReturnReasonDistribution(accountId, marketplaceId, 30)
      .then((d: any) => {
        const items = d ?? []
        setData(items)
        // 渲染饼图
        setTimeout(() => {
          if (chartRef.current && !chartInst.current) {
            chartInst.current = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
          }
          if (chartInst.current && items.length > 0) {
            const colors = ['#ef4444','#f59e0b','#3b82f6','#10b981','#8b5cf6','#ec4899','#06b6d4','#f97316','#6366f1','#14b8a6']
            chartInst.current.setOption({
              tooltip: { trigger: 'item', formatter: '{b}: {c} 件 ({d}%)' },
              legend: { orient: 'vertical', right: 10, top: 'center', textStyle: { fontSize: 11 } },
              series: [{
                type: 'pie', radius: ['42%', '70%'], center: ['35%', '50%'],
                avoidLabelOverlap: true,
                itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
                label: { show: false },
                emphasis: { label: { show: true, fontWeight: 'bold', fontSize: 13 } },
                data: items.map((d: any, i: number) => ({
                  value: d.count, name: d.reasonDesc || d.reason,
                  itemStyle: { color: colors[i % colors.length] },
                })),
              }],
            })
          } else if (chartInst.current) {
            chartInst.current.setOption({
              graphic: [{ type: 'text', left: 'center', top: 'center',
                style: { text: '暂无退货原因数据', fontSize: 12, fill: '#999' } }],
            })
          }
        }, 120)
      })
      .catch(() => setData([]))
      .finally(() => setLoading(false))

    return () => { chartInst.current?.dispose(); chartInst.current = null }
  }, [accountId, marketplaceId])

  return (
    <div className="voyage-card" style={{ marginTop: 16, padding: 16 }}>
      <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 12 }}>📊 退货原因分布（近 30 天）</div>
      <Spin spinning={loading}>
        <div style={{ display: 'flex', gap: 16, alignItems: 'stretch' }}>
          <div ref={chartRef} style={{ flex: '0 0 50%', height: 260 }} />
          <div style={{ flex: 1, overflow: 'auto' }}>
            <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
              <thead><tr style={{ borderBottom: '1px solid #e5e7eb' }}>
                <th style={{ textAlign: 'left', padding: '6px 8px' }}>原因</th>
                <th style={{ textAlign: 'right', padding: '6px 8px' }}>数量</th>
                <th style={{ textAlign: 'right', padding: '6px 8px' }}>占比</th>
              </tr></thead>
              <tbody>
                {data.map(d => (
                  <tr key={d.reason} style={{ borderBottom: '1px solid #f3f4f6' }}>
                    <td style={{ padding: '6px 8px' }}>{d.reasonDesc || d.reason}</td>
                    <td style={{ padding: '6px 8px', textAlign: 'right', fontWeight: 600, fontFamily: 'var(--font-number)' }}>{d.count}</td>
                    <td style={{ padding: '6px 8px', textAlign: 'right', fontFamily: 'var(--font-number)' }}>{d.percentage.toFixed(1)}%</td>
                  </tr>
                ))}
                {data.length === 0 && (
                  <tr><td colSpan={3} style={{ padding: 20, textAlign: 'center', color: '#9ca3af' }}>暂无退货原因数据</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </Spin>
    </div>
  )
}
