import { useState } from 'react'
import {
  Form, Input, Button, Select, Tabs, Popconfirm,
  Space, Tag, Alert, message, Row, Col,
} from 'antd'
import {
  PlusOutlined, CheckCircleOutlined,
  EyeInvisibleOutlined, EyeTwoTone, LockOutlined,
  GlobalOutlined, ShopOutlined, HistoryOutlined,
  DollarOutlined, DownloadOutlined, ExperimentOutlined, DeleteOutlined,
} from '@ant-design/icons'
import { useAppStore } from '../stores/appStore'
import { CostManager, SyncHistory } from '../components/CostImport'
import dayjs from 'dayjs'

// @ts-ignore
import { CreateAccount, SaveCredential, TestConnection, GetAccounts, GetMarketplaces, ExportDataCSV, TriggerSync, BackupDatabase, GetDatabaseInfo, OpenFileInExplorer, GenerateMockData, ClearMockData, GetUnreadAlertCount } from '../../wailsjs/go/main/App'

const REGIONS = [
  { value: 'na', label: '北美 (NA)', desc: 'sellingpartnerapi-na.amazon.com' },
  { value: 'eu', label: '欧洲 (EU)', desc: 'sellingpartnerapi-eu.amazon.com' },
  { value: 'fe', label: '远东 (FE)', desc: 'sellingpartnerapi-fe.amazon.com' },
]

const CREDENTIAL_FIELDS = [
  { key: 'lwa_client_id', label: 'LWA Client ID', placeholder: 'amzn1.application-oa2-client.xxxxx', hint: '来自 Amazon Developer Console → Security Profile → OAuth2 Credentials', sensitive: false },
  { key: 'lwa_client_secret', label: 'LWA Client Secret', placeholder: 'amzn1.oa2-cs.v1.xxxxx', hint: '来自 Amazon Developer Console，请妥善保管', sensitive: true },
  { key: 'refresh_token', label: 'Refresh Token', placeholder: 'Atzr|Iw...', hint: '通过 SP-API 授权流程获取', sensitive: true },
  { key: 'ads_client_id', label: '广告 API Client ID（可选）', placeholder: 'amzn1.application-oa2-client.xxxxx', hint: '广告 API 应用，不需要广告分析可留空', sensitive: false, optional: true },
  { key: 'ads_profile_id', label: '广告 Profile ID（可选）', placeholder: '1234567890', hint: '通过 GET /v2/profiles 获取', sensitive: false, optional: true },
]

export default function Settings() {
  const { accounts, marketplaces, setAccounts, setMarketplaces, setActiveAccount, setUnreadAlertCount, activeAccountId, activeMarketplaceId } = useAppStore()
  const [newAccountForm] = Form.useForm()
  const [credForm] = Form.useForm()
  const [testLoading, setTestLoading] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)
  const [saveLoading, setSaveLoading] = useState(false)
  const [selectedAccountForCred, setSelectedAccountForCred] = useState<number | null>(null)
  const [exportLoading, setExportLoading] = useState(false)
  const [syncLoading, setSyncLoading] = useState<Record<string, boolean>>({})
  const [backupLoading, setBackupLoading] = useState(false)
  const [dbInfo, setDbInfo] = useState<any>(null)
  const [mockGenerating, setMockGenerating] = useState(false)
  const [mockClearing, setMockClearing] = useState(false)

  const selectedAccount = accounts.find(a => a.id === selectedAccountForCred)
  const currency = marketplaces.find(m => m.marketplaceId === activeMarketplaceId)?.currencyCode ?? 'USD'

  const allMpOptions = marketplaces.map(m => ({
    value: m.marketplaceId,
    label: `${m.name} · ${m.countryCode} (${m.currencyCode})`,
    group: m.region.toUpperCase(),
  }))

  const handleCreateAccount = async (values: any) => {
    try {
      const accountId = await CreateAccount(values.name, values.sellerId, values.region, values.marketplaceIds ?? [])
      message.success(`店铺 "${values.name}" 创建成功`)
      newAccountForm.resetFields()
      setAccounts(await GetAccounts())
      setSelectedAccountForCred(accountId)
    } catch (err: any) {
      message.error('创建账户失败: ' + (err?.message ?? String(err)))
    }
  }

  const handleSaveCreds = async (values: any) => {
    if (!selectedAccountForCred) { message.warning('请先选择店铺'); return }
    setSaveLoading(true)
    try {
      for (const field of CREDENTIAL_FIELDS) {
        if (values[field.key]) await SaveCredential(selectedAccountForCred, field.key, values[field.key])
      }
      message.success('凭证已加密保存 ✓')
    } catch (err: any) {
      message.error('保存失败: ' + (err?.message ?? String(err)))
    } finally { setSaveLoading(false) }
  }

  const handleTestConnection = async () => {
    if (!selectedAccountForCred) { message.warning('请先选择店铺并保存凭证'); return }
    setTestLoading(true)
    setTestResult(null)
    try {
      const raw = await TestConnection(selectedAccountForCred)
      setTestResult({ success: Boolean(raw?.success), message: String(raw?.message ?? '') })
    } catch (err: any) {
      setTestResult({ success: false, message: err?.message ?? '未知错误' })
    } finally { setTestLoading(false) }
  }

  // 导出数据为 CSV 并在文件管理器中显示
  const handleExport = async (dataType: string) => {
    if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
    setExportLoading(true)
    try {
      const end = dayjs().format('YYYY-MM-DD')
      const start = dayjs().subtract(90, 'day').format('YYYY-MM-DD')
      const r: any = await ExportDataCSV(activeAccountId, activeMarketplaceId, dataType, start, end)
      if (r?.success) {
        message.success(`${r.message} (${r.fileSize})`)
        OpenFileInExplorer(r.path)
      } else {
        message.warning(r?.message ?? '导出失败')
      }
    } catch (err: any) {
      message.error('导出失败: ' + (err?.message ?? String(err)))
    } finally { setExportLoading(false) }
  }

  const tabItems = [
    {
      key: 'accounts',
      label: <span><ShopOutlined /> 店铺管理</span>,
      children: (
        <div>
          <Alert type="info" showIcon style={{ marginBottom: 20, borderRadius: 8 }}
            message="凭证安全说明"
            description="所有 API 凭证均使用 AES-256-GCM 加密后存储于本地 SQLite 数据库，密钥基于本机唯一标识派生，不上传至任何服务器。Voyage 仅进行只读操作。"
          />

          {/* 添加店铺 */}
          <div className="settings-block" style={{ marginBottom: 20 }}>
            <div className="settings-block-header"><ShopOutlined style={{ marginRight: 8 }} />添加新店铺账户</div>
            <div className="settings-block-body">
              <Form form={newAccountForm} layout="vertical" onFinish={handleCreateAccount}>
                <Row gutter={16}>
                  <Col span={8}>
                    <Form.Item name="name" label="店铺别名" rules={[{ required: true }]}>
                      <Input placeholder="如：主力美国店" prefix={<ShopOutlined />} />
                    </Form.Item>
                  </Col>
                  <Col span={8}>
                    <Form.Item name="sellerId" label="Amazon Seller ID" rules={[{ required: true }]}>
                      <Input placeholder="A2XXXXXXXXXXXXX" />
                    </Form.Item>
                  </Col>
                  <Col span={8}>
                    <Form.Item name="region" label="SP-API 区域" rules={[{ required: true }]}>
                      <Select placeholder="选择区域">
                        {REGIONS.map(r => (
                          <Select.Option key={r.value} value={r.value}>
                            <strong>{r.label}</strong>
                            <div style={{ fontSize: 11, color: '#9ca3af' }}>{r.desc}</div>
                          </Select.Option>
                        ))}
                      </Select>
                    </Form.Item>
                  </Col>
                </Row>
                <Form.Item name="marketplaceIds" label="运营站点" tooltip="选择该账户需要同步数据的 Amazon 站点">
                  <Select mode="multiple" placeholder="选择运营的站点" allowClear showSearch
                    filterOption={(input, opt) => String(opt?.label ?? '').toLowerCase().includes(input.toLowerCase())}
                    options={allMpOptions} />
                </Form.Item>
                <Button type="primary" htmlType="submit" icon={<PlusOutlined />} style={{ background: '#1a2744' }}>创建店铺</Button>
              </Form>
            </div>
          </div>

          {/* API 凭证配置 */}
          <div className="settings-block" style={{ marginBottom: 20 }}>
            <div className="settings-block-header"><LockOutlined style={{ marginRight: 8 }} />API 凭证配置</div>
            <div className="settings-block-body">
              <Form.Item label="选择店铺账户">
                <Select value={selectedAccountForCred} onChange={setSelectedAccountForCred}
                  placeholder="选择要配置凭证的店铺" style={{ maxWidth: 320 }}>
                  {accounts.map(acc => (
                    <Select.Option key={acc.id} value={acc.id}>
                      {acc.name} ({acc.region?.toUpperCase()})
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>

              {selectedAccountForCred && (
                <>
                  <Alert type="warning" showIcon style={{ marginBottom: 16, borderRadius: 6 }} message="获取凭证指引"
                    description={<div>
                      <p>1. 访问 <a href="https://developer.amazon.com" target="_blank" rel="noopener noreferrer">Amazon Developer Console</a> 注册开发者账户</p>
                      <p>2. 创建 SP-API 应用并申请权限（订单、库存、财务、报告）</p>
                      <p>3. 创建 LWA 应用，获取 Client ID 和 Client Secret</p>
                      <p>4. 完成 OAuth2 授权流程获取 Refresh Token</p>
                      <p>5. <a href="https://developer-docs.amazon.com/sp-api/docs/authorizing-selling-partner-api-applications" target="_blank" rel="noopener noreferrer">SP-API 官方授权指南 →</a></p>
                    </div>}
                  />
                  <Form form={credForm} layout="vertical" onFinish={handleSaveCreds}>
                    {CREDENTIAL_FIELDS.map(field => (
                      <Form.Item key={field.key} name={field.key}
                        label={<Space>{field.label}{field.optional && <Tag color="default">可选</Tag>}</Space>}
                        tooltip={field.hint}>
                        {field.sensitive
                          ? <Input.Password placeholder={field.placeholder} iconRender={v => v ? <EyeTwoTone /> : <EyeInvisibleOutlined />} />
                          : <Input placeholder={field.placeholder} />}
                      </Form.Item>
                    ))}
                    <Space>
                      <Button type="primary" htmlType="submit" loading={saveLoading} icon={<LockOutlined />} style={{ background: '#1a2744' }}>加密保存凭证</Button>
                      <Button onClick={handleTestConnection} loading={testLoading} icon={<CheckCircleOutlined />}>测试连接</Button>
                    </Space>
                  </Form>
                  {testResult && (
                    <Alert type={testResult.success ? 'success' : 'error'} showIcon style={{ marginTop: 16, borderRadius: 6 }} message={testResult.message} />
                  )}
                </>
              )}
            </div>
          </div>

          {/* 同步配置 */}
          <div className="settings-block">
            <div className="settings-block-header"><GlobalOutlined style={{ marginRight: 8 }} />数据同步配置</div>
            <div className="settings-block-body">
              <Row gutter={24}>
                <Col span={12}>
                  <Form.Item label="订单同步频率" tooltip="建议不超过每 30 分钟一次（Amazon API 限流）">
                    <Select defaultValue="30" style={{ width: '100%' }}>
                      <Select.Option value="30">每 30 分钟（推荐）</Select.Option>
                      <Select.Option value="60">每 1 小时</Select.Option>
                      <Select.Option value="120">每 2 小时</Select.Option>
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="历史数据保留时长">
                    <Select defaultValue="365">
                      <Select.Option value="90">3 个月</Select.Option>
                      <Select.Option value="365">1 年（推荐）</Select.Option>
                      <Select.Option value="730">2 年</Select.Option>
                    </Select>
                  </Form.Item>
                </Col>
              </Row>
              <Alert type="info" showIcon style={{ borderRadius: 6 }}
                message="注意：FBA 库存报告间隔至少 30 分钟（Amazon API 限制）；Data Kiosk 数据存在 T+2 延迟。"
              />
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'costs',
      label: <span><DollarOutlined /> 成本管理</span>,
      children: activeAccountId
        ? <div className="settings-block"><div className="settings-block-body">
          <CostManager accountId={activeAccountId} currency={currency} />
        </div></div>
        : <Alert message="请先在右上角选择店铺账户" type="info" showIcon />,
    },
    {
      key: 'export',
      label: <span><DownloadOutlined /> 数据导出</span>,
      children: (
        <div className="settings-block">
          <div className="settings-block-header"><DownloadOutlined style={{ marginRight: 8 }} />CSV 数据导出</div>
          <div className="settings-block-body">
            <Alert type="info" showIcon style={{ marginBottom: 16 }}
              message="导出的 CSV 包含 UTF-8 BOM，可直接用 Excel 打开中文不乱码。导出范围为近 90 天数据。"
            />
            <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
              {[
                { type: 'sales', label: '📊 销售趋势数据', desc: '日期/销售额/销量/浏览量/访客数/转化率' },
                { type: 'inventory', label: '📦 库存快照', desc: 'SKU/ASIN/可售/在途/不可售库存' },
              ].map(item => (
                <div key={item.type} className="voyage-card" style={{ padding: 16, minWidth: 220 }}>
                  <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 6 }}>{item.label}</div>
                  <div style={{ fontSize: 12, color: 'var(--color-text-secondary)', marginBottom: 12 }}>{item.desc}</div>
                  <Button type="primary" size="small" loading={exportLoading} icon={<DownloadOutlined />}
                    style={{ background: '#1a2744' }}
                    onClick={() => handleExport(item.type)}>
                    导出 CSV
                  </Button>
                </div>
              ))}
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'sync',
      label: <span>🔄 数据同步与备份</span>,
      children: (
        <div className="settings-block">
          <div className="settings-block-header">🔄 手动同步</div>
          <div className="settings-block-body">
            <Alert type="info" showIcon style={{ marginBottom: 16 }}
              message="点击下方按钮可立即触发对应类型的数据同步。同步完成后数据会自动刷新。" />
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 24 }}>
              {[
                { type: 'orders', label: '📦 订单数据', delay: '延迟 ~15分钟' },
                { type: 'datakiosk', label: '📊 销售流量', delay: '延迟 T+2 天' },
                { type: 'inventory', label: '📦 FBA 库存', delay: '延迟 ~30分钟' },
                { type: 'ads', label: '📢 广告数据', delay: '延迟 T+3 天' },
                { type: 'pricing', label: '💰 竞品价格', delay: '延迟 ~15分钟' },
                { type: 'finance', label: '💵 财务事件', delay: '默认近 60 天' },
              ].map(item => (
                <div key={item.type} className="voyage-card" style={{ padding: '14px 16px' }}>
                  <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>{item.label}</div>
                  <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 10 }}>{item.delay}</div>
                  <Button size="small" type="primary" loading={syncLoading[item.type]}
                    style={{ background: '#1a2744' }}
                    disabled={!activeAccountId}
                    onClick={async () => {
                      if (!activeAccountId || !activeMarketplaceId) { message.warning('请先选择账户和站点'); return }
                      setSyncLoading(p => ({ ...p, [item.type]: true }))
                      try {
                        await TriggerSync(activeAccountId, activeMarketplaceId, item.type)
                        message.success(`${item.label} 同步已触发`)
                      } catch (e: any) { message.error(`同步失败: ${e?.message ?? e}`) }
                      finally { setSyncLoading(p => ({ ...p, [item.type]: false })) }
                    }}
                  >开始同步</Button>
                </div>
              ))}
            </div>

            <div className="settings-block-header" style={{ marginTop: 16 }}>💾 数据库备份</div>
            <Alert type="warning" showIcon style={{ marginBottom: 16 }}
              message="自动备份已配置：每周日凌晨2点执行，保留最近 4 份。也可点击下方按钮手动备份。" />
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <Button type="primary" loading={backupLoading}
                style={{ background: '#1a2744' }}
                onClick={async () => {
                  setBackupLoading(true)
                  try {
                    const r: any = await BackupDatabase('')
                    if (r?.success) { message.success(r.message) }
                    else { message.error(r?.message ?? '备份失败') }
                  } catch { message.error('备份失败') } finally { setBackupLoading(false) }
                }}
              >💾 立即备份</Button>
              <Button onClick={async () => {
                const info: any = await GetDatabaseInfo()
                setDbInfo(info)
              }}>📊 查看数据库信息</Button>
              {dbInfo && (
                <div style={{ marginTop: 12, background: 'var(--color-bg-secondary)', borderRadius: 8, padding: '12px 16px', fontSize: 12, lineHeight: 2 }}>
                  <div><strong>📁 数据目录：</strong><code style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{dbInfo.dataDir}</code></div>
                  <div><strong>🗄️ 数据库：</strong><code style={{ fontSize: 11 }}>{dbInfo.dbPath}</code> · {dbInfo.sizeMB} MB · {dbInfo.tableCount} 张表</div>
                  <div><strong>📊 PDF 报告：</strong><code style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{dbInfo.reportsDir}</code></div>
                  <div><strong>💾 数据库备份：</strong><code style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{dbInfo.backupsDir}</code></div>
                  <div><strong>📤 导出文件：</strong><code style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{dbInfo.exportsDir}</code></div>
                </div>
              )}
            </div>

            {/* ── 开发者工具：模拟数据 ── */}
            <div className="settings-block-header" style={{ marginTop: 24 }}>🧪 开发者工具</div>
            <Alert type="info" showIcon style={{ marginBottom: 16 }}
              message="用于演示和测试：生成模拟数据可填充 8 款商品的 90 天完整业务数据（销售、订单、广告、库存等），清空数据将删除所有业务数据但保留表结构。" />
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <Button
                type="primary"
                loading={mockGenerating}
                style={{ background: '#059669' }}
                onClick={async () => {
                  setMockGenerating(true)
                  try {
                    const r: any = await GenerateMockData()
                    if (r?.success) {
                      message.success(r.message)
                      // 重新加载账户和站点数据，刷新右上角切换器
                      const [newAccounts, newMps] = await Promise.all([GetAccounts(), GetMarketplaces()])
                      setMarketplaces(newMps ?? [])
                      setAccounts(newAccounts ?? [])
                      if (newAccounts?.length > 0) {
                        const firstId = newAccounts[0].id
                        setActiveAccount(firstId)
                        // 重新查询未读预警计数
                        const cnt = await GetUnreadAlertCount(firstId)
                        setUnreadAlertCount(Number(cnt) || 0)
                      }
                    } else {
                      message.error(r?.message ?? '生成失败')
                    }
                  } catch (e: any) {
                    message.error(`生成异常: ${e?.message ?? String(e)}`)
                  } finally { setMockGenerating(false) }
                }}
              >🚀 生成模拟数据</Button>
              <Popconfirm
                title="确认清空所有数据？"
                description="此操作将删除所有业务数据（订单、销售、广告、库存等），且不可撤销！"
                okText="确认清空"
                cancelText="取消"
                okButtonProps={{ danger: true }}
                onConfirm={async () => {
                  setMockClearing(true)
                  try {
                    const r: any = await ClearMockData()
                    if (r?.success) {
                      message.success(r.message)
                      // 重新加载：清空后账户和站点都没了，需要同步前端状态
                      const [newAccounts, newMps] = await Promise.all([GetAccounts(), GetMarketplaces()])
                      setMarketplaces(newMps ?? [])
                      setAccounts(newAccounts ?? [])
                      // 清空后预警应为零
                      setUnreadAlertCount(0)
                      // 无账户可选，重置为 null 清除残留
                      if (!newAccounts?.length) {
                        setActiveAccount(0) // 触发 store 清空
                      } else {
                        setActiveAccount(newAccounts[0].id)
                      }
                    } else {
                      message.error(r?.message ?? '清空失败')
                    }
                  } catch (e: any) {
                    message.error(`清空异常: ${e?.message ?? String(e)}`)
                  } finally { setMockClearing(false) }
                }}
              >
                <Button
                  danger
                  loading={mockClearing}
                >🗑️ 清空所有数据</Button>
              </Popconfirm>
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'history',
      label: <span><HistoryOutlined /> 同步历史</span>,
      children: activeAccountId
        ? <div className="settings-block"><div className="settings-block-body">
          <SyncHistory accountId={activeAccountId} />
        </div></div>
        : <Alert message="请先在右上角选择店铺账户" type="info" showIcon />,
    },
  ]

  return (
    <div>
      <div className="page-header">
        <div className="page-title">⚙️ 系统设置</div>
        <div className="page-subtitle">管理店铺账户、API 凭证、成本数据和同步配置</div>
      </div>

      <div className="voyage-card" style={{ padding: '0 20px' }}>
        <Tabs items={tabItems} size="large" tabBarStyle={{ marginBottom: 24 }} />
      </div>
    </div>
  )
}
