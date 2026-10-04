import AdminShell from '../components/AdminShell'
import { RulesManager } from '../components/RulesManager'
import { useI18n } from '../lib/i18n'

export default function AdminRules() {
  const { t } = useI18n()
  return (
    <AdminShell title={t('rules.siteTitle')} desc={t('rules.siteDesc')}>
      <RulesManager site />
    </AdminShell>
  )
}
