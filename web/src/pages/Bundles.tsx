import { useTranslation } from 'react-i18next'
import { ago, api } from '../api'
import { WeaveStrip } from '../weave'
import { Icon } from '../icons'
import { Empty, notify } from '../ui'
import type { PageProps } from '../App'

const groupByLabel: Record<string, string> = {
  host: '按机器分组',
  type: '按协议分组',
  region: '按地区分组',
  flat: '不分组',
}

export function Bundles({ state, refresh, navigate }: PageProps) {
  const { t } = useTranslation()
  async function remove(id: string, name: string, subs: number) {
    const extra = subs
      ? t('同时会保留 {{n}} 条订阅链接，但它们会拉不到配置。', { n: subs })
      : ''
    if (!confirm(t('删除编排「{{name}}」？{{extra}}', { name, extra }))) return
    await api.deleteBundle(id)
    notify(t('编排已删除'))
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>{t('编排')}</h1>
          <p className="sub">{t('从所有机器里挑节点，决定怎么分组，输出成一条订阅。')}</p>
        </div>
        <div className="title-actions">
          <button className="btn btn--primary" onClick={() => navigate('/bundles/new')}>
            <Icon name="plus" />
            {t('新建编排')}
          </button>
        </div>
      </div>

      <section className="panel">
        {state.bundles.length === 0 ? (
          <Empty
            title={t('还没有编排')}
            body={t('编排是节点选集：选择节点、分组方式，然后生成订阅链接。')}
            action={
              <button className="btn btn--primary" onClick={() => navigate('/bundles/new')}>
                {t('新建编排')}
              </button>
            }
          />
        ) : (
          <table className="table table--click">
            <thead>
              <tr>
                <th>{t('编排')}</th>
                <th>{t('构成')}</th>
                <th>{t('分组')}</th>
                <th>{t('规则')}</th>
                <th>{t('协议')}</th>
                <th className="num">{t('节点')}</th>
                <th className="num">{t('订阅')}</th>
                <th className="num">{t('创建')}</th>
                <th aria-label={t('操作')} />
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
                    aria-label={t('编辑 {{name}}', { name: b.name })}
                    onClick={() => navigate(`/bundles/${b.id}`)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        navigate(`/bundles/${b.id}`)
                      }
                    }}
                  >
                    <td data-label={t('编排')}>
                      <span className="cell-main">{b.name}</span>
                      <span className="cell-sub mono">/{b.slug}</span>
                    </td>
                    <td data-label={t('构成')}>
                      <WeaveStrip segments={segments} />
                    </td>
                    <td data-label={t('分组')} className="muted">
                      {t(groupByLabel[b.groupBy] ?? b.groupBy)} · {b.groupType}
                    </td>
                    <td data-label={t('规则')} className="muted">
                      {state.ruleSets.find((r) => r.id === b.ruleSetId)?.name ?? '—'}
                    </td>
                    <td data-label={t('协议')}>
                      <span className="chips">
                        {[...types].sort().map((t) => (
                          <span key={t} className="chip mono">
                            {t}
                          </span>
                        ))}
                        {types.size === 0 && <span className="muted">—</span>}
                      </span>
                    </td>
                    <td className="num" data-label={t('节点')}>
                      {b.members.length}
                    </td>
                    <td className="num" data-label={t('订阅')}>
                      {subs}
                    </td>
                    <td className="num muted" data-label={t('创建')}>
                      {ago(b.createdAt)}
                    </td>
                    <td className="cell-action">
                      <button
                        className="icon-btn"
                        aria-label={t('删除编排')}
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
          <h2>{t('分组方式怎么选')}</h2>
          <span className="meta">{t('一条编排可以被多条订阅引用')}</span>
        </header>
        <div className="panel-split">
          <ul className="facts facts--defs">
            <li>
              <span>{t('按机器')}</span>
              <span>{t('每台一个组，切换最直观；单机下线只影响一个组。')}</span>
            </li>
            <li>
              <span>{t('按协议')}</span>
              <span>{t('同协议聚合，便于比较传输差异。')}</span>
            </li>
            <li>
              <span>{t('不分组')}</span>
              <span>{t('节点平铺，交给客户端分组。')}</span>
            </li>
          </ul>
          <p className="prose">
            {t('组行为取')} <span className="mono">url-test</span> {t('自动测速、')}{' '}
            <span className="mono">select</span> {t('手动切换。改完保存，客户端下次拉取即生效。')}
          </p>
        </div>
      </section>
    </>
  )
}
