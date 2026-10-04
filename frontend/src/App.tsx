import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login from './pages/Login'
import MailApp from './pages/MailApp'
import DnsPage from './pages/DnsPage'
import Setup from './pages/Setup'
import AdminPage from './pages/AdminPage'
import AdminSettings from './pages/AdminSettings'
import AdminSSL from './pages/AdminSSL'
import AdminProviders from './pages/AdminProviders'
import AdminUsers from './pages/AdminUsers'
import AdminAbout from './pages/AdminAbout'
import AdminRules from './pages/AdminRules'
import AdminRoutes from './pages/AdminRoutes'
import AdminAliases from './pages/AdminAliases'
import Contacts from './pages/Contacts'
import Accounts from './pages/Accounts'
import Rules from './pages/Rules'
import Security from './pages/Security'
import Sieve from './pages/Sieve'
import Privacy from './pages/Privacy'

const authed = () => !!localStorage.getItem('token')

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/" element={authed() ? <MailApp /> : <Navigate to="/login" />} />
        <Route path="/dns" element={authed() ? <DnsPage /> : <Navigate to="/login" />} />
        <Route path="/setup" element={authed() ? <Setup /> : <Navigate to="/login" />} />
        <Route path="/contacts" element={authed() ? <Contacts /> : <Navigate to="/login" />} />
        <Route path="/accounts" element={authed() ? <Accounts /> : <Navigate to="/login" />} />
        <Route path="/rules" element={authed() ? <Rules /> : <Navigate to="/login" />} />
        <Route path="/security" element={authed() ? <Security /> : <Navigate to="/login" />} />
        <Route path="/sieve" element={authed() ? <Sieve /> : <Navigate to="/login" />} />
        <Route path="/privacy" element={<Privacy />} />
        <Route path="/admin" element={authed() ? <AdminPage /> : <Navigate to="/login" />} />
        <Route path="/admin/settings" element={authed() ? <AdminSettings /> : <Navigate to="/login" />} />
        <Route path="/admin/ssl" element={authed() ? <AdminSSL /> : <Navigate to="/login" />} />
        <Route path="/admin/providers" element={authed() ? <AdminProviders /> : <Navigate to="/login" />} />
        <Route path="/admin/users" element={authed() ? <AdminUsers /> : <Navigate to="/login" />} />
        <Route path="/admin/about" element={authed() ? <AdminAbout /> : <Navigate to="/login" />} />
        <Route path="/admin/rules" element={authed() ? <AdminRules /> : <Navigate to="/login" />} />
        <Route path="/admin/routes" element={authed() ? <AdminRoutes /> : <Navigate to="/login" />} />
        <Route path="/admin/aliases" element={authed() ? <AdminAliases /> : <Navigate to="/login" />} />
      </Routes>
    </BrowserRouter>
  )
}
