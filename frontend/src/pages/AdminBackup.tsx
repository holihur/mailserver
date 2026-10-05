import { useEffect, useState } from 'react'
import { api } from '../api/client'
import AdminShell from '../components/AdminShell'
import { Card, Badge, Button, Input, Label } from '../components/ui/controls'
import { SkeletonRows } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { toast, confirmAsync } from '../lib/ui'
import { HardDriveDownload, Plus, RotateCcw, Trash2, RefreshCw } from 'lucide-react'

function fmtSize(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${u[i]}`
}

// 管理后台「备份」：定时策略、立即备份、列表 / 下载 / 删除 / 恢复。
export default function AdminBackup() {
  const { t } = useI18n()
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [dir, setDir] = useState('')
  const [intervalH, setIntervalH] = useState('24')
  const [keep, setKeep] = useState('7')

  const load = () =>
    api.backups().then(d => {
      setData(d)
      setDir(d.dir || '')
      setIntervalH(String(d.interval_hours ?? 24))
      setKeep(String(d.keep ?? 7))
    }).catch(() => {}).finally(() => setLoading(false))
  useEffect(() => { load() }, [])

  async function savePolicy() {
    try {
      await api.settingsPatch({ backup_dir: dir, backup_interval_hours: intervalH, backup_keep: keep })
      toast(t('backup.saved'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message, { type: 'error' }) }
  }

  async function backupNow() {
    setBusy(true)
    try {
      await api.backupCreate()
      toast(t('backup.created'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message, { type: 'error' }) } finally { setBusy(false) }
  }

  async function del(name: string) {
    if (!(await confirmAsync(`${t('backup.delete')}: ${name}?`))) return
    try {
      await api.backupDelete(name)
      toast(t('backup.deleted'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message, { type: 'error' }) }
  }

  async function restore(name: string) {
    if (!(await confirmAsync(t('backup.confirmRestore')))) return
    setBusy(true)
    try {
      await api.backupRestore(name)
      toast(t('backup.restored'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message, { type: 'error' }) } finally { setBusy(false) }
  }

  const list: any[] = data?.backups || []

  return (
    <AdminShell title={t('backup.title')} desc={t('backup.desc')}>
      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2 text-sm font-medium">{t('backup.policy')}</div>
        <div className="grid sm:grid-cols-3 gap-3">
          <div>
            <Label>{t('backup.dir')}</Label>
            <Input value={dir} onChange={e => setDir(e.target.value)} placeholder="/data/backups" />
          </div>
          <div>
            <Label>{t('backup.interval')}</Label>
            <Input type="number" min={1} max={8760} value={intervalH} onChange={e => setIntervalH(e.target.value)} />
          </div>
          <div>
            <Label>{t('backup.keep')}</Label>
            <Input type="number" min={1} max={3650} value={keep} onChange={e => setKeep(e.target.value)} />
          </div>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <Button onClick={savePolicy}>{t('backup.save')}</Button>
          <Button variant="outline" onClick={backupNow} disabled={busy}>
            <Plus size={14} />{t('backup.now')}
          </Button>
          <Button variant="ghost" size="sm" onClick={load}><RefreshCw size={14} /></Button>
          <div className="flex-1" />
          <Badge title={data?.encrypted ? t('backup.encryptedOn') : t('backup.encryptedOff')}>
            {t('backup.encrypted')}: {data?.encrypted ? 'AES-256-GCM' : 'off'}
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">{t('backup.restoreHint')}</p>
      </Card>

      <Card className="p-4 overflow-x-auto">
        <div className="text-sm font-medium mb-2">{t('backup.list')}</div>
        {loading && <SkeletonRows rows={3} />}
        {!loading && list.length === 0 && <p className="text-sm text-muted-foreground">{t('backup.empty')}</p>}
        {!loading && list.length > 0 && (
          <table className="w-full text-sm min-w-[720px]">
            <thead><tr className="text-left text-xs text-muted-foreground">
              <th className="py-1">{t('backup.name')}</th>
              <th>{t('backup.size')}</th>
              <th>{t('backup.time')}</th>
              <th>{t('backup.sha')}</th>
              <th className="text-right">{t('backup.restore')}</th>
            </tr></thead>
            <tbody>
              {list.map(b => (
                <tr key={b.name} className="border-t border-border align-top">
                  <td className="py-2 pr-2 text-xs font-mono break-all">{b.name}</td>
                  <td className="pr-2 text-xs whitespace-nowrap">{fmtSize(b.size)}</td>
                  <td className="pr-2 text-xs whitespace-nowrap">{new Date(b.mod_time).toLocaleString()}</td>
                  <td className="pr-2 text-xs font-mono text-muted-foreground">{String(b.sha256 || '').slice(0, 12)}</td>
                  <td className="text-right whitespace-nowrap">
                    <Button variant="ghost" size="sm" title={t('backup.download')} onClick={() => api.backupDownload(b.name).catch(() => toast(t('backup.download') + ' ✗', { type: 'error' }))}>
                      <HardDriveDownload size={14} />
                    </Button>
                    <Button variant="ghost" size="sm" title={t('backup.restore')} onClick={() => restore(b.name)} disabled={busy}>
                      <RotateCcw size={14} />
                    </Button>
                    <Button variant="ghost" size="sm" title={t('backup.delete')} onClick={() => del(b.name)} disabled={busy}>
                      <Trash2 size={14} className="text-destructive" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </AdminShell>
  )
}
