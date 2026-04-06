import { Component, ErrorInfo, ReactNode } from 'react'
import { Button, Result } from 'antd'
import { BugOutlined, ReloadOutlined } from '@ant-design/icons'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
  errorInfo: ErrorInfo | null
}

/**
 * 全局错误边界组件
 * 捕获子组件树中的未处理 JavaScript 错误，防止整个应用崩溃
 */
export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null, errorInfo: null }
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    this.setState({ errorInfo })
    // 实际项目中可接入错误监控（Sentry 等）
    console.error('[Voyage ErrorBoundary]', error, errorInfo)
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null, errorInfo: null })
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback

      return (
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          minHeight: 300, padding: 24,
        }}>
          <Result
            icon={<BugOutlined style={{ color: '#ef4444' }} />}
            title="页面渲染出错"
            subTitle={
              <div>
                <div style={{ marginBottom: 8, color: '#6b7280' }}>
                  {this.state.error?.message ?? '未知错误'}
                </div>
                <details style={{ textAlign: 'left', maxWidth: 600 }}>
                  <summary style={{ cursor: 'pointer', color: '#9ca3af', fontSize: 12 }}>
                    查看错误详情
                  </summary>
                  <pre style={{
                    fontSize: 11, background: '#fef2f2', padding: 12,
                    borderRadius: 4, marginTop: 8, overflow: 'auto',
                    maxHeight: 200, color: '#ef4444',
                  }}>
                    {this.state.errorInfo?.componentStack}
                  </pre>
                </details>
              </div>
            }
            extra={[
              <Button key="retry" type="primary" icon={<ReloadOutlined />}
                style={{ background: '#1a2744' }} onClick={this.handleReset}>
                重试
              </Button>,
              <Button key="reload" onClick={() => window.location.reload()}>
                刷新页面
              </Button>,
            ]}
          />
        </div>
      )
    }

    return this.props.children
  }
}
