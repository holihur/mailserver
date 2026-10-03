import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { Plus, Trash2, UserCog, Ban, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react'

export default function AdminUsers() {
  const [users, setUsers] = useState([])
  const [form, setForm] = useState({ email: '', name: '', password: '' })
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    try { setUsers(await api.adminUsers()) } catch (e) { setMsg(e.message) }
  }
  useEffect(() => { load() }, [])

  async function create(e) {
    e.preventDefault()
    setBusy(true); setMsg('')
    try {
      await api.adminUserCreate(form)
      setForm({ email: '', name: '', password: '' })
      await load()
      setMsg('账号已创建')
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  async function patch(u, body) {
    try { await api.adminUserPatch(u.id, body); await load() } catch (e) { alert(e.message) }
  }
  async function resetPwd(u) {
    const p = prompt(`为 ${u.email} 设置新密码（至少 6 位）`)
    if (p) await patch(u, { password: p })
  }
  async function del(u) {
    if (confirm(`删除 ${u.email} 及其全部邮件？`)) {
      try { await api.adminUserDelete(u.id); await load() } catch (e) { alert(e.message) }
    }
  }

  return (
    <AdminShell title="用户账号" desc="为你的域名创建邮箱账号。创建前请先在「域名服务商」托管对应域名。">
      <Card className="p-4">
        <b className="text-sm">新建邮箱</b>
        <form onSubmit={create} className="grid sm:grid-cols-4 gap-2 mt-3">
          <Input placeholder="someone@example.com" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} required />
          <Input placeholder="姓名（可选）" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          <Input type="password" placeholder="密码（≥6 位）" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required />
          <Button size="sm" disabled={busy}>{busy ? <Loader2 className="animate-spin" /> : <Plus />}创建</Button>
        </form>
        {msg && <p className="text-xs mt-2 flex items-center gap-1">
          {msg.includes('已创建') ? <CheckCircle2 size={12} className="text-green-600" /> : <AlertCircle size={12} className="text-yellow-600" />}{msg}
        </p>}
      </Card>

      <Card className="p-4 overflow-x-auto">
        <b className="text-sm">全部账号（{users.length}）</b>
        <table className="w-full text-sm mt-3">
          <thead><tr className="text-left text-muted-foreground text-xs"><th className="py-1">邮箱</th><th>姓名</th><th>邮件</th><th>状态</th><th></th></tr></thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id} className="border-t border-border">
                <td className="py-2 pr-2 font-mono text-xs">{u.email}{u.admin && <Badge className="ml-2 text-blue-600">管理员</Badge>}</td>
                <td className="pr-2">{u.name}</td>
                <td className="pr-2 text-muted-foreground">{u.mail_count ?? 0}</td>
                <td className="pr-2">{u.disabled ? <Badge className="text-red-500">已禁用</Badge> : <Badge className="text-green-600">正常</Badge>}</td>
                <td className="whitespace-nowrap text-right">
                  <Button variant="ghost" size="icon" title={u.disabled ? '启用' : '禁用'} onClick={() => patch(u, { disabled: !u.disabled })}>
                    {u.disabled ? <CheckCircle2 /> : <Ban />}
                  </Button>
                  <Button variant="ghost" size="icon" title="重置密码" onClick={() => resetPwd(u)}><UserCog /></Button>
                  <Button variant="ghost" size="icon" title="删除" onClick={() => del(u)}><Trash2 /></Button>
                </td>
              </tr>
            ))}
            {users.length === 0 && <tr><td colSpan={5} className="py-4 text-center text-muted-foreground text-sm">还没有账号</td></tr>}
          </tbody>
        </table>
      </Card>
    </AdminShell>
  )
}
