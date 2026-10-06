import AdminShell from '../components/AdminShell'
import AIProviderManager from '../components/AIProviderManager'
import { useI18n } from '../lib/i18n'

export default function AdminAI() {
  const { t } = useI18n()
  return (
    <AdminShell title={t('ai.adminTitle')} desc={t('ai.adminDesc')}>
      <AIProviderManager scope="system" />
    </AdminShell>
  )
}
