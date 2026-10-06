import PageShell from '../components/PageShell'
import AIProviderManager from '../components/AIProviderManager'
import { useI18n } from '../lib/i18n'
import { Sparkles } from 'lucide-react'

export default function AI() {
  const { t } = useI18n()
  return (
    <PageShell title={t('ai.title')} icon={Sparkles}>
      <p className="text-sm text-muted-foreground">{t('ai.desc')}</p>
      <AIProviderManager scope="user" />
    </PageShell>
  )
}
