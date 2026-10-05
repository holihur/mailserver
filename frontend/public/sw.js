// Sweetcorn 离线壳：导航请求 network-first，静态资源 stale-while-revalidate。
// API/协议端点不缓存，避免拿到过期邮件数据。
const CACHE = 'sweetcorn-v1'
const SHELL = ['/', '/index.html', '/icon.svg', '/manifest.webmanifest']

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()))
})

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (e) => {
  const req = e.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.origin !== location.origin) return
  if (/^\/(api|jmap|mcp|metrics)\b/.test(url.pathname)) return

  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).catch(() => caches.match('/index.html')))
    return
  }

  e.respondWith(
    caches.match(req).then((cached) => {
      const network = fetch(req)
        .then((res) => {
          if (res && res.ok && res.type === 'basic') {
            caches.open(CACHE).then((c) => c.put(req, res.clone()))
          }
          return res
        })
        .catch(() => cached)
      return cached || network
    }),
  )
})
