import { useState, useEffect, useCallback, useRef } from 'react'
import { Table, Input, Button, Modal, Form, InputNumber, message, Tag, Tooltip, Drawer, Descriptions, Tabs, Empty, Select } from 'antd'
import { SearchOutlined, DollarOutlined, BarChartOutlined, ReloadOutlined, DownloadOutlined } from '@ant-design/icons'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetSalesByAsin, SaveProductCost, GetAsinDailyTrend, GetAsinFeeInfo, ExportDataCSV, GetPriceHistory } from '../../wailsjs/go/main/App'
import { getAmazonDomain } from '../utils/marketplaceUtils'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent, CanvasRenderer])

interface ProductRow {
  asin: string; title: string; sales: number; units: number
  pageViews: number; sessions: number; conversionRate: number; buyBoxPct: number
}

export default function Products() {
  const { activeAccountId, activeMarketplaceId, marketplaces } = useAppStore()
  const mp = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)
  const currency = mp?.currencyCode ?? 'USD'

  // 根据当前站点生成 Amazon 链接域名
  const domain = getAmazonDomain(mp?.countryCode)

  const [products, setProducts] = useState<ProductRow[]>([])
  const [loading, setLoading] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [search, setSearch] = useState('')
  const [costModal, setCostModal] = useState<{ open: boolean; asin: string }>({ open: false, asin: '' })
  const [costForm] = Form.useForm()

  // 多 ASIN 对比
  const [selectedAsins, setSelectedAsins] = useState<string[]>([])
  const [compareOpen, setCompareOpen] = useState(false)
  const [compareMetric, setCompareMetric] = useState<'sales'|'units'|'sessions'>('sales')
  const [compareData, setCompareData] = useState<Record<string, any[]>>({})
  const [compareLoading, setCompareLoading] = useState(false)
  const compareChartRef = useRef<HTMLDivElement>(null)
  const compareChartInst = useRef<echarts.ECharts | null>(null)

  // 详情抽屉
  const [drawer, setDrawer] = useState<{ open: boolean; product: ProductRow | null }>({ open: false, product: null })
  const [asinFeeInfo, setAsinFeeInfo] = useState<any>(null)
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInst = useRef<echarts.ECharts | null>(null)
  const priceChartRef = useRef<HTMLDivElement>(null)
  const priceChartInst = useRef<echarts.ECharts | null>(null)

  const dateEnd   = dayjs().subtract(2, 'day').format('YYYY-MM-DD')
  const dateStart = dayjs().subtract(92, 'day').format('YYYY-MM-DD')

  const fetchData = useCallback(async () => {
    if (!activeAccountId || !activeMarketplaceId) return
    setLoading(true)
    try {
      const data = await GetSalesByAsin(activeAccountId, activeMarketplaceId, dateStart, dateEnd, 200)
      setProducts((data ?? []) as unknown as ProductRow[])
    } catch { setProducts([]) } finally { setLoading(false) }
  }, [activeAccountId, activeMarketplaceId])

  useEffect(() => { fetchData() }, [fetchData])

  // 初始化抽屉图表
  useEffect(() => {
    if (drawer.open && chartRef.current && !chartInst.current) {
      chartInst.current = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    }
    if (!drawer.open) {
      chartInst.current?.dispose()
      chartInst.current = null
      priceChartInst.current?.dispose()
      priceChartInst.current = null
    }
  }, [drawer.open])

  // 打开 ASIN 详情抽屉
  const openDrawer = async (product: ProductRow) => {
    setDrawer({ open: true, product })
  }

  // 初始化图表并拉取趋势数据（真实 Data Kiosk 数据，T+2 延迟）
  useEffect(() => {
    if (!drawer.open || !drawer.product || !activeAccountId || !activeMarketplaceId) return
    const init = async () => {
      await new Promise(r => setTimeout(r, 100))
      if (chartRef.current && !chartInst.current) {
        chartInst.current = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
      }

      try {
        // 从 Data Kiosk sales_traffic_by_asin 获取真实趋势（T+2）
        const trendEnd = dayjs().subtract(2, 'day').format('YYYY-MM-DD')
        const trendStart = dayjs().subtract(30, 'day').format('YYYY-MM-DD')
        const trendData: any[] = await GetAsinDailyTrend(
          activeAccountId, activeMarketplaceId,
          drawer.product!.asin, trendStart, trendEnd
        ) || []

        // 加载实际费率
        const feeInfo = await GetAsinFeeInfo(
          activeAccountId, activeMarketplaceId,
          drawer.product!.asin, trendStart, trendEnd
        )
        setAsinFeeInfo(feeInfo)

        if (chartInst.current && trendData.length > 0) {
          chartInst.current.setOption({
            tooltip: { trigger: 'axis' },
            legend: { data: ['销售额', '销量'], bottom: 0, textStyle: { fontSize: 11 } },
            grid: { top: 10, right: 20, bottom: 40, left: 55 },
            xAxis: { type: 'category', data: trendData.map((d: any) => d.date),
              axisLabel: { fontSize: 10, rotate: 30 } },
            yAxis: [
              { type: 'value', name: '销售额', axisLabel: { fontSize: 10 } },
              { type: 'value', name: '销量', axisLabel: { fontSize: 10 }, position: 'right' },
            ],
            series: [
              { name: '销售额', type: 'line', data: trendData.map((d: any) => d.sales.toFixed(2)),
                smooth: true, itemStyle: { color: '#c9a84c' }, areaStyle: { opacity: 0.1 } },
              { name: '销量', type: 'bar', yAxisIndex: 1, data: trendData.map((d: any) => d.units),
                itemStyle: { color: '#1a2744', opacity: 0.7 } },
            ],
          })
        } else if (chartInst.current) {
          chartInst.current.setOption({
            tooltip: { trigger: 'axis' },
            grid: { top: 10, right: 20, bottom: 30, left: 50 },
            xAxis: { type: 'category', data: [] },
            yAxis: { type: 'value' },
            series: [{ type: 'line', data: [], name: '销售额' }],
            graphic: [{ type: 'text', left: 'center', top: 'center',
              style: { text: '暂无 ASIN 历史数据\n（请先同步 Data Kiosk 数据）', fontSize: 12, fill: '#999' } }],
          })
        }
      } catch {
        if (chartInst.current) {
          chartInst.current.setOption({
            graphic: [{ type: 'text', left: 'center', top: 'center',
              style: { text: '趋势数据加载失败', fontSize: 12, fill: '#ef4444' } }],
          })
        }
      }

      // ── 竞品价格历史图表 ──
      try {
        await new Promise(r => setTimeout(r, 150))
        if (priceChartRef.current && !priceChartInst.current) {
          priceChartInst.current = echarts.init(priceChartRef.current, undefined, { renderer: 'canvas' })
        }
        const priceData: any[] = await GetPriceHistory(
          activeAccountId, activeMarketplaceId, drawer.product!.asin, 90
        ) || []

        if (priceChartInst.current && priceData.length > 0) {
          priceChartInst.current.setOption({
            tooltip: {
              trigger: 'axis',
              formatter: (params: any[]) => {
                const d = params[0]?.axisValue ?? ''
                let html = `<b>${d}</b>`
                params.forEach((p: any) => {
                  html += `<br/>${p.marker}${p.seriesName}: ${typeof p.value === 'number' ? (p.seriesIndex < 2 ? `$ ${p.value.toFixed(2)}` : p.value) : p.value}`
                })
                return html
              },
            },
            legend: { data: ['Buy Box', '落地价', '卖家数'], bottom: 0, textStyle: { fontSize: 11 } },
            grid: { top: 14, right: 50, bottom: 40, left: 55 },
            xAxis: { type: 'category', data: priceData.map((d: any) => d.date),
              axisLabel: { fontSize: 10, rotate: 30 } },
            yAxis: [
              { type: 'value', name: '价格 ($)', axisLabel: { fontSize: 10 }, splitLine: { lineStyle: { color: '#f0f0f0' } } },
              { type: 'value', name: '卖家数', axisLabel: { fontSize: 10 }, position: 'right', splitLine: { show: false } },
            ],
            series: [
              { name: 'Buy Box', type: 'line', data: priceData.map((d: any) => d.buyBoxPrice),
                smooth: true, itemStyle: { color: '#f59e0b' }, lineStyle: { width: 2.5 },
                markPoint: { data: [{ type: 'max', name: '最高' }, { type: 'min', name: '最低' }],
                  symbolSize: 36, label: { fontSize: 9 } } },
              { name: '落地价', type: 'line', data: priceData.map((d: any) => d.landedPrice),
                smooth: true, itemStyle: { color: '#8b5cf6' }, lineStyle: { width: 1.5, type: 'dashed' } },
              { name: '卖家数', type: 'bar', yAxisIndex: 1, data: priceData.map((d: any) => d.numberOfOffers),
                itemStyle: { color: '#1a2744', opacity: 0.3 }, barWidth: '40%' },
            ],
          })
        } else if (priceChartInst.current) {
          priceChartInst.current.setOption({
            graphic: [{ type: 'text', left: 'center', top: 'center',
              style: { text: '暂无竞品价格数据\n（请先同步竞品价格）', fontSize: 12, fill: '#999' } }],
          })
        }
      } catch {
        // 价格图表加载失败静默处理
      }
    }
    init()
  }, [drawer.open, drawer.product, activeAccountId, activeMarketplaceId])

  // 利润预估（使用来自财务事件的实际费率，非硬编码估算）
  const profitEstimate = (p: ProductRow, cost: number, feeInfo: any) => {
    if (!cost || !p.units) return null
    const totalCost = cost * p.units
    // 优先使用真实费率；如无财务数据则回退到行业参考值（明确标注）
    const feeRate = feeInfo?.actualFeeRate > 0 ? feeInfo.actualFeeRate / 100 : 0.15
    const fees = p.sales * feeRate
    // 广告花费暂时按账户总花费/销售额比例，从 KPI 取（无则 0）
    const net = p.sales - totalCost - fees
    const margin = p.sales > 0 ? (net / p.sales) * 100 : 0
    const isRealFee = feeInfo?.actualFeeRate > 0
    return { net, margin, feeRate: feeRate * 100, isRealFee }
  }

  const handleSaveCost = async (values: any) => {
    try {
      await SaveProductCost(activeAccountId!, costModal.asin, currency, values.cost)
      message.success(`ASIN ${costModal.asin} 成本已保存`)
      setCostModal({ open: false, asin: '' })
      costForm.resetFields()
    } catch (err: any) {
      message.error('保存失败: ' + (err?.message ?? String(err)))
    }
  }

  const handleExport = async () => {
    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
    setExporting(true)
    try {
      const csv = await ExportDataCSV(activeAccountId, activeMarketplaceId, 'sales', dateStart, dateEnd)
      if (!csv) { message.warning('暂无数据可导出'); return }
      const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8' })
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `voyage_products_${dayjs().format('YYYY-MM-DD')}.csv`
      a.click()
      message.success('商品数据导出成功')
    } catch { message.error('导出失败') } finally { setExporting(false) }
  }

  const filtered = products.filter(p =>
    search ? (p.asin.includes(search.toUpperCase()) || (p.title ?? '').toLowerCase().includes(search.toLowerCase())) : true
  )

  // 汇总统计
  const totalSales  = filtered.reduce((s, p) => s + p.sales, 0)
  const totalUnits  = filtered.reduce((s, p) => s + p.units, 0)
  const avgCvr      = filtered.length > 0 ? filtered.reduce((s, p) => s + p.conversionRate, 0) / filtered.length : 0
  const avgBuyBox   = filtered.length > 0 ? filtered.reduce((s, p) => s + p.buyBoxPct, 0) / filtered.length : 0

  const columns: ColumnsType<ProductRow> = [
    { title: 'ASIN', dataIndex: 'asin', width: 120,
      render: (v: string) => <a href={`https://${domain}/dp/${v}`} target="_blank" rel="noopener noreferrer"
        style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</a> },
    { title: '商品名称', dataIndex: 'title', ellipsis: true,
      render: (v: string) => <Tooltip title={v}><span style={{ fontSize: 13 }}>{v || '-'}</span></Tooltip> },
    { title: `销售额 (${currency})`, dataIndex: 'sales', sorter: (a, b) => a.sales - b.sales,
      defaultSortOrder: 'descend',
      render: (v: number) => <span className="amount">{v.toLocaleString('en-US', { minimumFractionDigits: 2 })}</span> },
    { title: '销量', dataIndex: 'units', sorter: (a, b) => a.units - b.units,
      render: (v: number) => <span className="num">{v.toLocaleString()}</span> },
    { title: '页面浏览', dataIndex: 'pageViews', sorter: (a, b) => a.pageViews - b.pageViews,
      render: (v: number) => <span className="num">{v.toLocaleString()}</span> },
    { title: '转化率', dataIndex: 'conversionRate', sorter: (a, b) => a.conversionRate - b.conversionRate,
      render: (v: number) => (
        <span style={{ color: v >= 15 ? '#10b981' : v >= 8 ? '#f59e0b' : '#ef4444', fontWeight: 600 }}>
          {v.toFixed(1)}%
        </span>
      ) },
    { title: 'Buy Box', dataIndex: 'buyBoxPct', sorter: (a, b) => a.buyBoxPct - b.buyBoxPct,
      render: (v: number) => (
        <Tag color={v >= 90 ? 'green' : v >= 70 ? 'orange' : 'red'}>{v.toFixed(0)}%</Tag>
      ) },
    {
      title: '操作', width: 120,
      render: (_: any, r: ProductRow) => (
        <div style={{ display: 'flex', gap: 4 }}>
          <Tooltip title="查看详情">
            <Button size="small" icon={<BarChartOutlined />} onClick={() => openDrawer(r)} />
          </Tooltip>
          <Tooltip title="录入成本">
            <Button size="small" icon={<DollarOutlined />}
              onClick={() => { setCostModal({ open: true, asin: r.asin }); costForm.resetFields() }} />
          </Tooltip>
        </div>
      ),
    },
  ]

  return (
    <div>
      {/* ── 页头 ── */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="page-title">🏷️ 商品管理</div>
          <div className="page-subtitle">近 90 天活跃商品 · 共 {filtered.length} 个 ASIN</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <Input.Search
            placeholder="搜索 ASIN 或商品名称"
            allowClear style={{ width: 260 }}
            onChange={e => setSearch(e.target.value)}
            prefix={<SearchOutlined />}
          />
          <Tooltip title="导出 CSV">
            <Button icon={<DownloadOutlined />} loading={exporting} onClick={handleExport} />
          </Tooltip>
          <Button
            disabled={selectedAsins.length < 2}
            onClick={async () => {
              if (selectedAsins.length < 2) { message.warning('请至少勾选 2 个 ASIN'); return }
              if (selectedAsins.length > 5) { message.warning('最多对比 5 个 ASIN'); return }
              setCompareOpen(true)
              setCompareLoading(true)
              try {
                const trendEnd = dayjs().subtract(2, 'day').format('YYYY-MM-DD')
                const trendStart = dayjs().subtract(30, 'day').format('YYYY-MM-DD')
                const results: Record<string, any[]> = {}
                await Promise.all(selectedAsins.map(async asin => {
                  const d = await GetAsinDailyTrend(activeAccountId!, activeMarketplaceId!, asin, trendStart, trendEnd)
                  results[asin] = (d ?? []) as any[]
                }))
                setCompareData(results)
              } catch { message.error('对比数据加载失败') } finally { setCompareLoading(false) }
            }}
            style={{ background: selectedAsins.length >= 2 ? '#7c3aed' : undefined, color: selectedAsins.length >= 2 ? '#fff' : undefined, borderColor: selectedAsins.length >= 2 ? '#7c3aed' : undefined }}
          >🔍 对比 ({selectedAsins.length})</Button>
          <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchData}>刷新</Button>
        </div>
      </div>

      {/* ── 汇总 KPI ── */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 16 }}>
        {[
          { label: `总销售额 (${currency})`, value: totalSales.toFixed(2), color: '#1a2744' },
          { label: '总销量', value: `${totalUnits.toLocaleString()} 件`, color: '#c9a84c' },
          { label: '平均转化率', value: `${avgCvr.toFixed(1)}%`, color: avgCvr >= 10 ? '#10b981' : '#f59e0b' },
          { label: '平均 Buy Box', value: `${avgBuyBox.toFixed(0)}%`, color: avgBuyBox >= 80 ? '#10b981' : '#ef4444' },
        ].map(c => (
          <div key={c.label} className="voyage-card" style={{ padding: '14px 18px' }}>
            <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 4 }}>{c.label}</div>
            <div style={{ fontSize: 22, fontWeight: 700, color: c.color, fontFamily: 'var(--font-number)' }}>{c.value}</div>
          </div>
        ))}
      </div>

      {/* ── 商品表格 ── */}
      <div className="voyage-card">
        <Table
          columns={columns}
          dataSource={filtered}
          rowKey="asin"
          loading={loading}
          size="small"
          rowSelection={{
            selectedRowKeys: selectedAsins,
            onChange: (keys) => setSelectedAsins((keys as string[]).slice(0, 5)),
            getCheckboxProps: (r: ProductRow) => ({
              disabled: selectedAsins.length >= 5 && !selectedAsins.includes(r.asin),
            }),
          }}
          pagination={{ pageSize: 25, showSizeChanger: true, showTotal: t => `共 ${t} 个商品` }}
          scroll={{ x: 1000 }}
          locale={{ emptyText: <Empty description="同步数据后可查看商品列表" /> }}
        />
      </div>

      {/* ── ASIN 详情抽屉 ── */}
      <Drawer
        title={`📦 ${drawer.product?.asin ?? ''} · ASIN 详情`}
        placement="right"
        width={520}
        open={drawer.open}
        onClose={() => setDrawer({ open: false, product: null })}
      >
        {drawer.product && (
          <div>
            {/* 商品名称 */}
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, color: 'var(--color-text)' }}>
              {drawer.product.title || '（无商品名称）'}
            </div>

            {/* 核心指标 */}
            <Descriptions bordered size="small" column={2}>
              <Descriptions.Item label="销售额">
                <span className="amount">{currency} {drawer.product.sales.toFixed(2)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="销量">
                <span className="num">{drawer.product.units.toLocaleString()} 件</span>
              </Descriptions.Item>
              <Descriptions.Item label="页面浏览">
                <span className="num">{drawer.product.pageViews.toLocaleString()}</span>
              </Descriptions.Item>
              <Descriptions.Item label="访客数">
                <span className="num">{drawer.product.sessions.toLocaleString()}</span>
              </Descriptions.Item>
              <Descriptions.Item label="转化率">
                <span style={{ color: drawer.product.conversionRate >= 10 ? '#10b981' : '#f59e0b', fontWeight: 600 }}>
                  {drawer.product.conversionRate.toFixed(1)}%
                </span>
              </Descriptions.Item>
              <Descriptions.Item label="Buy Box 占比">
                <Tag color={drawer.product.buyBoxPct >= 90 ? 'green' : drawer.product.buyBoxPct >= 70 ? 'orange' : 'red'}>
                  {drawer.product.buyBoxPct.toFixed(0)}%
                </Tag>
              </Descriptions.Item>
            </Descriptions>

            {/* 利润预估区（基于真实财务事件费率）*/}
            <div style={{ marginTop: 20, padding: '16px', background: '#f8fafc', borderRadius: 8, border: '1px solid var(--color-border-light)' }}>
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 4, color: 'var(--color-text-secondary)' }}>
                💡 利润预估
                {asinFeeInfo?.actualFeeRate > 0
                  ? <span style={{ fontSize: 11, color: '#10b981', marginLeft: 8 }}>✓ 基于真实财务事件费率</span>
                  : <span style={{ fontSize: 11, color: '#f59e0b', marginLeft: 8 }}>⚠ 暂用行业参考费率（请同步财务数据）</span>}
              </div>
              {asinFeeInfo?.dataNote && (
                <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 8 }}>{asinFeeInfo.dataNote}</div>
              )}
              <Form layout="inline" size="small"
                onFinish={(v) => {
                  const r = profitEstimate(drawer.product!, v.cost, asinFeeInfo)
                  if (r) {
                    message.info(
                      `实际费率 ${r.feeRate.toFixed(1)}%${r.isRealFee ? '（财务数据）' : '（行业参考）'} ｜` +
                      `预估净利润: ${currency} ${r.net.toFixed(2)}，净利率: ${r.margin.toFixed(1)}%`
                    )
                  }
                }}
              >
                <Form.Item name="cost" label="单件成本">
                  <InputNumber min={0} precision={2} prefix={currency} style={{ width: 130 }} />
                </Form.Item>
                <Form.Item>
                  <Button type="primary" htmlType="submit" size="small" style={{ background: '#1a2744' }}>计算</Button>
                </Form.Item>
              </Form>
              <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginTop: 8 }}>
                * 净利润 = 销售额 - 采购成本 - 实际平台+FBA费用（广告费另计）
              </div>
            </div>

            {/* 竞品价格历史趋势图 */}
            <div style={{ marginTop: 20 }}>
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                📊 竞品价格历史（90 天）
              </div>
              <div ref={priceChartRef} style={{ height: 220, borderRadius: 8, background: '#f9fafb', border: '1px solid #f0f0f0' }} />
            </div>

            {/* 快速链接 */}
            <div style={{ marginTop: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <Button size="small" href={`https://${domain}/dp/${drawer.product.asin}`} target="_blank">
                🛒 Amazon 商品页
              </Button>
              <Button size="small" href={`https://www.amazon.com/sp?seller=&marketplaceID=${activeMarketplaceId}&asin=${drawer.product.asin}`} target="_blank">
                📊 卖家后台
              </Button>
              <Button size="small" onClick={() => {
                setCostModal({ open: true, asin: drawer.product!.asin })
                costForm.resetFields()
              }}>
                💰 录入成本
              </Button>
            </div>
          </div>
        )}
      </Drawer>

      {/* ── 成本录入 Modal ── */}
      <Modal
        title={`录入 COGS — ${costModal.asin}`}
        open={costModal.open}
        onCancel={() => setCostModal({ open: false, asin: '' })}
        onOk={() => costForm.submit()}
        okText="保存" cancelText="取消"
      >
        <Form form={costForm} layout="vertical" onFinish={handleSaveCost}>
          <Form.Item
            name="cost"
            label={`单件采购成本 (${currency})`}
            rules={[{ required: true, message: '请输入采购成本' }, { type: 'number', min: 0, message: '成本不能为负' }]}
            extra="输入每件商品的采购成本（含头程运费），用于计算净利润"
          >
            <InputNumber style={{ width: '100%' }} min={0} precision={2} prefix={currency} />
          </Form.Item>
        </Form>
      </Modal>

      {/* ── 多 ASIN 对比 Modal ── */}
      <Modal
        title="📊 多 ASIN 趋势对比"
        open={compareOpen}
        onCancel={() => { setCompareOpen(false); compareChartInst.current?.dispose(); compareChartInst.current = null }}
        footer={null}
        width={780}
        afterOpenChange={(open) => {
          if (!open) return
          setTimeout(() => {
            if (compareChartRef.current && !compareChartInst.current) {
              compareChartInst.current = echarts.init(compareChartRef.current, undefined, { renderer: 'canvas' })
            }
            renderCompareChart()
          }, 150)
        }}
      >
        <div style={{ marginBottom: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>指标：</span>
          <Select value={compareMetric} onChange={(v) => { setCompareMetric(v); setTimeout(renderCompareChart, 50) }}
            size="small" style={{ width: 120 }}
            options={[
              { value: 'sales', label: '销售额' },
              { value: 'units', label: '销量' },
              { value: 'sessions', label: '访客数' },
            ]}
          />
          <span style={{ fontSize: 11, color: '#9ca3af', marginLeft: 'auto' }}>
            对比 {selectedAsins.length} 个 ASIN · 近 30 天
          </span>
        </div>
        <div ref={compareChartRef} style={{ height: 360, background: '#f9fafb', borderRadius: 8 }} />
      </Modal>
    </div>
  )

  function renderCompareChart() {
    if (!compareChartInst.current || compareLoading) return
    const colors = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6']
    // 获取联合日期轴
    const allDates = new Set<string>()
    Object.values(compareData).forEach(arr => arr.forEach((d: any) => allDates.add(d.date)))
    const sortedDates = [...allDates].sort()

    const metricLabel = compareMetric === 'sales' ? '销售额' : compareMetric === 'units' ? '销量' : '访客数'
    const series = selectedAsins.map((asin, i) => {
      const asinData = compareData[asin] || []
      const dataMap = new Map(asinData.map((d: any) => [d.date, d]))
      return {
        name: asin.length > 12 ? asin.slice(0, 12) + '…' : asin,
        type: 'line' as const,
        smooth: true,
        data: sortedDates.map(date => {
          const d = dataMap.get(date) as any
          if (!d) return null
          return compareMetric === 'sales' ? d.sales : compareMetric === 'units' ? d.units : d.sessions
        }),
        itemStyle: { color: colors[i % colors.length] },
        lineStyle: { width: 2 },
      }
    })

    compareChartInst.current.setOption({
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, textStyle: { fontSize: 10 } },
      grid: { top: 16, right: 20, bottom: 45, left: 55 },
      xAxis: { type: 'category', data: sortedDates, axisLabel: { fontSize: 10, rotate: 30 } },
      yAxis: { type: 'value', name: metricLabel, axisLabel: { fontSize: 10 } },
      series,
    }, true)
  }
}
