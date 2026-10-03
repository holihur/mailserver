import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { Server, ShieldCheck, Cloud, Users, KeyRound, CheckCircle2, Circle, Globe } from 'lucide-react'

export default function AdminPage() {
  const [ov, setOv] = useState(null)
  const [tls, setTls] = useState(null)
  const [settings, setSettings] = useState(null)
  const [providers, setProviders] = useState([])

  useEffect(() => {
    api.adminOverview().then(setOv).catch(() => {})
    api.tlsGet().then(setTls).catch(() => {})
    api.settingsGet().then(setSettings).catch(() => {})
    api.providers().then(setProviders).catch(() => {})
  }, [])

  const host = settings?.mail_host
  const steps = [
    { ok: !!host, label: '填写邮件域名（如 mail.example.com）', to: '#/admin/settings' },
    { ok: providers.length > 0, label: '接入域名服务商（阿里云 / Cloudflare）', to: '#/admin/providers' },
    { ok: !!tls?.cert?.exists, label: '配置 SSL 证书（一键申请 Let\'s Encrypt）', to: '#/admin/ssl' },
    { ok: !!settings?.dkim_ready, label: '生成 DKIM 密钥（防进垃圾箱）', to: '#/admin/settings' },
    { ok: (ov?.users || 0) > 0, label: '创建邮箱账号', to: '#/admin/users' },
  ]

  return (
    <AdminShell title="概览" desc="按下面的清单一步步配置，通常 5 分钟就能收发邮件。">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[
          ['邮箱账号', ov?.users ?? '—', Users, '#/admin/users'],
          ['托管域名', ov?.domains ?? '—', Globe, '#/admin/providers'],
          ['邮件总数', ov?.mails ?? '—', Server, null],
          ['待发出', ov?.pending ?? '—', Cloud, null],
        ].map(([k, v, Icon, to]) => (
          <Card key={k} className="p-3">
            <div className="flex items-center gap-1 text-xs text-muted-foreground"><Icon size={13} />{k}</div>
            <div className="text-2xl font-semibold mt-1">{v}</div>
          </Card>
        ))}
      </div>

      <Card className="p-4">
        <b className="text-sm">配置清单</b>
        <div className="mt-3 space-y-1">
          {steps.map((s, i) => (
            <a key={i} href={s.to} className="flex items-center gap-2 rounded-md px-2 py-2 hover:bg-muted text-sm">
              {s.ok ? <CheckCircle2 size={16} className="text-green-600" /> : <Circle size={16} className="text-muted-foreground" />}
              <span className={s.ok ? 'text-muted-foreground line-through' : ''}>{s.label}</span>
            </a>
          ))}
        </div>
      </Card>

      <div className="grid sm:grid-cols-3 gap-3">
        <QuickCard to="#/admin/settings" icon={Server} title="邮件主机" desc="域名、公网 IP、DKIM、发件中继" />
        <QuickCard to="#/admin/ssl" icon={ShieldCheck} title="SSL 证书" desc="自动申请或手动上传" />
        <QuickCard to="#/admin/providers" icon={Cloud} title="域名服务商" desc="一键下发 MX/SPF/DKIM" />
      </div>

      <Card className="p-4 text-xs text-muted-foreground">
        提示：改动大部分设置会立即生效，无需重启；SMTP/IMAP/POP3 的端口号等少数系统参数需要重启进程。
      </Card>
    </AdminShell>
  )
}

function QuickCard({ to, icon: Icon, title, desc }) {
  return (
    <a href={to} className="block">
      <Card className="p-4 hover:bg-muted/50 transition-colors h-full">
        <Icon size={18} />
        <div className="font-medium mt-2">{title}</div>
        <div className="text-xs text-muted-foreground mt-0.5">{desc}</div>
      </Card>
    </a>
  )
}
