import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { zh } from './zh'
import { en } from './en'

// 轻量 i18n（无第三方依赖）。新增语言：加一份文案文件并注册到 DICTS。
export const DICTS = { zh, en }

const I18nCtx = createContext(null)

function detect() {
  const saved = localStorage.getItem('lang')
  if (saved) return saved
  return (navigator.language || 'en').toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

export function I18nProvider({ children }) {
  const [lang, setLang] = useState(detect)
  useEffect(() => {
    localStorage.setItem('lang', lang)
    document.documentElement.lang = lang
  }, [lang])
  const value = useMemo(() => ({
    lang,
    setLang,
    t: (key, vars) => {
      const s = DICTS[lang]?.[key] ?? DICTS.zh[key] ?? key
      return vars ? s.replace(/\{(\w+)\}/g, (_, k) => (vars[k] ?? '')) : s
    },
  }), [lang])
  return <I18nCtx.Provider value={value}>{children}</I18nCtx.Provider>
}

export function useI18n() {
  return useContext(I18nCtx) || { lang: 'zh', setLang: () => {}, t: (k) => k }
}
