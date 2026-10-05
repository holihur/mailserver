import { useEffect, useState } from 'react'
import { toast, confirmDestructive, promptAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, KeyRound, Loader2, AlertCircle, CheckCircle2, UserCog, HardDrive, Send } from 'lucide-react'

export default function AdminUsers() {
  const { t } = useI18n()
  const [users, setUsers] = useState([])
  const [form, setForm] = useState({ email: '', name: '', password: '', quota_mb: 0, send_daily_limit: 0, send_per_minute: 0 })
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')

  async function load() {
    try { setUsers(await api.adminUsers()) } catch (e) { setMsg(e.message) }
  }
  useEffect(() => { load() }, [])

  async function create(e) {
    e.preventDefault()
    setBusy('create'); setMsg('')
    try {
      await api.adminUserCreate(form)
      setForm({ email: '', name: '', password: '', quota_mb: 0, send_daily_limit: 0, send_per_minute: 0 })
      setMsg(t('users.create'))
      load()
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  async function resetPass(u) {
    const pw = await promptAsync(`${t('users.changePass')}: ${u.email}`, { password: true, placeholder: '≥6' })
    if (!pw) return
    try { await api.adminUserPatch(u.id, { password: pw }); setMsg(t('users.changePass')) } catch (e) { setMsg(e.message) }
  }

  async function setQuota(u) {
    const v = await promptAsync(`${t('users.setQuota')}: ${u.email} (MB, 0=${t('users.unlimited')})`, { defaultValue: String(u.quota_mb || 0), inputType: 'number', placeholder: 'MB' })
    if (v === null) return
    const mb = parseInt(v, 10)
    if (isNaN(mb) || mb < 0) return
    try { await api.adminUserPatch(u.id, { quota_mb: mb }); load() } catch (e) { setMsg(e.message) }
  }

  // 每用户发信配额 / 速率（0 = 用全局默认）
  async function setSendLimit(u) {
    const d = await promptAsync(`${t('users.sendDaily')}: ${u.email} (0=${t('users.default')})`, { defaultValue: String(u.send_daily_limit || 0), inputType: 'number' })
    if (d === null) return
    const m = await promptAsync(`${t('users.sendMinute')}: ${u.email} (0=${t('users.default')})`, { defaultValue: String(u.send_per_minute || 0), inputType: 'number' })
    if (m === null) return
    try { await api.adminUserPatch(u.id, { send_daily_limit: parseInt(d, 10) || 0, send_per_minute: parseInt(m, 10) || 0 }); load() } catch (e) { setMsg(e.message) }
  }

  async function toggle(u, key) {
    try { await api.adminUserPatch(u.id, { [key]: !u[key] }); load() } catch (e) { setMsg(e.message) }
  }

  async function del(u) {
    if (!await confirmDestructive(`${t('common.delete')} ${u.email}?`)) return
    try { await api.adminUserDelete(u.id); setMsg(t('common.delete')); load() } catch (e) { setMsg(e.message) }
  }

  return (
    <AdminShell title={t('users.title')} desc={t('users.desc')}>
      <Card className="p-4">
        <div className="flex items-center gap-2 mb-3"><UserCog size={16} /><b className="text-sm">{t('users.create')}</b></div>
        <form onSubmit={create} className="grid sm:grid-cols-3 lg:grid-cols-6 gap-3 items-end">
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.email')}</span>
            <Input type="email" placeholder="user@example.com" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} required />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.name')}</span>
            <Input placeholder="Zhang San" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.password')}</span>
            <Input type="text" placeholder="≥6" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.quota')}</span>
            <Input type="number" placeholder="MB" value={form.quota_mb} onChange={e => setForm({ ...form, quota_mb: +e.target.value })} />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.sendDaily')}</span>
            <Input type="number" placeholder={t('users.default')} value={form.send_daily_limit} onChange={e => setForm({ ...form, send_daily_limit: +e.target.value })} />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.sendMinute')}</span>
            <Input type="number" placeholder={t('users.default')} value={form.send_per_minute} onChange={e => setForm({ ...form, send_per_minute: +e.target.value })} />
          </label>
          <Button disabled={busy === 'create'}>
            {busy === 'create' ? <Loader2 className="animate-spin" /> : <Plus />}{t('common.create')}
          </Button>
        </form>
        <p className="text-xs text-muted-foreground mt-2">{t('users.hostHint')}</p>
      </Card>

      {msg && <p className="text-sm flex items-center gap-1 text-muted-foreground"><AlertCircle size={14} />{msg}</p>}

      <Card className="p-4">
        <b className="text-sm">{t('users.all', { n: users.length })}</b>
        <div className="hidden sm:block overflow-x-auto">
        <table className="w-full text-sm mt-3 min-w-[560px]">
          <thead><tr className="text-left text-xs text-muted-foreground">
            <th scope="col" className="py-1">{t('users.email')}</th><th scope="col">{t('users.name')}</th><th scope="col">{t('users.mails')}</th><th scope="col">{t('users.sentToday')}</th><th scope="col">{t('users.quota')}</th><th scope="col">{t('users.status')}</th><th scope="col" className="text-right">{t('users.actions')}</th>
          </tr></thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id} className="border-t border-border">
                <td className="py-2 pr-2">{u.email} {u.admin && <Badge>{t('users.adminBadge')}</Badge>}</td>
                <td className="pr-2">{u.name}</td>
                <td className="pr-2">{u.mail_count}</td>
                <td className="pr-2 text-xs">{u.sent_today ?? 0}</td>
                <td className="pr-2 text-xs">{u.quota_mb > 0 ? `${u.quota_mb} MB` : t('users.unlimited')}</td>
                <td className="pr-2">{u.disabled ? <span className="text-destructive text-xs">{t('users.disabled')}</span> : <span className="text-success text-xs">{t('users.normal')}</span>}</td>
                <td className="text-right whitespace-nowrap">
                  <Button variant="ghost" size="sm" onClick={() => setQuota(u)}><HardDrive />{t('users.setQuota')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => setSendLimit(u)}><Send />{t('users.sendLimit')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => resetPass(u)}><KeyRound />{t('users.changePass')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => toggle(u, 'admin')}>{u.admin ? t('users.unsetAdmin') : t('users.setAdmin')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => toggle(u, 'disabled')}>{u.disabled ? t('users.enable') : t('users.disable')}</Button>
                  <Button variant="ghost" size="icon" onClick={() => del(u)} aria-label={t('common.delete')}><Trash2 /></Button>
                </td>
              </tr>
            ))}
            {users.length === 0 && <tr><td colSpan={7} className="py-4 text-center text-muted-foreground">{t('users.none')}</td></tr>}
          </tbody>
        </table>
        </div>
        <div className="sm:hidden space-y-2 mt-3">
          {users.map(u => (
            <div key={u.id} className="rounded-md border border-border p-3 space-y-1 text-sm">
              <div className="flex items-center gap-2">
                <span className="font-medium truncate">{u.email}</span>
                {u.admin && <Badge>{t('users.adminBadge')}</Badge>}
                {u.disabled ? <span className="ml-auto text-destructive text-xs">{t('users.disabled')}</span> : <span className="ml-auto text-success text-xs">{t('users.normal')}</span>}
              </div>
              <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                <span>{t('users.name')}: {u.name || '—'}</span>
                <span>{t('users.mails')}: {u.mail_count}</span>
                <span>{t('users.sentToday')}: {u.sent_today ?? 0}</span>
                <span>{t('users.quota')}: {u.quota_mb > 0 ? `${u.quota_mb} MB` : t('users.unlimited')}</span>
              </div>
              <div className="flex flex-wrap gap-1 pt-1">
                <Button variant="ghost" size="sm" onClick={() => setQuota(u)}><HardDrive />{t('users.setQuota')}</Button>
                <Button variant="ghost" size="sm" onClick={() => setSendLimit(u)}><Send />{t('users.sendLimit')}</Button>
                <Button variant="ghost" size="sm" onClick={() => resetPass(u)}><KeyRound />{t('users.changePass')}</Button>
                <Button variant="ghost" size="sm" onClick={() => toggle(u, 'admin')}>{u.admin ? t('users.unsetAdmin') : t('users.setAdmin')}</Button>
                <Button variant="ghost" size="sm" onClick={() => toggle(u, 'disabled')}>{u.disabled ? t('users.enable') : t('users.disable')}</Button>
                <Button variant="ghost" size="icon" onClick={() => del(u)} aria-label={t('common.delete')}><Trash2 /></Button>
              </div>
            </div>
          ))}
          {users.length === 0 && <p className="py-4 text-center text-muted-foreground text-sm">{t('users.none')}</p>}
        </div>
      </Card>
    </AdminShell>
  )
}
