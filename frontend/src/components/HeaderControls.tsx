import { Button } from './ui/controls'
import { Dropdown, DropdownItem, DropdownLabel, DropdownSeparator } from './Dropdown'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/theme'
import { Sun, Moon, Monitor, Languages, Settings2 } from 'lucide-react'

// 右上角统一的下拉菜单：主题 + 语言。
export function SettingsMenu() {
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  const tick = (on: boolean) => (on ? ' ✓' : '')
  return (
    <Dropdown align="right" trigger={
      <Button variant="ghost" size="icon" aria-label={t('nav.settings')} title={t('nav.settings')}><Settings2 /></Button>
    }>
      <DropdownLabel>{t('theme.label')}</DropdownLabel>
      <DropdownItem icon={Sun} onClick={() => setTheme('light')}>{t('theme.light')}{tick(theme === 'light')}</DropdownItem>
      <DropdownItem icon={Moon} onClick={() => setTheme('dark')}>{t('theme.dark')}{tick(theme === 'dark')}</DropdownItem>
      <DropdownItem icon={Monitor} onClick={() => setTheme('system')}>{t('theme.system')}{tick(theme === 'system')}</DropdownItem>
      <DropdownSeparator />
      <DropdownLabel>{t('lang.label')}</DropdownLabel>
      <DropdownItem icon={Languages} onClick={() => setLang('zh')}>中文{tick(lang === 'zh')}</DropdownItem>
      <DropdownItem icon={Languages} onClick={() => setLang('en')}>English{tick(lang === 'en')}</DropdownItem>
    </Dropdown>
  )
}
