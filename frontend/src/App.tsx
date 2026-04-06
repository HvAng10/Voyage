import { useEffect, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Spin } from 'antd'
import { useAppStore } from './stores/appStore'
import MainLayout from './components/Layout/MainLayout'
import ErrorBoundary from './components/ErrorBoundary'
import Dashboard from './pages/Dashboard'
import Sales from './pages/Sales'
import Inventory from './pages/Inventory'
import Advertising from './pages/Advertising'
import Finance from './pages/Finance'
import Products from './pages/Products'
import Alerts from './pages/Alerts'
import Settings from './pages/Settings'

// Wails Go 绑定（wails dev 运行时自动生成）
// @ts-ignore
import { GetAccounts, GetMarketplaces, GetCurrentTimes, GetUnreadAlertCount } from '../wailsjs/go/main/App'

// 页面 Suspense Fallback
const PageSpin = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 300 }}>
    <Spin size="large" tip="加载中..." />
  </div>
)

// 用 ErrorBoundary 包裹每个页面，隔离单页错误
const guard = (el: React.ReactElement) => (
  <ErrorBoundary>
    <Suspense fallback={<PageSpin />}>{el}</Suspense>
  </ErrorBoundary>
)

export default function App() {
  const { setAccounts, setMarketplaces, setActiveAccount, setUnreadAlertCount, setTimes, storeTimezone, activeAccountId } = useAppStore()

  // 初始化：加载账户和站点数据
  useEffect(() => {
    const init = async () => {
      try {
        const [accounts, marketplaces] = await Promise.all([
          GetAccounts(),
          GetMarketplaces(),
        ])
        setMarketplaces(marketplaces ?? [])
        setAccounts(accounts ?? [])
        // 自动激活第一个账户
        if (accounts?.length > 0) {
          setActiveAccount(accounts[0].id)
        }
      } catch (err) {
        console.error('初始化数据加载失败:', err)
      }
    }
    init()
  }, [])

  // 切换账户时刷新侧边栏智能预警红点计数
  useEffect(() => {
    if (!activeAccountId) {
      setUnreadAlertCount(0)
      return
    }
    GetUnreadAlertCount(activeAccountId)
      .then((cnt: any) => setUnreadAlertCount(Number(cnt) || 0))
      .catch(() => {})
  }, [activeAccountId])

  // 双时区时钟（每秒刷新）
  useEffect(() => {
    const refreshTime = async () => {
      try {
        const times = await GetCurrentTimes(storeTimezone || '')
        setTimes(times.beijing, times.store, times.timezone)
      } catch {
        const now = new Date()
        const fmt = now.toLocaleString('zh-CN', { hour12: false })
        setTimes(fmt, fmt, '')
      }
    }
    refreshTime()
    const timer = setInterval(refreshTime, 1000)
    return () => clearInterval(timer)
  }, [storeTimezone])

  return (
    // 最外层 ErrorBoundary：捕获 Layout/路由 级别的崩溃
    <ErrorBoundary>
      <MainLayout>
        <Routes>
          <Route path="/"            element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard"   element={guard(<Dashboard />)} />
          <Route path="/sales"       element={guard(<Sales />)} />
          <Route path="/inventory"   element={guard(<Inventory />)} />
          <Route path="/advertising" element={guard(<Advertising />)} />
          <Route path="/finance"     element={guard(<Finance />)} />
          <Route path="/products"    element={guard(<Products />)} />
          <Route path="/alerts"      element={guard(<Alerts />)} />
          <Route path="/settings"    element={guard(<Settings />)} />
          <Route path="*"            element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </MainLayout>
    </ErrorBoundary>
  )
}
