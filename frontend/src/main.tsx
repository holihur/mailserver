import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { UIHost } from './components/UIHost'
import { I18nProvider } from './lib/i18n'
import { ThemeProvider } from './lib/theme'
import './index.css'

// 处理 OIDC 回调带回的 token（登录后跳回 /?oidc_token=...）
const sp = new URLSearchParams(location.search)
const ot = sp.get('oidc_token')
if (ot) {
  localStorage.setItem('token', ot)
  sp.delete('oidc_token')
  const q = sp.toString()
  history.replaceState(null, '', location.pathname + (q ? '?' + q : '') + location.hash)
}

createRoot(document.getElementById('root')).render(
  <ThemeProvider>
    <I18nProvider>
      <App />
      <UIHost />
    </I18nProvider>
  </ThemeProvider>
)
