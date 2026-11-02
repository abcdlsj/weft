import { ago, api } from '../api'
import { WeaveStrip } from '../weave'
import { Icon } from '../icons'
import { Empty, notify } from '../ui'
import type { PageProps } from '../App'

const groupByLabel: Record<string, string> = {
  host: '按机器分组',
  type: '按协议分组',
  flat: '不分组',
}

export function Bundles({ state, refresh, navigate }: PageProps) {
  async function remove(id: string, name: string, subs: number) {
    const extra = subs ? `\n同时会保留 ${subs} 条订阅链接，但它们会拉不到配置。` : ''
    if (!confirm(`删除编排「${name}」？${extra}`)) return
    await api.deleteBundle(id)
    notify('编排已删除')
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>编排</h1>
          <p className="sub">从所有机器里挑节点，决定怎么分组，输出成一条订阅。</p>
        </div>
        <div className="title-actions">
          <button className="btn btn--primary" onClick={() => navigate('/bundles/new')}>
            <Icon name="plus" />
            新建编排
          </button>
        </div>
      </div>

      <section className="panel">
        {state.bundles.length === 0 ? (
          <Empty
            title="还没有编排"
            body="编排是节点选集：选择节点、分组方式，然后生成订阅链接。"
            action={
              <button className="btn btn--primary" onClick={() => navigate('/bundles/new')}>
                新建编排
              </button>
            }
          />
        ) : (
          <table className="table table--click">
            <thead>
              <tr>
                <th>编排</th>
                <th>构成</th>
                <th>分组</th>
                <th>协议</th>
                <th className="num">节点</th>
                <th className="num">订阅</th>
                <th className="num">创建</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {state.bundles.map((b) => {
                const subs = state.subscriptions.filter((s) => s.bundleId === b.id).length
                const types = new Set<string>()
                const perHost = new Map<string, number>()
                for (const m of b.members) perHost.set(m.hostId, (perHost.get(m.hostId) ?? 0) + 1)
                const segments = [...perHost.entries()]
                  .map(([id, count]) => ({
                    label: state.hosts.find((h) => h.id === id)?.name ?? id,
                    count,
                  }))
                  .sort((a, c) => c.count - a.count)
                for (const n of state.nodes) {
                  if (b.members.some((m) => m.hostId === n.hostId && m.key === n.key)) types.add(n.type)
                }
                return (
                  <tr
                    key={b.id}
                    tabIndex={0}
                    role="button"
                    aria-label={`编辑 ${b.name}`}
                    onClick={() => navigate(`/bundles/${b.id}`)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        navigate(`/bundles/${b.id}`)
                      }
                    }}
                  >
                    <td data-label="编排">
                      <span className="cell-main">{b.name}</span>
                      <span className="cell-sub mono">/{b.slug}</span>
                    </td>
                    <td data-label="构成">
                      <WeaveStrip segments={segments} />
                    </td>
                    <td data-label="分组" className="muted">
                      {groupByLabel[b.groupBy] ?? b.groupBy} · {b.groupType}
                    </td>
                    <td data-label="协议">
                      <span className="chips">
                        {[...types].sort().map((t) => (
                          <span key={t} className="chip mono">
                            {t}
                          </span>
                        ))}
                        {types.size === 0 && <span className="muted">—</span>}
                      </span>
                    </td>
                    <td className="num" data-label="节点">
                      {b.members.length}
                    </td>
                    <td className="num" data-label="订阅">
                      {subs}
                    </td>
                    <td className="num muted" data-label="创建">
                      {ago(b.createdAt)}
                    </td>
                    <td className="cell-action">
                      <button
                        className="icon-btn"
                        aria-label="删除编排"
                        onClick={(e) => {
                          e.stopPropagation()
                          void remove(b.id, b.name, subs)
                        }}
                      >
                        <Icon name="trash" />
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel panel--note">
        <header>
          <h2>分组方式怎么选</h2>
          <span className="meta">一条编排可以被多条订阅引用</span>
        </header>
        <div className="panel-split">
          <ul className="facts facts--defs">
            <li>
              <span>按机器</span>
              <span>每台一个组，切换最直观；单机下线只影响一个组。</span>
            </li>
            <li>
              <span>按协议</span>
              <span>同协议聚合，便于比较传输差异。</span>
            </li>
            <li>
              <span>不分组</span>
              <span>节点平铺，交给客户端分组。</span>
            </li>
          </ul>
          <p className="prose">
            组行为取 <span className="mono">url-test</span> 自动测速、<span className="mono">select</span>{' '}
            手动切换。改完保存，客户端下次拉取即生效。
          </p>
        </div>
      </section>
    </>
  )
}
