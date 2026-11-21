import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { Icon } from '../icons'
import { WeaveStrip } from '../weave'
import { ConfirmModal, Empty, notify } from '../ui'
import type { PageProps } from '../App'

const groupByLabel: Record<string, string> = {
  host: '按机器分组',
  type: '按协议分组',
  region: '按地区分组',
  flat: '不分组',
}

export function Bundles({ state, refresh, navigate }: PageProps) {
  const { t } = useTranslation()
  const [removing, setRemoving] = useState<{ id: string; name: string; subs: number } | null>(null)
  const [busy, setBusy] = useState(false)

  async function confirmRemove() {
    if (!removing) return
    setBusy(true)
    try {
      await api.deleteBundle(removing.id)
      setRemoving(null)
      notify(t('编排已删除'), 'ok')
      await refresh()
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('{{n}} 条编排', { n: state.bundles.length })}</h1>
          <p className="sub">{t('从所有机器里挑节点，决定怎么分组，输出成一条订阅。')}</p>
        </div>
        <div className="actions">
          <button className="primary" onClick={() => navigate('/bundles/new')}>
            <Icon name="plus" size={15} />
            <span>{t('新建编排')}</span>
          </button>
        </div>
      </div>

      <section className="panel panel-flush">
        {state.bundles.length === 0 ? (
          <div className="panel-pad">
            <Empty
              icon="layers"
              title={t('还没有编排')}
              body={t('编排是节点选集：选择节点、分组方式，然后生成订阅链接。')}
              action={
                <button className="primary" onClick={() => navigate('/bundles/new')}>
                  <Icon name="plus" size={15} />
                  <span>{t('新建编排')}</span>
                </button>
              }
            />
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t('编排')}</th>
                  <th>{t('构成')}</th>
                  <th>{t('分组')}</th>
                  <th>{t('规则')}</th>
                  <th className="num">{t('节点')}</th>
                  <th className="num">{t('订阅')}</th>
                  <th aria-label={t('操作')} />
                </tr>
              </thead>
              <tbody>
                {state.bundles.map((b) => {
                  const subs = state.subscriptions.filter((s) => s.bundleId === b.id).length
                  const perHost = new Map<string, number>()
                  for (const m of b.members) perHost.set(m.hostId, (perHost.get(m.hostId) ?? 0) + 1)
                  const segments = [...perHost.entries()]
                    .map(([id, count]) => ({
                      label: state.hosts.find((h) => h.id === id)?.name ?? id,
                      count,
                    }))
                    .sort((a, c) => c.count - a.count)
                  const ruleName = state.ruleSets.find((r) => r.id === b.ruleSetId)?.name
                  return (
                    <tr
                      key={b.id}
                      className="click"
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
                      <td>
                        <span className="cell-main">{b.name}</span>
                        <span className="cell-sub mono">/{b.slug}</span>
                      </td>
                      <td>
                        <WeaveStrip segments={segments} />
                      </td>
                      <td className="small muted">
                        <span>{t(groupByLabel[b.groupBy] ?? b.groupBy)}</span>
                        <span className="badge mono badge-space">{b.groupType}</span>
                      </td>
                      <td className="small muted">
                        {ruleName ? <span className="badge">{ruleName}</span> : '—'}
                      </td>
                      <td className="num">{b.members.length}</td>
                      <td className="num">{subs}</td>
                      <td className="num">
                        <span className="row-actions row-actions-end">
                          <button
                            className="icon small icon-btn"
                            title={t('创建订阅')}
                            aria-label={t('创建订阅')}
                            onClick={(e) => {
                              e.stopPropagation()
                              navigate(`/subscriptions?create=${encodeURIComponent(b.id)}`)
                            }}
                          >
                            <Icon name="link" size={14} />
                          </button>
                          <button
                            className="icon small icon-btn icon-danger"
                            title={t('删除编排')}
                            aria-label={t('删除编排')}
                            onClick={(e) => {
                              e.stopPropagation()
                              setRemoving({ id: b.id, name: b.name, subs })
                            }}
                          >
                            <Icon name="trash" size={14} />
                          </button>
                        </span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <details className="disclosure panel" open={state.bundles.length === 0}>
        <summary>
          <span className="disclosure-title">
            <Icon name="layers" size={15} />
            <span>{t('分组方式怎么选')}</span>
          </span>
          <span className="disclosure-meta small muted">{t('一条编排可以被多条订阅引用')}</span>
          <Icon name="chevronDown" size={15} className="disclosure-chevron" />
        </summary>
        <div className="disclosure-body">
          <div className="grid two">
            <ul className="facts">
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
            <p>
              {t('组行为取')} <code className="mono">url-test</code> {t('自动测速、')}{' '}
              <code className="mono">select</code> {t('手动切换。改完保存，客户端下次拉取即生效。')}
            </p>
          </div>
        </div>
      </details>

      {removing && (
        <ConfirmModal
          title={t('确认删除')}
          message={t('删除编排「{{name}}」？{{extra}}', {
            name: removing.name,
            extra: removing.subs
              ? t('同时会保留 {{n}} 条订阅链接，但它们会拉不到配置。', { n: removing.subs })
              : '',
          })}
          confirmLabel={t('删除')}
          busy={busy}
          onConfirm={confirmRemove}
          onClose={() => setRemoving(null)}
        />
      )}
    </>
  )
}
