import { create } from 'zustand'

/* =====================================================
   全局应用状态（Zustand Store）
   ===================================================== */

export interface Account {
  id: number
  name: string
  sellerId: string
  region: string
  isActive: boolean
  marketplaces: string[]
}

export interface MarketplaceInfo {
  marketplaceId: string
  countryCode: string
  name: string
  currencyCode: string
  region: string
  timezone: string
}

export type SyncStatus = 'idle' | 'syncing' | 'success' | 'error'

export interface SyncState {
  status: SyncStatus
  lastSyncTime: string | null
  message: string
  progress: number // 0-100
}

export interface AppState {
  // 账户管理
  accounts: Account[]
  activeAccountId: number | null
  activeMarketplaceId: string | null

  // 站点参考数据
  marketplaces: MarketplaceInfo[]

  // 时区
  beijingTime: string
  storeTime: string
  storeTimezone: string

  // 同步状态
  syncState: SyncState

  // 预警数量（侧边栏角标）
  unreadAlertCount: number

  // 操作方法
  setAccounts: (accounts: Account[]) => void
  setActiveAccount: (accountId: number) => void
  setActiveMarketplace: (marketplaceId: string) => void
  setMarketplaces: (marketplaces: MarketplaceInfo[]) => void
  setTimes: (beijing: string, store: string, tz: string) => void
  setSyncState: (state: Partial<SyncState>) => void
  setUnreadAlertCount: (count: number) => void
}

export const useAppStore = create<AppState>((set, get) => ({
  accounts: [],
  activeAccountId: null,
  activeMarketplaceId: null,
  marketplaces: [],
  beijingTime: '--:--:--',
  storeTime: '--:--:--',
  storeTimezone: '',
  syncState: {
    status: 'idle',
    lastSyncTime: null,
    message: '等待同步',
    progress: 0,
  },
  unreadAlertCount: 0,

  setAccounts: (accounts) => set({ accounts }),

  setActiveAccount: (accountId) => {
    const { accounts, marketplaces } = get()
    const account = accounts.find(a => a.id === accountId)

    // 如果账户不存在（如清空数据后），重置为 null
    if (!account) {
      set({ activeAccountId: null, activeMarketplaceId: null })
      return
    }

    let defaultMarketplace: string | null = null
    if (account.marketplaces?.length) {
      defaultMarketplace = account.marketplaces[0]
    }

    set({
      activeAccountId: accountId,
      activeMarketplaceId: defaultMarketplace,
    })
  },

  setActiveMarketplace: (marketplaceId) => {
    const { marketplaces } = get()
    const mp = marketplaces.find(m => m.marketplaceId === marketplaceId)
    set({
      activeMarketplaceId: marketplaceId,
      storeTimezone: mp?.timezone ?? '',
    })
  },

  setMarketplaces: (marketplaces) => set({ marketplaces }),

  setTimes: (beijing, store, tz) => set({
    beijingTime: beijing,
    storeTime: store,
    storeTimezone: tz,
  }),

  setSyncState: (state) => set(prev => ({
    syncState: { ...prev.syncState, ...state },
  })),

  setUnreadAlertCount: (count) => set({ unreadAlertCount: count }),
}))
