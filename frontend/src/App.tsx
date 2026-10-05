import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Loader2 } from 'lucide-react'

// 路由级代码分割：首屏只加载 React/Router + 登录页，其余页面按需加载。
const Login = lazy(() => import('./pages/Login'))
const MailApp = lazy(() => import('./pages/MailApp'))
const DnsPage = lazy(() => import('./pages/DnsPage'))
const Setup = lazy(() => import('./pages/Setup'))
const AdminPage = lazy(() => import('./pages/AdminPage'))
const AdminSettings = lazy(() => import('./pages/AdminSettings'))
const AdminSSL = lazy(() => import('./pages/AdminSSL'))
const AdminProviders = lazy(() => import('./pages/AdminProviders'))
const AdminUsers = lazy(() => import('./pages/AdminUsers'))
const AdminAbout = lazy(() => import('./pages/AdminAbout'))
const AdminAudit = lazy(() => import('./pages/AdminAudit'))
const AdminRules = lazy(() => import('./pages/AdminRules'))
const AdminRoutes = lazy(() => import('./pages/AdminRoutes'))
const AdminAliases = lazy(() => import('./pages/AdminAliases'))
const AdminBackup = lazy(() => import('./pages/AdminBackup'))
const Contacts = lazy(() => import('./pages/Contacts'))
const Accounts = lazy(() => import('./pages/Accounts'))
const Rules = lazy(() => import('./pages/Rules'))
const Security = lazy(() => import('./pages/Security'))
const Sieve = lazy(() => import('./pages/Sieve'))
const Scheduled = lazy(() => import('./pages/Scheduled'))
const Privacy = lazy(() => import('./pages/Privacy'))

const authed = () => !!localStorage.getItem('token')

function Loading() {
  return (
    <div className="min-h-screen grid place-items-center bg-background text-muted-foreground">
      <Loader2 className="animate-spin" />
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route path="/login" element={authed() ? <Navigate to="/" /> : <Login />} />
          <Route path="/" element={authed() ? <MailApp /> : <Navigate to="/login" />} />
          <Route path="/dns" element={authed() ? <DnsPage /> : <Navigate to="/login" />} />
          <Route path="/setup" element={authed() ? <Setup /> : <Navigate to="/login" />} />
          <Route path="/contacts" element={authed() ? <Contacts /> : <Navigate to="/login" />} />
          <Route path="/accounts" element={authed() ? <Accounts /> : <Navigate to="/login" />} />
          <Route path="/rules" element={authed() ? <Rules /> : <Navigate to="/login" />} />
          <Route path="/security" element={authed() ? <Security /> : <Navigate to="/login" />} />
          <Route path="/sieve" element={authed() ? <Sieve /> : <Navigate to="/login" />} />
          <Route path="/scheduled" element={authed() ? <Scheduled /> : <Navigate to="/login" />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="/admin" element={authed() ? <AdminPage /> : <Navigate to="/login" />} />
          <Route path="/admin/settings" element={authed() ? <AdminSettings /> : <Navigate to="/login" />} />
          <Route path="/admin/ssl" element={authed() ? <AdminSSL /> : <Navigate to="/login" />} />
          <Route path="/admin/providers" element={authed() ? <AdminProviders /> : <Navigate to="/login" />} />
          <Route path="/admin/users" element={authed() ? <AdminUsers /> : <Navigate to="/login" />} />
          <Route path="/admin/about" element={authed() ? <AdminAbout /> : <Navigate to="/login" />} />
          <Route path="/admin/audit" element={authed() ? <AdminAudit /> : <Navigate to="/login" />} />
          <Route path="/admin/rules" element={authed() ? <AdminRules /> : <Navigate to="/login" />} />
          <Route path="/admin/routes" element={authed() ? <AdminRoutes /> : <Navigate to="/login" />} />
          <Route path="/admin/aliases" element={authed() ? <AdminAliases /> : <Navigate to="/login" />} />
          <Route path="/admin/backup" element={authed() ? <AdminBackup /> : <Navigate to="/login" />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  )
}
