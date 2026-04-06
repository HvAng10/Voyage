import { NavLink, useNavigate } from 'react-router-dom'
import { Dropdown, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import {
  DashboardOutlined,
  LineChartOutlined,
  InboxOutlined,
  FundProjectionScreenOutlined,
  WalletOutlined,
  ShoppingOutlined,
  BellOutlined,
  SettingOutlined,
  SwapOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ReloadOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons'
import { useState } from 'react'
import { useAppStore } from '../../stores/appStore'

// 旗帜 emoji 映射（按国家代码）
const countryFlags: Record<string, string> = {
  US: '🇺🇸', CA: '🇨🇦', MX: '🇲🇽', BR: '🇧🇷',
  DE: '🇩🇪', GB: '🇬🇧', FR: '🇫🇷', IT: '🇮🇹', ES: '🇪🇸',
  NL: '🇳🇱', SE: '🇸🇪', PL: '🇵🇱', TR: '🇹🇷', AE: '🇦🇪',
  SA: '🇸🇦', EG: '🇪🇬', IN: '🇮🇳', BE: '🇧🇪',
  JP: '🇯🇵', AU: '🇦🇺', SG: '🇸🇬',
}

const navItems = [
  { path: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
  { path: '/sales', icon: <LineChartOutlined />, label: '销售分析' },
  { path: '/inventory', icon: <InboxOutlined />, label: '库存管理' },
  { path: '/advertising', icon: <FundProjectionScreenOutlined />, label: '广告分析' },
  { path: '/finance', icon: <WalletOutlined />, label: '财务报表' },
  { path: '/products', icon: <ShoppingOutlined />, label: '商品管理' },
]

interface MainLayoutProps {
  children: React.ReactNode
}

export default function MainLayout({ children }: MainLayoutProps) {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()

  const {
    accounts, activeAccountId, activeMarketplaceId,
    marketplaces, beijingTime, storeTime, storeTimezone,
    syncState, unreadAlertCount,
    setActiveAccount, setActiveMarketplace,
  } = useAppStore()

  // 当前活跃账户和站点
  const activeAccount = accounts.find(a => a.id === activeAccountId)
  const activeMarketplace = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)

  // 账户可用站点
  const accountMarketplaces = marketplaces.filter(m =>
    activeAccount?.marketplaces?.includes(m.marketplaceId)
  )

  // 店铺切换器下拉菜单
  const storeMenuItems: MenuProps['items'] = [
    {
      type: 'group',
      label: '切换店铺',
      children: accounts.map(acc => ({
        key: `account-${acc.id}`,
        label: acc.name,
        onClick: () => setActiveAccount(acc.id),
      })),
    },
    { type: 'divider' },
    {
      type: 'group',
      label: '切换站点',
      children: accountMarketplaces.map(mp => ({
        key: `mp-${mp.marketplaceId}`,
        label: `${countryFlags[mp.countryCode] ?? '🌐'} ${mp.name} (${mp.currencyCode})`,
        onClick: () => setActiveMarketplace(mp.marketplaceId),
      })),
    },
  ]

  // 同步状态图标
  const SyncIcon = () => {
    switch (syncState.status) {
      case 'syncing': return <SyncOutlined spin style={{ color: '#f59e0b' }} />
      case 'success': return <CheckCircleOutlined style={{ color: '#10b981' }} />
      case 'error': return <ExclamationCircleOutlined style={{ color: '#ef4444' }} />
      default: return <ReloadOutlined style={{ color: '#9ca3af' }} />
    }
  }

  // 店铺时区缩写
  const tzAbbr = activeMarketplace
    ? new Date().toLocaleTimeString('en-US', {
      timeZone: activeMarketplace.timezone,
      timeZoneName: 'short',
    }).split(' ').pop() ?? ''
    : ''

  return (
    <div className="voyage-layout">
      {/* ── 侧边栏 ─────────────────────────────── */}
      <aside className={`voyage-sidebar${collapsed ? ' collapsed' : ''}`}>
        {/* Logo */}
        <div className="sidebar-logo">
          <img
            src="/appicon.png"
            alt="Voyage"
            className="sidebar-logo-icon"
            onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
          />
          {!collapsed && (
            <div className="sidebar-logo-text">
              V<span>oyage</span>
            </div>
          )}
        </div>

        {/* 导航菜单 */}
        <nav className="sidebar-nav">
          {!collapsed && (
            <div className="nav-section-label">主菜单</div>
          )}

          {navItems.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) =>
                `nav-item${isActive ? ' active' : ''}`
              }
            >
              <span className="nav-item-icon">{item.icon}</span>
              {!collapsed && (
                <span className="nav-item-label">{item.label}</span>
              )}
            </NavLink>
          ))}

          {!collapsed && (
            <div className="nav-section-label" style={{ marginTop: 8 }}>系统</div>
          )}

          {/* 预警（带角标） */}
          <NavLink
            to="/alerts"
            className={({ isActive }) =>
              `nav-item${isActive ? ' active' : ''}`
            }
          >
            <span className="nav-item-icon"><BellOutlined /></span>
            {!collapsed && (
              <>
                <span className="nav-item-label">智能预警</span>
                {unreadAlertCount > 0 && (
                  <span className="nav-item-badge">
                    {unreadAlertCount > 99 ? '99+' : unreadAlertCount}
                  </span>
                )}
              </>
            )}
          </NavLink>

          <NavLink
            to="/settings"
            className={({ isActive }) =>
              `nav-item${isActive ? ' active' : ''}`
            }
          >
            <span className="nav-item-icon"><SettingOutlined /></span>
            {!collapsed && (
              <span className="nav-item-label">系统设置</span>
            )}
          </NavLink>
        </nav>

        {/* 折叠按钮 */}
        <div className="sidebar-footer">
          <Tooltip title={collapsed ? '展开菜单' : '收起菜单'} placement="right">
            <div
              className="nav-item"
              onClick={() => setCollapsed(!collapsed)}
              style={{ justifyContent: collapsed ? 'center' : undefined }}
            >
              <span className="nav-item-icon">
                {collapsed
                  ? <MenuUnfoldOutlined />
                  : <MenuFoldOutlined />
                }
              </span>
              {!collapsed && (
                <span className="nav-item-label">收起菜单</span>
              )}
            </div>
          </Tooltip>
        </div>
      </aside>

      {/* ── 右侧主体 ────────────────────────────── */}
      <div className="voyage-main">
        {/* 顶栏 */}
        <header className="voyage-topbar">
          {/* 面包屑（当前页面名称，由子页面控制） */}
          <div className="topbar-breadcrumb">
            <strong>Voyage</strong>
            <span style={{ margin: '0 8px', opacity: 0.4 }}>·</span>
            <span>亚马逊卖家数据分析</span>
          </div>

          <div className="topbar-actions">
            {/* 双时区时钟 */}
            <div className="timezone-clock">
              <div className="timezone-clock-row">
                <span className="timezone-clock-icon">🕐</span>
                <span className="timezone-clock-label">北京</span>
                <span className="timezone-clock-time">{beijingTime}</span>
              </div>
              {activeMarketplace && (
                <div className="timezone-clock-row">
                  <span className="timezone-clock-icon">🌍</span>
                  <span className="timezone-clock-label">店铺</span>
                  <span className="timezone-clock-time">{storeTime}</span>
                  {tzAbbr && (
                    <span className="timezone-clock-tz">{tzAbbr}</span>
                  )}
                </div>
              )}
            </div>

            {/* 店铺/站点切换器 */}
            <Dropdown
              menu={{ items: storeMenuItems }}
              trigger={['click']}
              placement="bottomRight"
            >
              <div className="store-switcher">
                <span className="store-switcher-flag">
                  {activeMarketplace
                    ? (countryFlags[activeMarketplace.countryCode] ?? '🌐')
                    : '🏪'
                  }
                </span>
                <span>
                  {activeAccount?.name ?? '选择店铺'}
                  {activeMarketplace ? ` · ${activeMarketplace.name}` : ''}
                </span>
                <SwapOutlined style={{ fontSize: 12, opacity: 0.6 }} />
              </div>
            </Dropdown>
          </div>
        </header>

        {/* 内容区 */}
        <main className="voyage-content">
          {children}
        </main>

        {/* 底栏 */}
        <footer className="voyage-bottombar">
          <div className="bottombar-sync-status">
            <div className={`sync-dot ${syncState.status}`} />
            <span>{syncState.message}</span>
          </div>

          {syncState.lastSyncTime && (
            <>
              <div className="bottombar-divider" />
              <span>
                最后同步：{syncState.lastSyncTime}
              </span>
            </>
          )}

          <div style={{ marginLeft: 'auto' }} />

          {activeMarketplace && (
            <>
              <span>{activeMarketplace.name} · {activeMarketplace.currencyCode}</span>
              <div className="bottombar-divider" />
            </>
          )}
          <span>Voyage v2.4.0</span>
        </footer>
      </div>
    </div>
  )
}
