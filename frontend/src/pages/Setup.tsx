import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Card, Badge, Input } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { useI18n } from '../lib/i18n'
import { RefreshCw, CheckCircle2, Clock, XCircle, KeyRound, Plus, Trash2, Copy, Settings } from 'lucide-react'

export default function Setup() {
  const { t } = useI18n()
  const [outbox, setOutbox] = useState([])
  const [dkim, setDkim] = useState(null)
  const [tokens, setTokens] = useState<any[]>([])
  const [tokName, setTokName] = useState('')
  const [newToken, setNewToken] = useState('')
  const host = location.hostname

  async function load() {
    try { setOutbox(await api.outbox()) } catch {}
    try { setDkim(await api.dkimGet()) } catch {}
    try { setTokens(await api.tokens()) } catch {}
  }
  useEffect(() => { load() }, [])

  async function createToken(e: any) {
    e.preventDefault()
    try {
      const r = await api.tokenCreate(tokName)
      setNewToken(r.token)
      setTokName('')
      setTokens(await api.tokens())
    } catch (err: any) { alert(err.message) }
  }
  async function revokeToken(id: number) {
    if (!confirm(t('token.confirmRevoke'))) return
    try { await api.tokenDelete(id); setTokens(await api.tokens()) } catch (e: any) { alert(e.message) }
  }
  function copyText(s: string) { navigator.clipboard?.writeText(s) }

  const rows = [
    [t('setup.smtp'), `${host}:587`, t('setup.starttls'), t('setup.authHint')],
    [t('setup.smtps'), `${host}:465`, t('setup.ssl'), t('setup.authHint')],
    [t('setup.pop3'), `${host}:110`, t('setup.plain'), t('setup.authHint')],
    [t('setup.pop3s'), `${host}:995`, t('setup.ssl'), t('setup.authHint')],
    [t('setup.imap'), `${host}:143`, t('setup.starttls'), t('setup.authHint')],
    [t('setup.imaps'), `${host}:993`, t('setup.ssl'), t('setup.authHint')],
    [t('setup.webmail'), `${host}/`, '—', t('setup.webmail')],
  ]

  return (
    <PageShell title={t('setup.title')} icon={Settings}>
        <Card className="p-4 space-y-3">
          <div className="flex items-center gap-2">
            <KeyRound size={16} />
            <b className="text-sm">{t('token.title')}</b>
            <Badge>{t('token.required')}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">{t('token.intro')}</p>
          <form onSubmit={createToken} className="flex gap-2">
            <Input placeholder={t('token.namePlaceholder')} value={tokName} onChange={e => setTokName(e.target.value)} />
            <Button size="sm" type="submit" className="shrink-0"><Plus />{t('token.generate')}</Button>
          </form>
          {newToken && (
            <div className="rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 space-y-2">
              <p className="text-xs text-yellow-700 dark:text-yellow-400">{t('token.onceWarning')}</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all text-xs font-mono bg-background rounded px-2 py-1">{newToken}</code>
                <Button size="sm" variant="outline" type="button" onClick={() => copyText(newToken)}><Copy />{t('common.copy')}</Button>
              </div>
            </div>
          )}
          {tokens.length > 0 && (
            <div className="text-sm">
              {tokens.map((tk: any) => (
                <div key={tk.id} className="flex items-center gap-2 border-t border-border py-2">
                  <span className="font-medium truncate max-w-[40%]">{tk.name}</span>
                  <code className="text-xs text-muted-foreground">{tk.prefix}…</code>
                  <span className="text-xs text-muted-foreground hidden sm:inline">
                    {tk.last_used ? t('token.lastUsed', { t: new Date(tk.last_used).toLocaleDateString() }) : t('token.neverUsed')}
                  </span>
                  <div className="flex-1" />
                  <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={() => revokeToken(tk.id)}><Trash2 /></Button>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card className="p-4 overflow-x-auto">
          <b className="text-sm">{t('setup.clientParams')}</b>
          <table className="w-full text-sm mt-2 min-w-[520px]">
            <thead><tr className="text-left text-muted-foreground text-xs">
              <th className="py-1">{t('setup.usage')}</th><th>{t('setup.server')}</th><th>{t('setup.encryption')}</th><th>{t('setup.account')}</th>
            </tr></thead>
            <tbody>
              {rows.map(r => (
                <tr key={r[0]} className="border-t border-border">
                  <td className="py-2 pr-2 font-medium">{r[0]}</td>
                  <td className="pr-2 font-mono text-xs">{r[1]}</td>
                  <td className="pr-2 text-xs text-muted-foreground">{r[2]}</td>
                  <td className="text-xs text-muted-foreground">{r[3]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>

        <Card className="p-4">
          <div className="flex items-center gap-2 mb-2">
            <b className="text-sm">{t('setup.outbox')} {t('setup.outboxHint')}</b>
            <div className="flex-1" />
            <Button variant="outline" size="sm" onClick={load}><RefreshCw />{t('common.refresh')}</Button>
          </div>
          {outbox.length === 0 && <p className="text-sm text-muted-foreground">{t('setup.noOutbox')}</p>}
          {outbox.map(m => (
            <div key={m.id} className="flex items-center gap-2 text-sm border-t border-border py-2">
              {m.relayed
                ? <span className="text-green-600 flex items-center gap-1 text-xs"><CheckCircle2 size={14} />OK</span>
                : m.attempts >= 8
                  ? <span className="text-red-500 flex items-center gap-1 text-xs"><XCircle size={14} />{t('users.disabled')}</span>
                  : <span className="text-yellow-600 flex items-center gap-1 text-xs"><Clock size={14} />{m.attempts}</span>}
              <span className="truncate flex-1">→ {m.to}《{m.subject || t('mail.noSubject')}》</span>
            </div>
          ))}
          {outbox.some(m => !m.relayed && m.relay_err) && (
            <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 mt-2 whitespace-pre-wrap">{outbox.find(m => !m.relayed && m.relay_err)?.relay_err}</pre>
          )}
        </Card>

        <Card className="p-4">
          <b className="text-sm">{t('setup.dkim')} {dkim?.ready ? <Badge className="text-green-600">{t('common.enabled')}</Badge> : <Badge className="text-yellow-600">{t('common.disabled')}</Badge>}</b>
          {!dkim?.ready ? (
            <p className="text-sm text-muted-foreground mt-1">{dkim?.hint || t('setup.dkimNotReady')}</p>
          ) : (
            <div className="text-sm mt-1 space-y-2">
              <p className="text-muted-foreground">
                selector <code>{dkim.selector}</code> · domain <code>{dkim.domain}</code> · {t('setup.dkimReady')}
              </p>
              <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto break-all whitespace-pre-wrap">{dkim.name}.{dkim.domain} TXT "{dkim.txt}"</pre>
              <Button size="sm" variant="outline" onClick={async () => { const r = await api.dkimPublish({ domain: dkim.domain }); alert(t('setup.publishOk', { name: r.name })) }}>{t('setup.publish')}</Button>
            </div>
          )}
        </Card>

        <Card className="p-4 text-sm text-muted-foreground space-y-1">
          <b className="text-foreground text-sm">{t('setup.why25')}</b>
          <p>{t('setup.why25Body')}</p>
        </Card>
    </PageShell>
  )
}
