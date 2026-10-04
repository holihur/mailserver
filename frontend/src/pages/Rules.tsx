import PageShell from '../components/PageShell'
import { RulesManager } from '../components/RulesManager'
import { useI18n } from '../lib/i18n'
import { Filter } from 'lucide-react'

export default function Rules() {
  const { t } = useI18n()
  return (
    <PageShell title={t('rules.myTitle')} icon={Filter}>
      <p className="text-sm text-muted-foreground">{t('rules.myDesc')}</p>
      <RulesManager />
    </PageShell>
  )
}
