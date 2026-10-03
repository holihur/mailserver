import { createContext, useContext, useEffect, useMemo, useState } from 'react'

// 主题：light / dark / system，持久化，跟随系统时自动切换。
const ThemeCtx = createContext(null)

const systemDark = () =>
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches

function detect() {
  const saved = localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark' || saved === 'system') return saved
  return 'system'
}

export function ThemeProvider({ children }) {
  const [theme, setTheme] = useState(detect)

  useEffect(() => {
    const apply = () => {
      const dark = theme === 'dark' || (theme === 'system' && systemDark())
      document.documentElement.classList.toggle('dark', dark)
      const meta = document.querySelector('meta[name="theme-color"]')
      if (meta) meta.setAttribute('content', dark ? '#0b1220' : '#ffffff')
    }
    apply()
    localStorage.setItem('theme', theme)
    if (theme !== 'system') return undefined
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    mq.addEventListener?.('change', apply)
    return () => mq.removeEventListener?.('change', apply)
  }, [theme])

  const value = useMemo(() => ({
    theme,
    setTheme,
    cycle: () => setTheme(t => (t === 'light' ? 'dark' : t === 'dark' ? 'system' : 'light')),
  }), [theme])

  return <ThemeCtx.Provider value={value}>{children}</ThemeCtx.Provider>
}

export function useTheme() {
  return useContext(ThemeCtx) || { theme: 'system', setTheme: () => {}, cycle: () => {} }
}
