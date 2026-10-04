const BASE = import.meta.env.VITE_API || ''

function headers() {
  const t = localStorage.getItem('token')
  return { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }
}

async function req(path, opts = {}) {
  const r = await fetch(BASE + path, { ...opts, headers: headers() })
  if (r.status === 401) { localStorage.removeItem('token'); if (location.pathname !== '/login') location.href = '/login' }
  const data = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error(data.error || '请求失败')
  return data
}

export const api = {
  site: () => req('/api/site'),
  version: () => req('/api/version'),
  changelog: () => req('/api/changelog'),
  register: (b) => req('/api/register', { method: 'POST', body: JSON.stringify(b) }),
  login: (b) => req('/api/login', { method: 'POST', body: JSON.stringify(b) }),
  loginTOTP: (b) => req('/api/login/totp', { method: 'POST', body: JSON.stringify(b) }),
  totp: () => req('/api/totp'),
  totpSetup: () => req('/api/totp/setup', { method: 'POST' }),
  totpEnable: (code) => req('/api/totp/enable', { method: 'POST', body: JSON.stringify({ code }) }),
  totpDisable: (code) => req('/api/totp/disable', { method: 'POST', body: JSON.stringify({ code }) }),
  gdprExport: () => req('/api/gdpr/export'),
  gdprDelete: (password) => req('/api/gdpr/delete', { method: 'POST', body: JSON.stringify({ password }) }),
  me: () => req('/api/me'),
  list: (folder = 'inbox', q = '', page = 1, sort = 'newest') =>
    req(`/api/mails?folder=${folder}&q=${encodeURIComponent(q)}&page=${page}&sort=${sort}`),
  get: (id) => req(`/api/mails/${id}`),
  send: (b) => req('/api/mails', { method: 'POST', body: JSON.stringify(b) }),
  patch: (id, b) => req(`/api/mails/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  trash: (id) => req(`/api/mails/${id}`, { method: 'DELETE' }),
  batch: (ids, action, folder = '') => req('/api/mails/batch', { method: 'POST', body: JSON.stringify({ ids, action, folder }) }),
  emptyTrash: () => req('/api/mails/batch', { method: 'POST', body: JSON.stringify({ ids: [], action: 'empty' }) }),
  contacts: () => req('/api/contacts'),
  contactCreate: (b) => req('/api/contacts', { method: 'POST', body: JSON.stringify(b) }),
  contactPatch: (id, b) => req(`/api/contacts/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  contactDelete: (id) => req(`/api/contacts/${id}`, { method: 'DELETE' }),
  directory: () => req('/api/directory'),
  rules: () => req('/api/rules'),
  ruleCreate: (b) => req('/api/rules', { method: 'POST', body: JSON.stringify(b) }),
  rulePatch: (id, b) => req(`/api/rules/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  ruleDelete: (id) => req(`/api/rules/${id}`, { method: 'DELETE' }),
  ruleTest: (b) => req('/api/rules/test', { method: 'POST', body: JSON.stringify(b) }),
  adminRules: () => req('/api/admin/rules'),
  adminRuleCreate: (b) => req('/api/admin/rules', { method: 'POST', body: JSON.stringify(b) }),
  adminRulePatch: (id, b) => req(`/api/admin/rules/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  adminRuleDelete: (id) => req(`/api/admin/rules/${id}`, { method: 'DELETE' }),
  adminRuleTest: (b) => req('/api/admin/rules/test', { method: 'POST', body: JSON.stringify(b) }),
  adminRoutes: () => req('/api/admin/routes'),
  adminRouteCreate: (b) => req('/api/admin/routes', { method: 'POST', body: JSON.stringify(b) }),
  adminRoutePatch: (id, b) => req(`/api/admin/routes/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  adminRouteDelete: (id) => req(`/api/admin/routes/${id}`, { method: 'DELETE' }),
  adminRouteTest: (domain) => req('/api/admin/routes/test', { method: 'POST', body: JSON.stringify({ domain }) }),
  outbox: () => req('/api/outbox'),
  tokens: () => req('/api/tokens'),
  tokenCreate: (name) => req('/api/tokens', { method: 'POST', body: JSON.stringify({ name }) }),
  tokenDelete: (id) => req(`/api/tokens/${id}`, { method: 'DELETE' }),
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
  adminAbout: () => req('/api/admin/about'),
  adminUpdateCheck: () => req('/api/admin/update/check'),
  adminUpdate: () => req('/api/admin/update', { method: 'POST' }),
  providers: () => req('/api/admin/providers'),
  providerCreate: (b) => req('/api/admin/providers', { method: 'POST', body: JSON.stringify(b) }),
  providerDelete: (id) => req(`/api/admin/providers/${id}`, { method: 'DELETE' }),
  providerTest: (id) => req(`/api/admin/providers/${id}/test`, { method: 'POST' }),
  providerDomains: (id) => req(`/api/admin/providers/${id}/domains`),
  providerApply: (id, b) => req(`/api/admin/providers/${id}/apply`, { method: 'POST', body: JSON.stringify(b) }),
  settingsGet: () => req('/api/admin/settings'),
  relayTest: () => req('/api/admin/relay/test', { method: 'POST' }),
  settingsPatch: (b) => req('/api/admin/settings', { method: 'PATCH', body: JSON.stringify(b) }),
  tlsGet: () => req('/api/admin/tls'),
  tlsManual: (b) => req('/api/admin/tls/manual', { method: 'POST', body: JSON.stringify(b) }),
  tlsDelete: () => req('/api/admin/tls', { method: 'DELETE' }),
  tlsAcme: (b) => req('/api/admin/tls/acme', { method: 'POST', body: JSON.stringify(b) }),
  dkimGenerate: (b) => req('/api/admin/dkim/generate', { method: 'POST', body: JSON.stringify(b) }),
  dkimUpload: (b) => req('/api/admin/dkim/upload', { method: 'POST', body: JSON.stringify(b) }),
  adminUsers: () => req('/api/admin/users'),
  adminUserCreate: (b) => req('/api/admin/users', { method: 'POST', body: JSON.stringify(b) }),
  adminUserPatch: (id, b) => req(`/api/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(b) }),
  adminUserDelete: (id) => req(`/api/admin/users/${id}`, { method: 'DELETE' }),
}
