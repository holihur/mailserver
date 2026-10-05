import { useEffect, useState } from 'react'
import { api } from '../api/client'
import AdminShell from '../components/AdminShell'
import { Card, Badge } from '../components/ui/controls'
import { SkeletonRows } from '../components/Skeleton'
import { EmptyState } from '../components/EmptyState'
import { useI18n } from '../lib/i18n'
import { History } from 'lucide-react'

// 管理员操作审计日志。
export default function AdminAudit() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  useEffect(() => { api.auditLogs().then(setList).catch(() => {}).finally(() => setLoading(false)) }, [])

  return (
    <AdminShell title={t('admin.audit')} desc={t('admin.auditDesc')}>
      <Card className="p-4 overflow-x-auto">
        {loading && <SkeletonRows rows={4} />}
        {!loading && list.length === 0 && <EmptyState icon={History} title={t('admin.auditEmpty')} />}
        {!loading && list.length > 0 && (
          <table className="w-full text-sm min-w-[680px]">
            <thead><tr className="text-left text-xs text-muted-foreground">
              <th scope="col" className="py-1">{t('admin.auditTime')}</th>
              <th scope="col">{t('admin.auditActor')}</th>
              <th scope="col">{t('admin.auditAction')}</th>
              <th scope="col">{t('admin.auditTarget')}</th>
              <th scope="col">{t('admin.auditIP')}</th>
              <th scope="col">{t('admin.auditDetail')}</th>
            </tr></thead>
            <tbody>
              {list.map(l => (
                <tr key={l.id} className="border-t border-border align-top">
                  <td className="py-2 pr-2 text-xs whitespace-nowrap">{new Date(l.created_at).toLocaleString()}</td>
                  <td className="pr-2 text-xs">{l.actor_email}</td>
                  <td className="pr-2"><Badge>{l.action}</Badge></td>
                  <td className="pr-2 text-xs font-mono break-all">{l.target}</td>
                  <td className="pr-2 text-xs whitespace-nowrap">{l.ip}</td>
                  <td className="text-xs text-muted-foreground break-all">{l.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </AdminShell>
  )
}
