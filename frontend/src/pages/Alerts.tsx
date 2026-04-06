import { useState, useEffect, useCallback } from 'react'
import { Tabs, Empty, Button, Badge, Spin, message } from 'antd'
import {
  ExclamationCircleOutlined, WarningOutlined, InfoCircleOutlined,
  CheckOutlined, CloseOutlined, ReloadOutlined,
} from '@ant-design/icons'
import { useAppStore } from '../stores/appStore'

// @ts-ignore
import { GetAlerts, MarkAlertRead, DismissAlert, GetUnreadAlertCount } from '../../wailsjs/go/main/App'

interface Alert {
  id: number; alertType: string; severity: string
  title: string; message: string
  relatedEntityType: string; relatedEntityId: string
  isRead: boolean; isDismissed: boolean; createdAt: string
}

const severityConfig = {
  critical: { color: '#ef4444', bg: '#fef2f2', border: '#ef4444', icon: <ExclamationCircleOutlined />, label: '紧急' },
  warning:  { color: '#f59e0b', bg: '#fffbeb', border: '#f59e0b', icon: <WarningOutlined />, label: '警告' },
  info:     { color: '#3b82f6', bg: '#eff6ff', border: '#3b82f6', icon: <InfoCircleOutlined />, label: '提示' },
}

const alertTypeLabels: Record<string, string> = {
  low_inventory:   '📦 库存预警',
  high_acos:       '📢 ACoS 预警',
  sales_drop:      '📉 销售下滑',
  listing_inactive: '🚫 Listing 下架',
  price_change:    '💲 价格变动',
}

export default function Alerts() {
  const { activeAccountId, setUnreadAlertCount } = useAppStore()
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [loading, setLoading] = useState(false)
  const [tab, setTab] = useState<'unread' | 'all'>('unread')

  const fetchAlerts = useCallback(async () => {
    if (!activeAccountId) return
    setLoading(true)
    try {
      const data = await GetAlerts(activeAccountId, tab === 'unread')
      setAlerts(data ?? [])

      // 刷新未读计数
      const count = await GetUnreadAlertCount(activeAccountId)
      setUnreadAlertCount(Number(count) || 0)
    } catch { setAlerts([]) }
    finally { setLoading(false) }
  }, [activeAccountId, tab])

  useEffect(() => { fetchAlerts() }, [fetchAlerts])

  const handleMarkRead = async (id: number) => {
    try {
      await MarkAlertRead(id)
      setAlerts(prev => prev.map(a => a.id === id ? { ...a, isRead: true } : a))
      const count = await GetUnreadAlertCount(activeAccountId!)
      setUnreadAlertCount(Number(count) || 0)
    } catch { message.error('操作失败') }
  }

  const handleDismiss = async (id: number) => {
    try {
      await DismissAlert(id)
      setAlerts(prev => prev.filter(a => a.id !== id))
      const count = await GetUnreadAlertCount(activeAccountId!)
      setUnreadAlertCount(Number(count) || 0)
    } catch { message.error('操作失败') }
  }

  const critical = alerts.filter(a => a.severity === 'critical')
  const warning  = alerts.filter(a => a.severity === 'warning')
  const info     = alerts.filter(a => a.severity === 'info')

  const AlertCard = ({ alert }: { alert: Alert }) => {
    const cfg = severityConfig[alert.severity as keyof typeof severityConfig] ?? severityConfig.info
    return (
      <div className="alert-card" style={{
        borderLeftColor: cfg.border,
        background: alert.isRead ? '#fafafa' : cfg.bg,
        opacity: alert.isRead ? 0.7 : 1,
      }}>
        <div className="alert-icon" style={{ color: cfg.color }}>{cfg.icon}</div>
        <div className="alert-content">
          <div className="alert-title">
            {alert.title}
            {!alert.isRead && (
              <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%',
                background: cfg.color, marginLeft: 8, verticalAlign: 'middle' }} />
            )}
          </div>
          <div className="alert-message">{alert.message}</div>
          <div style={{ marginTop: 6, display: 'flex', gap: 12, alignItems: 'center' }}>
            <span style={{ fontSize: 10, color: 'var(--color-text-muted)' }}>
              {alertTypeLabels[alert.alertType] ?? alert.alertType}
            </span>
            {alert.relatedEntityId && (
              <span style={{ fontSize: 10, fontFamily: 'monospace', color: 'var(--color-text-muted)',
                background: 'var(--color-border-light)', padding: '1px 6px', borderRadius: 3 }}>
                {alert.relatedEntityId}
              </span>
            )}
          </div>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 6, flexShrink: 0 }}>
          <span className="alert-time">{alert.createdAt?.slice(5, 16)}</span>
          <div style={{ display: 'flex', gap: 4 }}>
            {!alert.isRead && (
              <Button size="small" type="text" icon={<CheckOutlined />}
                style={{ color: '#10b981' }} onClick={() => handleMarkRead(alert.id)}>
                已读
              </Button>
            )}
            <Button size="small" type="text" icon={<CloseOutlined />}
              style={{ color: 'var(--color-text-muted)' }} onClick={() => handleDismiss(alert.id)}>
              忽略
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const sections = [
    { severity: 'critical', label: '🔴 紧急', items: critical },
    { severity: 'warning',  label: '🟡 警告', items: warning },
    { severity: 'info',     label: '🔵 提示', items: info },
  ]

  return (
    <div>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div>
            <div className="page-title">🔔 智能预警</div>
            <div className="page-subtitle">基于库存天数、ACoS、销售波动的自动预警</div>
          </div>
          <div style={{ marginLeft: 'auto' }}>
            <Button icon={<ReloadOutlined />} onClick={fetchAlerts}>刷新</Button>
          </div>
        </div>
      </div>

      {/* 统计卡片 */}
      <div style={{ display: 'flex', gap: 16, marginBottom: 20 }}>
        {[
          { label: '🔴 紧急', count: critical.length, color: '#ef4444' },
          { label: '🟡 警告', count: warning.length, color: '#f59e0b' },
          { label: '🔵 提示', count: info.length, color: '#3b82f6' },
          { label: '📬 未读总计', count: alerts.filter(a => !a.isRead).length, color: '#1a2744' },
        ].map(card => (
          <div key={card.label} className="voyage-card" style={{ flex: 1, padding: '14px 18px' }}>
            <div style={{ fontSize: 12, color: 'var(--color-text-secondary)', marginBottom: 4 }}>{card.label}</div>
            <div style={{ fontSize: 28, fontWeight: 700, color: card.color }}>{card.count}</div>
          </div>
        ))}
      </div>

      {/* 筛选 Tab */}
      <Tabs activeKey={tab} onChange={v => setTab(v as any)} style={{ marginBottom: 16 }}
        items={[
          { key: 'unread', label: <Badge count={alerts.filter(a=>!a.isRead).length} size="small">未读预警</Badge> },
          { key: 'all', label: '全部记录' },
        ]}
      />

      {loading ? (
        <div className="voyage-loading"><Spin size="large" /></div>
      ) : alerts.length === 0 ? (
        <div className="voyage-card">
          <Empty style={{ padding: '60px 0' }} description={
            <span style={{ color: 'var(--color-text-muted)' }}>
              {tab === 'unread' ? '暂无未读预警，系统运行正常 ✓' : '暂无预警记录'}
            </span>
          } />
        </div>
      ) : (
        sections.map(section => (
          section.items.length > 0 && (
            <div key={section.severity} style={{ marginBottom: 20 }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-text-secondary)',
                marginBottom: 10, paddingLeft: 4 }}>
                {section.label}（{section.items.length} 条）
              </div>
              {section.items.map(alert => (
                <AlertCard key={alert.id} alert={alert} />
              ))}
            </div>
          )
        ))
      )}
    </div>
  )
}
