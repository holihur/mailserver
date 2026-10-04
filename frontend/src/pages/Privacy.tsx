import PageShell from '../components/PageShell'
import { Card } from '../components/ui/controls'
import { useI18n } from '../lib/i18n'
import { ShieldCheck } from 'lucide-react'

export default function Privacy() {
  const { t } = useI18n()
  const sections = [
    ['privacy.collectTitle', 'privacy.collectBody'],
    ['privacy.useTitle', 'privacy.useBody'],
    ['privacy.retentionTitle', 'privacy.retentionBody'],
    ['privacy.rightsTitle', 'privacy.rightsBody'],
    ['privacy.contactTitle', 'privacy.contactBody'],
  ]
  return (
    <PageShell title={t('privacy.title')} icon={ShieldCheck}>
      <p className="text-sm text-muted-foreground">{t('privacy.intro')}</p>
      <Card className="p-4 space-y-4">
        {sections.map(([h, b]) => (
          <div key={h}>
            <b className="text-sm">{t(h)}</b>
            <p className="text-sm text-muted-foreground mt-1 whitespace-pre-line">{t(b)}</p>
          </div>
        ))}
      </Card>
    </PageShell>
  )
}
