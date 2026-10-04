import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { UIHost } from './components/UIHost'
import { I18nProvider } from './lib/i18n'
import { ThemeProvider } from './lib/theme'
import './index.css'

createRoot(document.getElementById('root')).render(
  <ThemeProvider>
    <I18nProvider>
      <App />
      <UIHost />
    </I18nProvider>
  </ThemeProvider>
)
