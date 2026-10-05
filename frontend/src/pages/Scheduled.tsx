import { useEffect, useState } from 'react'
import { toast, confirmDestructive } from '../lib/ui'
import { api } from '../api/client'
import { Button, Card, Badge } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { EmptyState } from '../components/EmptyState'
import { SkeletonRows } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { Clock, Trash2, Pause, Play, RefreshCw, Repeat } from 'lucide-react'

// 定时 / 周期性发送管理页。
export default function Scheduled() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)

  async function load() {
    try { setList(await api.scheduled()) } catch (e: any) { toast(e.message) } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  async function toggle(s: any) {
    try { await api.scheduledPatch(s.id, { enabled: !s.enabled }); load() } catch (e: any) { toast(e.message) }
  }
  async function del(s: any) {
    if (!await confirmDestructive(t('scheduled.confirmDelete'))) return
    try { await api.scheduledDelete(s.id); load() } catch (e: any) { toast(e.message) }
  }

  const repeatLabel = (r: string) =>
    r === 'daily' ? t('mail.repeatDaily') : r === 'weekly' ? t('mail.repeatWeekly') : r === 'monthly' ? t('mail.repeatMonthly') : t('mail.repeatNone')

  return (
    <PageShell title={t('scheduled.title')} icon={Clock}>
      <p className="text-sm text-muted-foreground">{t('scheduled.desc')}</p>
      <div className="flex justify-end">
        <Button variant="outline" size="sm" onClick={load}><RefreshCw />{t('common.refresh')}</Button>
      </div>
      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && list.length === 0 && <EmptyState icon={Clock} title={t('scheduled.empty')} />}
        {!loading && list.map(s => (
          <div key={s.id} className="p-3 flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{s.subject || t('mail.noSubject')}</span>
                {s.repeat ? <Badge><Repeat size={11} />{repeatLabel(s.repeat)}</Badge> : null}
                {s.enabled
                  ? <Badge className="text-success">{t('scheduled.active')}</Badge>
                  : <Badge className="text-muted-foreground">{s.last_sent ? t('scheduled.done') : t('scheduled.paused')}</Badge>}
              </div>
              <div className="text-xs text-muted-foreground truncate">→ {s.to}</div>
              <div className="text-xs text-muted-foreground mt-0.5">
                {t('scheduled.next')}: {s.send_at ? new Date(s.send_at).toLocaleString() : '—'}
                {s.last_sent ? ` · ${t('scheduled.lastSent')}: ${new Date(s.last_sent).toLocaleString()}` : ''}
              </div>
              {s.last_error && <div className="text-xs text-destructive break-words">{s.last_error}</div>}
            </div>
            <Button variant="ghost" size="icon" onClick={() => toggle(s)} aria-label={s.enabled ? t('scheduled.pause') : t('scheduled.resume')}>
              {s.enabled ? <Pause /> : <Play />}
            </Button>
            <Button variant="ghost" size="icon" onClick={() => del(s)} aria-label={t('scheduled.cancel')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </PageShell>
  )
}
