import i18n from './i18n'

export type HostStatus = 'online' | 'stale' | 'offline' | 'never' | 'manual'
export type HostKind = 'agent' | 'static'

export interface Host {
  id: string
  name: string
  token?: string
  kind: HostKind
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
  source: 'agent' | 'config' | 'manual'
  raw?: Record<string, unknown>
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
  ruleSetId: string
  createdAt: number
  members: Member[]
}

export interface RuleSet {
  id: string
  name: string
  description: string
  content?: string
  source: string
  updatedAt: number
  createdAt: number
  ruleCount?: number
  providerCount?: number
  warningCount?: number
  warnings?: string[]
  usedBy?: number
}

export interface RulePreset {
  key: string
  name: string
  description: string
  content: string
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
  accessKey: string
  bearer: string
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
  ruleSets: RuleSet[]
  rulePresets: RulePreset[]
  gfwListUrl: string
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
  csrf?: string
}

export interface RefreshResult {
  content: string
  warnings?: string[]
  source: string
}

export interface RestoreResult {
  ruleSetsCreated: number
  ruleSetsUpdated: number
  bundlesCreated: number
  bundlesUpdated: number
  warnings: string[]
}

let csrfToken = ''

export function setCSRF(token: string) {
  csrfToken = token
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase()
  const headers = new Headers(init?.headers)
  if (init?.body) headers.set('Content-Type', 'application/json')
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const res = await fetch(path, {
    credentials: 'include',
    ...init,
    headers,
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
    const message = data?.error || `${res.status} ${res.statusText}`
    throw new Error(i18n.t(message))
  }
  return data as T
}

export const api = {
  me: () => req<Me>('/api/me'),
  version: () => req<{ version: string }>('/api/version'),
  login: (password: string) =>
    req<{ ok: boolean; csrf?: string }>('/api/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  setup: (password: string) =>
    req<{ ok: boolean; csrf?: string }>('/api/setup', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  logout: () => req<{ ok: boolean }>('/api/logout', { method: 'POST' }),
  state: () => req<FleetState>('/api/state'),
  createHost: (name: string, kind: HostKind = 'agent') =>
    req<Host>('/api/hosts', { method: 'POST', body: JSON.stringify({ name, kind }) }),
  createNode: (hostId: string, body: { links?: string[]; proxy?: Record<string, unknown> }) =>
    req<{ saved: ProxyNode[]; failed: string[] }>(`/api/hosts/${hostId}/nodes`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  updateNode: (hostId: string, key: string, proxy: Record<string, unknown>) =>
    req<ProxyNode>(`/api/hosts/${hostId}/nodes/${key}`, {
      method: 'PUT',
      body: JSON.stringify(proxy),
    }),
  deleteNode: (hostId: string, key: string) =>
    req<{ ok: boolean }>(`/api/hosts/${hostId}/nodes/${key}`, { method: 'DELETE' }),
  parseNode: (link: string) =>
    req<{ proxy: Record<string, unknown>; name: string; type: string }>('/api/nodes/parse', {
      method: 'POST',
      body: JSON.stringify({ link }),
    }),
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
  previewDraft: (body: BundlePayload, target: 'mihomo' | 'text' | 'base64') =>
    req<{ body: string; nodes: number }>(`/api/bundles/preview?target=${target}`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  createSubscription: (body: { bundleId: string; label: string; format: string }) =>
    req<{ subscription: Subscription; url: string }>('/api/subscriptions', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  patchSubscription: (
    id: string,
    body: { label?: string; enabled?: boolean; format?: string; bearer?: string; rotate?: boolean },
  ) =>
    req<{ ok: boolean; url?: string; bearer?: string }>(`/api/subscriptions/${id}`, {
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
  createRuleSet: (body: { name: string; description?: string; content: string; source?: string }) =>
    req<RuleSet>('/api/rulesets', { method: 'POST', body: JSON.stringify(body) }),
  updateRuleSet: (id: string, body: { name: string; description?: string; content: string; source?: string }) =>
    req<RuleSet>(`/api/rulesets/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteRuleSet: (id: string) => req<{ ok: boolean }>(`/api/rulesets/${id}`, { method: 'DELETE' }),
  importRuleSet: (body: { name: string; url: string; description?: string }) =>
    req<RuleSet>('/api/rulesets/import', { method: 'POST', body: JSON.stringify(body) }),
  ruleSet: (id: string) => req<RuleSet>(`/api/rulesets/${id}`),
  refreshRuleSet: (id: string) =>
    req<RefreshResult>(`/api/rulesets/${id}/refresh`, { method: 'POST' }),
  restore: (body: unknown) =>
    req<RestoreResult>('/api/backup', { method: 'POST', body: JSON.stringify(body) }),
}

export interface BundlePayload {
  name?: string
  slug?: string
  groupBy?: string
  groupType?: string
  template?: string
  ruleSetId?: string
  members?: Member[]
}

export function ago(ts: number): string {
  if (!ts) return '—'
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts))
  if (s < 60) return i18n.t('{{n}} 秒前', { n: s })
  if (s < 3600) return i18n.t('{{n}} 分钟前', { n: Math.floor(s / 60) })
  if (s < 86400) return i18n.t('{{n}} 小时前', { n: Math.floor(s / 3600) })
  return i18n.t('{{n}} 天前', { n: Math.floor(s / 86400) })
}

export function clock(ts: number): string {
  const d = new Date(ts * 1000)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function statusLabel(status: HostStatus): string {
  switch (status) {
    case 'online':
      return i18n.t('在线')
    case 'stale':
      return i18n.t('待复核')
    case 'offline':
      return i18n.t('离线')
    case 'manual':
      return i18n.t('手工')
    default:
      return i18n.t('未接入')
  }
}
