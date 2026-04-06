import { useState, useCallback } from 'react'
import { Modal, Button, Upload, Table, Alert, Progress, message, Tabs, Input, InputNumber, Form, Space } from 'antd'
import {
  UploadOutlined, CheckCircleOutlined, CloseCircleOutlined,
  PlusOutlined, DownloadOutlined, InfoCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { ImportCostCSV, GetProductCosts, SaveProductCost, GetSyncHistory } from '../../wailsjs/go/main/App'

interface CostRow { sku: string; cost: number; currency: string; date: string }

interface CostImportModalProps {
  open: boolean
  onClose: () => void
  accountId: number
  defaultCurrency?: string
}

// ── CSV 成本批量导入 Modal ──────────────────────────────
export function CostImportModal({ open, onClose, accountId, defaultCurrency = 'USD' }: CostImportModalProps) {
  const [csvText, setCsvText] = useState('')
  const [importing, setImporting] = useState(false)
  const [progress, setProgress] = useState<{ imported: number; errors: string[] } | null>(null)

  const handleFile = (file: File) => {
    const reader = new FileReader()
    reader.onload = e => setCsvText(e.target?.result as string)
    reader.readAsText(file, 'UTF-8')
    return false // 阻止 antd 自动上传
  }

  const handleImport = async () => {
    if (!csvText.trim()) {
      message.warning('请先上传或粘贴 CSV 内容')
      return
    }
    setImporting(true)
    try {
      const result = await ImportCostCSV(accountId, csvText, defaultCurrency)
      setProgress({ imported: result?.imported ?? 0, errors: result?.errors ?? [] })
      if ((result?.imported ?? 0) > 0) {
        message.success(`成功导入 ${result.imported} 条成本记录`)
      }
    } catch (e: any) {
      message.error('导入失败: ' + (e?.message ?? String(e)))
    } finally { setImporting(false) }
  }

  const templateCSV = `SKU,成本(${defaultCurrency}),货币\nEXAMPLE-SKU-001,12.50,${defaultCurrency}\nEXAMPLE-SKU-002,8.00,${defaultCurrency}`
  const downloadTemplate = () => {
    const blob = new Blob([templateCSV], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = 'cost_template.csv'; a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Modal title="📥 批量导入 COGS（商品成本）" open={open} onCancel={onClose}
      footer={[
        <Button key="dl" icon={<DownloadOutlined />} onClick={downloadTemplate}>下载模板</Button>,
        <Button key="cancel" onClick={onClose}>取消</Button>,
        <Button key="import" type="primary" loading={importing} onClick={handleImport}>开始导入</Button>,
      ]}
      width={640}
    >
      <Alert type="info" style={{ marginBottom: 16 }}
        icon={<InfoCircleOutlined />} showIcon
        description={<>
          <div>CSV 格式：<code>SKU, 单件成本, 货币（可选）</code></div>
          <div style={{ marginTop: 4 }}>支持带引号的字段，第一行如含 "sku" 字样将自动跳过表头。</div>
        </>}
      />
      <Upload.Dragger accept=".csv,.txt" beforeUpload={handleFile} showUploadList={false}
        style={{ marginBottom: 12 }}>
        <p className="ant-upload-drag-icon"><UploadOutlined /></p>
        <p>点击或拖拽 CSV 文件到此处</p>
        <p style={{ fontSize: 12, color: '#9ca3af' }}>仅支持 .csv / .txt 格式</p>
      </Upload.Dragger>

      <div style={{ marginBottom: 8, fontSize: 12, color: '#6b7280' }}>或直接粘贴 CSV 内容：</div>
      <Input.TextArea
        value={csvText}
        onChange={e => setCsvText(e.target.value)}
        rows={6}
        placeholder={`SKU,成本\nEXAMPLE-SKU-001,12.50\nEXAMPLE-SKU-002,8.00`}
        style={{ fontFamily: 'monospace', fontSize: 12 }}
      />

      {progress && (
        <div style={{ marginTop: 16 }}>
          <div style={{ display: 'flex', gap: 12, marginBottom: 8 }}>
            <span style={{ color: '#10b981' }}>
              <CheckCircleOutlined /> 成功导入 {progress.imported} 条
            </span>
            {progress.errors.length > 0 && (
              <span style={{ color: '#ef4444' }}>
                <CloseCircleOutlined /> {progress.errors.length} 条错误
              </span>
            )}
          </div>
          {progress.errors.length > 0 && (
            <div style={{ fontSize: 11, color: '#ef4444', background: '#fef2f2',
              padding: 8, borderRadius: 4, maxHeight: 100, overflowY: 'auto' }}>
              {progress.errors.map((e, i) => <div key={i}>{e}</div>)}
            </div>
          )}
        </div>
      )}
    </Modal>
  )
}

// ── 成本管理面板（内嵌于设置页） ──────────────────────
interface CostManagerProps { accountId: number; currency?: string }

export function CostManager({ accountId, currency = 'USD' }: CostManagerProps) {
  const [costs, setCosts] = useState<CostRow[]>([])
  const [loading, setLoading] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [form] = Form.useForm()

  const loadCosts = useCallback(async () => {
    setLoading(true)
    try {
      const data = await GetProductCosts(accountId)
      setCosts((data ?? []) as unknown as CostRow[])
    } finally { setLoading(false) }
  }, [accountId])

  const handleSingle = async (values: any) => {
    await SaveProductCost(accountId, values.sku, currency, values.cost)
    message.success('成本已保存')
    form.resetFields()
    loadCosts()
  }

  const cols: ColumnsType<CostRow> = [
    { title: 'SKU', dataIndex: 'sku', render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: `成本 (${currency})`, dataIndex: 'cost', render: v => <span className="amount">{v.toFixed(2)}</span> },
    { title: '币种', dataIndex: 'currency', width: 80 },
    { title: '更新日期', dataIndex: 'date', width: 110 },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginBottom: 12 }}>
        <Button icon={<UploadOutlined />} onClick={() => { setImportOpen(true); loadCosts() }}>批量 CSV 导入</Button>
        <Button icon={<DownloadOutlined />} onClick={loadCosts}>刷新列表</Button>
      </div>

      <Form form={form} layout="inline" onFinish={handleSingle} style={{ marginBottom: 12 }}>
        <Form.Item name="sku" rules={[{ required: true }]}>
          <Input placeholder="SKU" style={{ width: 180 }} />
        </Form.Item>
        <Form.Item name="cost" rules={[{ required: true }]}>
          <InputNumber placeholder="成本" min={0} precision={2} style={{ width: 120 }} prefix={currency} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<PlusOutlined />}>添加单条</Button>
        </Form.Item>
      </Form>

      <Table columns={cols} dataSource={costs} rowKey="sku" loading={loading}
        size="small" pagination={{ pageSize: 10, showSizeChanger: false }} />

      <CostImportModal open={importOpen} onClose={() => { setImportOpen(false); loadCosts() }}
        accountId={accountId} defaultCurrency={currency} />
    </div>
  )
}

// ── 同步历史面板 ──────────────────────────────────────
interface SyncHistoryProps { accountId: number }
interface SyncRecord {
  syncType: string; status: string; startedAt: string
  completedAt: string; records: number; error: string; duration: string
}

export function SyncHistory({ accountId }: SyncHistoryProps) {
  const [history, setHistory] = useState<SyncRecord[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await GetSyncHistory(accountId)
      setHistory((data ?? []) as unknown as SyncRecord[])
    } finally { setLoading(false) }
  }, [accountId])

  const typeLabels: Record<string, string> = {
    orders: '📦 订单', inventory: '🏪 库存', datakiosk: '📊 Data Kiosk',
    advertising: '📢 广告', financial_events: '💰 财务事件', settlement_reports: '📄 结算报告',
  }

  const cols: ColumnsType<SyncRecord> = [
    { title: '同步类型', dataIndex: 'syncType', width: 140,
      render: v => typeLabels[v] ?? v },
    { title: '状态', dataIndex: 'status', width: 80,
      render: v => <span style={{ color: v === 'success' ? '#10b981' : v === 'running' ? '#f59e0b' : '#ef4444',
        fontWeight: 600 }}>{v === 'success' ? '✓ 成功' : v === 'running' ? '⟳ 运行中' : '✗ 失败'}</span> },
    { title: '开始时间', dataIndex: 'startedAt', width: 150 },
    { title: '耗时', dataIndex: 'duration', width: 90 },
    { title: '记录数', dataIndex: 'records', width: 80,
      render: v => <span className="num">{v}</span> },
    { title: '错误信息', dataIndex: 'error', ellipsis: true,
      render: v => v ? <span style={{ color: '#ef4444', fontSize: 12 }}>{v}</span> : '-' },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 8 }}>
        <Button onClick={load} loading={loading}>刷新</Button>
      </div>
      <Table columns={cols} dataSource={history} rowKey={r => r.startedAt + r.syncType}
        loading={loading} size="small" pagination={{ pageSize: 15 }} />
    </div>
  )
}
