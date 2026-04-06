import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import 'antd/dist/reset.css'
import './styles/global.css'
import App from './App'

// Ant Design 5.x 高级 (Premium) 主题令牌
const antdTheme = {
  token: {
    colorPrimary: '#1a2744',
    colorSuccess: '#059669', // 更深邃沉稳的绿色
    colorWarning: '#d97706', // 高级感橙
    colorError: '#dc2626',
    colorInfo: '#2563eb',
    borderRadius: 12,        // 提升基础组件圆角
    fontFamily: "'Inter', 'Noto Sans SC', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif",
    fontSize: 14,
    colorBgBase: '#ffffff',
    colorTextBase: '#111827', // 加深基础字色对比度
    colorBorder: '#e5e7eb',
    colorBgLayout: '#f4f7fb', // 调整为偏冷的高级灰背
    motionDurationMid: '250ms', // 动画稍长，显优雅
  },
  components: {
    Table: {
      headerBg: '#f8fafc',
      headerColor: '#4b5563',
      headerSplitColor: 'transparent', // 隐藏生硬的表头隔线
      headerSortHoverBg: '#f1f5f9',
      rowHoverBg: '#f8fafc',
      borderRadius: 16, // 表格外沿大圆角
    },
    Card: {
      borderRadius: 16,
      boxShadowTertiary: '0 4px 20px rgba(16, 24, 40, 0.04)', // 软投影覆盖 Antd 的阴影
    },
    Button: {
      controlHeight: 36,
      borderRadius: 8,
      primaryShadow: '0 2px 8px rgba(26,39,68,0.2)', // 按钮给予微投影
    },
    Menu: {
      itemBg: 'transparent',
      itemColor: '#c8d4e8',
      itemHoverColor: '#ffffff',
      itemSelectedColor: '#d4b966',      // 亮金色
      itemSelectedBg: 'rgba(212,185,102,0.12)', // 柔和霓虹背底
      horizontalItemSelectedColor: '#d4b966',
    },
    Select: {
      optionSelectedBg: '#f0f5ff',
    },
  },
}

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={antdTheme}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConfigProvider>
  </React.StrictMode>,
)
