import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login from './pages/Login'
import MailApp from './pages/MailApp'
import DnsPage from './pages/DnsPage'
import Setup from './pages/Setup'

const authed = () => !!localStorage.getItem('token')

export default function App() {
  return (
    <HashRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/" element={authed() ? <MailApp /> : <Navigate to="/login" />} />
        <Route path="/dns" element={authed() ? <DnsPage /> : <Navigate to="/login" />} />
        <Route path="/setup" element={authed() ? <Setup /> : <Navigate to="/login" />} />
      </Routes>
    </HashRouter>
  )
}
