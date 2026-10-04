import { Button } from './ui/controls'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/theme'
import { Sun, Moon, Monitor, Languages } from 'lucide-react'

export function ThemeToggle() {
  const { theme, cycle } = useTheme()
  const { t } = useI18n()
  const Icon = theme === 'light' ? Sun : theme === 'dark' ? Moon : Monitor
  const label = t('theme.' + theme)
  return (
    <Button variant="ghost" size="icon" onClick={cycle} title={label} aria-label={label}>
      <Icon />
    </Button>
  )
}

export function LangToggle() {
  const { lang, setLang } = useI18n()
  const next = lang === 'zh' ? 'English' : '中文'
  return (
    <Button variant="ghost" size="sm" onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')}
      title={next} aria-label={next}>
      <Languages />{lang === 'zh' ? 'EN' : '中文'}
    </Button>
  )
}

// 页面底部统一的语言 / 主题切换
import { BRAND } from '../lib/brand'
export function FooterControls() {
  return (
    <footer className="border-t border-border py-3 flex items-center justify-center gap-1 text-xs text-muted-foreground">
      <span className="mr-2">© {BRAND}</span>
      <LangToggle />
      <ThemeToggle />
    </footer>
  )
}
