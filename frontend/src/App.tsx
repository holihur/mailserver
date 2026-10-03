import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login from './pages/Login'
import MailApp from './pages/MailApp'
import DnsPage from './pages/DnsPage'
import Setup from './pages/Setup'
import AdminPage from './pages/AdminPage'
import AdminSettings from './pages/AdminSettings'
import AdminSSL from './pages/AdminSSL'
import AdminProviders from './pages/AdminProviders'
import AdminUsers from './pages/AdminUsers'

const authed = () => !!localStorage.getItem('token')

export default function App() {
  return (
    <HashRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/" element={authed() ? <MailApp /> : <Navigate to="/login" />} />
        <Route path="/dns" element={authed() ? <DnsPage /> : <Navigate to="/login" />} />
        <Route path="/setup" element={authed() ? <Setup /> : <Navigate to="/login" />} />
        <Route path="/admin" element={authed() ? <AdminPage /> : <Navigate to="/login" />} />
        <Route path="/admin/settings" element={authed() ? <AdminSettings /> : <Navigate to="/login" />} />
        <Route path="/admin/ssl" element={authed() ? <AdminSSL /> : <Navigate to="/login" />} />
        <Route path="/admin/providers" element={authed() ? <AdminProviders /> : <Navigate to="/login" />} />
        <Route path="/admin/users" element={authed() ? <AdminUsers /> : <Navigate to="/login" />} />
      </Routes>
    </HashRouter>
  )
}
