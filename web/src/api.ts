export type HostStatus = 'online' | 'stale' | 'offline' | 'never'

export interface Host {
  id: string
  name: string
  token?: string
  controller: string
  agentVersion: string
  mihomoVersion: string
  lastSeen: number
  createdAt: number
  nodeCount: number
  aliveCount: number
  status: HostStatus
}

export interface ProxyNode {
  hostId: string
  hostName: string
  key: string
  name: string
  type: string
  server: string
  port: number
  alive: boolean
  delayMs: number
  updatedAt: number
}

export interface Member {
  hostId: string
  key: string
}

export interface Bundle {
  id: string
  name: string
  slug: string
  groupBy: 'host' | 'type' | 'flat'
  groupType: 'url-test' | 'select' | 'fallback' | 'load-balance'
  template: string
  createdAt: number
  members: Member[]
}

export interface AccessStat {
  device: string
  ua: string
  ip: string
  hits: number
  lastAt: number
}

export interface Subscription {
  id: string
  bundleId: string
  bundleName: string
  label: string
  token: string
  format: 'mihomo' | 'base64' | 'text'
  enabled: boolean
  hits: number
  lastUsedAt: number
  lastUa: string
  createdAt: number
  devices?: AccessStat[]
}

export interface SeriesPoint {
  at: number
  nodes: number
  alive: number
  delay: number
}

export interface Stats {
  hostsTotal: number
  hostsOnline: number
  nodesTotal: number
  nodesAlive: number
  subHits: number
  lastUse: number
}

export interface FleetState {
  hosts: Host[]
  nodes: ProxyNode[]
  bundles: Bundle[]
  subscriptions: Subscription[]
  series: SeriesPoint[]
  publicBase: string
  demo: boolean
  defaultTemplate: string
  builtinTemplate: string
  stats: Stats
}

export interface Me {
  authenticated: boolean
  needsSetup: boolean
  demo: boolean
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    ...init,
  })
  const text = await res.text()
  let data: any = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!res.ok) {
    throw new Error(data?.error || `${res.status} ${res.statusText}`)
  }
  return data as T
}

export const api = {
  me: () => req<Me>('/api/me'),
  login: (password: string) =>
    req<{ ok: boolean }>('/api/login', { method: 'POST', body: JSON.stringify({ password }) }),
  setup: (password: string) =>
    req<{ ok: boolean }>('/api/setup', { method: 'POST', body: JSON.stringify({ password }) }),
  logout: () => req<{ ok: boolean }>('/api/logout', { method: 'POST' }),
  state: () => req<FleetState>('/api/state'),
  createHost: (name: string) =>
    req<Host>('/api/hosts', { method: 'POST', body: JSON.stringify({ name }) }),
  patchHost: (id: string, body: { name?: string; rotate?: boolean }) =>
    req<{ ok: boolean; token?: string }>(`/api/hosts/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteHost: (id: string) => req<{ ok: boolean }>(`/api/hosts/${id}`, { method: 'DELETE' }),
  createBundle: (body: BundlePayload) =>
    req<Bundle>('/api/bundles', { method: 'POST', body: JSON.stringify(body) }),
  updateBundle: (id: string, body: BundlePayload) =>
    req<Bundle>(`/api/bundles/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteBundle: (id: string) => req<{ ok: boolean }>(`/api/bundles/${id}`, { method: 'DELETE' }),
  preview: (id: string, target: 'mihomo' | 'text' | 'base64') =>
    req<{ body: string; nodes: number }>(`/api/bundles/${id}/preview?target=${target}`),
  createSubscription: (body: { bundleId: string; label: string; format: string }) =>
    req<{ subscription: Subscription; url: string }>('/api/subscriptions', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  patchSubscription: (
    id: string,
    body: { label?: string; enabled?: boolean; format?: string; rotate?: boolean },
  ) =>
    req<{ ok: boolean; url?: string }>(`/api/subscriptions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteSubscription: (id: string) =>
    req<{ ok: boolean }>(`/api/subscriptions/${id}`, { method: 'DELETE' }),
  changePassword: (current: string, next: string) =>
    req<{ ok: boolean }>('/api/password', {
      method: 'POST',
      body: JSON.stringify({ current, next }),
    }),
  saveSettings: (body: { defaultTemplate?: string }) =>
    req<{ ok: boolean }>('/api/settings', { method: 'PUT', body: JSON.stringify(body) }),
}

export interface BundlePayload {
  name?: string
  slug?: string
  groupBy?: string
  groupType?: string
  template?: string
  members?: Member[]
}

export function ago(ts: number): string {
  if (!ts) return '—'
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts))
  if (s < 60) return `${s} 秒前`
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  return `${Math.floor(s / 86400)} 天前`
}

export function clock(ts: number): string {
  const d = new Date(ts * 1000)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function statusLabel(status: HostStatus): string {
  switch (status) {
    case 'online':
      return '在线'
    case 'stale':
      return '待复核'
    case 'offline':
      return '离线'
    default:
      return '未接入'
  }
}
