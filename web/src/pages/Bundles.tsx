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
                  <tr key={b.id} onClick={() => navigate(`/bundles/${b.id}`)}>
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
              <span>每台机器一个组，exits 之间切换最直观；机器下线时只影响一个组。</span>
            </li>
            <li>
              <span>按协议</span>
              <span>同协议聚在一起，便于比较传输差异，适合排障时临时用。</span>
            </li>
            <li>
              <span>不分组</span>
              <span>节点平铺，由客户端自己分组；节点很多时列表会很长。</span>
            </li>
          </ul>
          <div className="prose">
            <p>
              组行为决定客户端怎么用这些节点：<span className="mono">url-test</span> 定时测速自动选最快，
              <span className="mono">select</span> 由你手动切换，
              <span className="mono">fallback</span> 只在当前节点失败时换下一个。
            </p>
            <p>
              订阅链接只认编排，换节点不需要重新发给设备：改完保存，客户端下次拉取就会拿到新配置。
            </p>
          </div>
        </div>
      </section>
    </>
  )
}
