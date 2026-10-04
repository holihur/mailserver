import { Link } from 'react-router-dom'
import { ThemeToggle, LangToggle } from '../components/HeaderControls'
import { RulesManager } from '../components/RulesManager'
import { useI18n } from '../lib/i18n'
import { BRAND } from '../lib/brand'
import { Filter } from 'lucide-react'

export default function Rules() {
  const { t } = useI18n()
  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3 sticky top-0 bg-background/90 backdrop-blur z-10">
        <Link to="/" className="font-semibold">← {BRAND}</Link>
        <span className="flex items-center gap-1 text-sm text-muted-foreground"><Filter size={15} />{t('rules.myTitle')}</span>
        <div className="flex-1" />
        <LangToggle />
        <ThemeToggle />
      </header>
      <div className="max-w-3xl mx-auto p-4 space-y-4">
        <p className="text-sm text-muted-foreground">{t('rules.myDesc')}</p>
        <RulesManager />
      </div>
    </div>
  )
}
