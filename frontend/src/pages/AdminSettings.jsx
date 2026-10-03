import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { Save, Wand2, Upload, KeyRound, Server, Globe, Send, ShieldAlert, CheckCircle2, Loader2 } from 'lucide-react'

// 邮件主机设置（小白友好）：域名 / 公网 IP / 发件中继 / DKIM / 管理员
export default function AdminSettings() {
  const [s, setS] = useState(null)
  const [relayPass, setRelayPass] = useState('')
  const [dkim, setDkim] = useState(null)
  const [busy, setBusy] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    try { setS(await api.settingsGet()) } catch (e) { setMsg(e.message) }
    try { setDkim(await api.dkimGet()) } catch {}
  }
  useEffect(() => { load() }, [])

  function field(k, v) { setS(prev => ({ ...prev, [k]: v })) }

  async function save() {
    setBusy('save'); setMsg('')
    try {
      const body = {
        mail_host: s.mail_host || '',
        public_ip: s.public_ip || '',
        admin_emails: s.admin_emails || '',
        relay_host: s.relay_host || '',
        relay_port: s.relay_port || '',
        relay_user: s.relay_user || '',
        relay_from: s.relay_from || '',
      }
      if (relayPass) body.relay_pass = relayPass
      const out = await api.settingsPatch(body)
      setS(out); setRelayPass('')
      setMsg('已保存，配置立即生效 ✓')
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  async function detectIP() {
    try {
      const r = await fetch('https://api.ipify.org')
      const ip = (await r.text()).trim()
      if (/^\d+\.\d+\.\d+\.\d+$/.test(ip)) field('public_ip', ip)
    } catch { setMsg('自动获取公网 IP 失败，请手动填写服务器公网 IP') }
  }

  async function genDKIM() {
    setBusy('dkim'); setMsg('')
    try {
      const r = await api.dkimGenerate({ domain: s?.dkim_domain || '', selector: s?.dkim_selector || 'dkim' })
      setMsg(`已生成 DKIM 公钥：${r.name}（可下发到服务商）`)
      await load()
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  return (
    <AdminShell title="邮件主机设置" desc="这里填你自己的邮件服务器信息，填完点保存即可，不用改配置文件。">
      {!s ? <p className="text-sm text-muted-foreground">加载中…</p> : (
        <>
          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><Server size={16} /><b className="text-sm">邮件域名</b></div>
            <Field label="收信用的域名" hint="别人给这个地址发信，例如 mail.example.com（建议用 mail. 开头的子域名）">
              <Input placeholder="mail.example.com" value={s.mail_host || ''} onChange={e => field('mail_host', e.target.value)} />
            </Field>
            <Field label="服务器公网 IP" hint="你服务器的公网 IPv4 地址，MX 解析会指向它">
              <div className="flex gap-2">
                <Input placeholder="1.2.3.4" value={s.public_ip || ''} onChange={e => field('public_ip', e.target.value)} />
                <Button type="button" variant="outline" onClick={detectIP}>自动获取</Button>
              </div>
            </Field>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><Send size={16} /><b className="text-sm">发件中继（可选，但强烈建议）</b></div>
            <p className="text-xs text-muted-foreground">
              云服务器大多封了 25 端口，不能直接对外发信。填写邮件推送服务（阿里云邮件推送、SendGrid 等）后即可正常外发。
              只收不发可以先跳过。
            </p>
            <div className="grid sm:grid-cols-2 gap-3">
              <Field label="中继服务器" hint="例如 smtpdm.aliyun.com">
                <Input placeholder="smtp.example.com" value={s.relay_host || ''} onChange={e => field('relay_host', e.target.value)} />
              </Field>
              <Field label="端口" hint="一般是 587">
                <Input placeholder="587" value={s.relay_port || ''} onChange={e => field('relay_port', e.target.value)} />
              </Field>
              <Field label="用户名" hint="中继服务给的账号">
                <Input placeholder="账号" value={s.relay_user || ''} onChange={e => field('relay_user', e.target.value)} />
              </Field>
              <Field label="密码" hint="留空表示不修改">
                <Input type="password" placeholder={s.relay_pass_set ? '已设置（留空不改）' : '密码'} value={relayPass} onChange={e => setRelayPass(e.target.value)} />
              </Field>
            </div>
            <Field label="发件人兜底地址" hint="部分中继要求发件人必须等于认证账号">
              <Input placeholder="noreply@example.com" value={s.relay_from || ''} onChange={e => field('relay_from', e.target.value)} />
            </Field>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><KeyRound size={16} /><b className="text-sm">DKIM 签名</b>
              {s.dkim_ready ? <Badge className="text-green-600">已启用</Badge> : <Badge className="text-yellow-600">未启用</Badge>}
            </div>
            <p className="text-xs text-muted-foreground">
              DKIM 相当于给邮件盖个防伪章，能显著降低进垃圾箱的概率。点下面的按钮即可自动生成，无需命令。
            </p>
            {s.dkim_ready && dkim?.ready && (
              <div className="text-xs bg-muted/50 rounded-md p-3 space-y-1">
                <div>选择器：<code>{dkim.selector}</code> · 域名：<code>{dkim.domain}</code></div>
                <div className="font-mono break-all">TXT 记录：{dkim.name}.{dkim.domain} → "{dkim.txt}"</div>
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={genDKIM} disabled={busy === 'dkim'}>
                {busy === 'dkim' ? <Loader2 className="animate-spin" /> : <Wand2 />}一键生成 DKIM 密钥
              </Button>
              <UploadDKIM onDone={load} onMsg={setMsg} />
            </div>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><ShieldAlert size={16} /><b className="text-sm">管理员邮箱</b></div>
            <Field label="谁能进管理后台" hint="逗号分隔，可填多个；留空则只有系统里 admin 标记的账号能进">
              <Input placeholder="admin@example.com,boss@example.com" value={s.admin_emails || ''} onChange={e => field('admin_emails', e.target.value)} />
            </Field>
          </Card>

          {msg && <p className={'text-sm flex items-center gap-1 ' + (msg.includes('✓') ? 'text-green-600' : 'text-muted-foreground')}>
            {msg.includes('✓') && <CheckCircle2 size={14} />}{msg}</p>}

          <div className="flex justify-end">
            <Button onClick={save} disabled={busy === 'save'}>
              {busy === 'save' ? <Loader2 className="animate-spin" /> : <Save />}保存设置
            </Button>
          </div>
        </>
      )}
    </AdminShell>
  )
}

function Field({ label, hint, children }) {
  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
    </label>
  )
}

function UploadDKIM({ onDone, onMsg }) {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ domain: '', selector: 'dkim', private_key: '' })
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    try { await api.dkimUpload(form); onMsg('已导入 DKIM 私钥 ✓'); setOpen(false); onDone() }
    catch (e) { onMsg(e.message) } finally { setBusy(false) }
  }
  if (!open) return <Button size="sm" variant="outline" onClick={() => setOpen(true)}><Upload />导入已有私钥</Button>
  return (
    <div className="w-full space-y-2 border border-border rounded-md p-3">
      <div className="grid sm:grid-cols-2 gap-2">
        <Input placeholder="签名域名，如 example.com" value={form.domain} onChange={e => setForm({ ...form, domain: e.target.value })} />
        <Input placeholder="选择器（默认 dkim）" value={form.selector} onChange={e => setForm({ ...form, selector: e.target.value })} />
      </div>
      <textarea className="w-full h-28 rounded-md border border-border bg-background p-2 text-xs font-mono"
        placeholder="粘贴 PEM 私钥（-----BEGIN RSA PRIVATE KEY----- …）"
        value={form.private_key} onChange={e => setForm({ ...form, private_key: e.target.value })} />
      <div className="flex gap-2">
        <Button size="sm" onClick={submit} disabled={busy}>{busy ? <Loader2 className="animate-spin" /> : <Save />}保存</Button>
        <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>取消</Button>
      </div>
    </div>
  )
}
