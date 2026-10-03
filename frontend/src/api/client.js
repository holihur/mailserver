const BASE = import.meta.env.VITE_API || ''

function headers() {
  const t = localStorage.getItem('token')
  return { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }
}

async function req(path, opts = {}) {
  const r = await fetch(BASE + path, { ...opts, headers: headers() })
  if (r.status === 401) { localStorage.removeItem('token'); location.hash = '#/login' }
  const data = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error(data.error || '请求失败')
  return data
}

export const api = {
  register: (b) => req('/api/register', { method: 'POST', body: JSON.stringify(b) }),
  login: (b) => req('/api/login', { method: 'POST', body: JSON.stringify(b) }),
  me: () => req('/api/me'),
  list: (folder = 'inbox', q = '', page = 1) =>
    req(`/api/mails?folder=${folder}&q=${encodeURIComponent(q)}&page=${page}`),
  get: (id) => req(`/api/mails/${id}`),
  send: (b) => req('/api/mails', { method: 'POST', body: JSON.stringify(b) }),
  patch: (id, b) => req(`/api/mails/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  trash: (id) => req(`/api/mails/${id}`, { method: 'DELETE' }),
  outbox: () => req('/api/outbox'),
  dkimGet: () => req('/api/dkim'),
  dkimPublish: (b) => req('/api/dkim', { method: 'POST', body: JSON.stringify(b) }),
  dnsList: () => req('/api/domains'),
  dnsCreate: (b) => req('/api/domains', { method: 'POST', body: JSON.stringify(b) }),
  dnsGet: (id) => req(`/api/domains/${id}`),
  dnsDelete: (id) => req(`/api/domains/${id}`, { method: 'DELETE' }),
  dnsRecCreate: (id, b) => req(`/api/domains/${id}/records`, { method: 'POST', body: JSON.stringify(b) }),
  dnsRecDelete: (id, rid) => req(`/api/domains/${id}/records/${rid}`, { method: 'DELETE' }),
  dnsZone: async (id) => { const r = await fetch((import.meta.env.VITE_API || '') + `/api/domains/${id}/zone`, { headers: localStorage.getItem('token') ? { Authorization: 'Bearer ' + localStorage.getItem('token') } : {} }); return r.text() },
  adminOverview: () => req('/api/admin/overview'),
  providers: () => req('/api/admin/providers'),
  providerCreate: (b) => req('/api/admin/providers', { method: 'POST', body: JSON.stringify(b) }),
  providerDelete: (id) => req(`/api/admin/providers/${id}`, { method: 'DELETE' }),
  providerTest: (id) => req(`/api/admin/providers/${id}/test`, { method: 'POST' }),
  providerDomains: (id) => req(`/api/admin/providers/${id}/domains`),
  providerApply: (id, b) => req(`/api/admin/providers/${id}/apply`, { method: 'POST', body: JSON.stringify(b) }),
}
