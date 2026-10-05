import { describe, it, expect, beforeEach, vi } from 'vitest'
import { api } from './client'

const store: Record<string, string> = {}

beforeEach(() => {
  vi.restoreAllMocks()
  for (const k of Object.keys(store)) delete store[k]
  ;(globalThis as any).localStorage = {
    getItem: (k: string) => (k in store ? store[k] : null),
    setItem: (k: string, v: string) => { store[k] = v },
    removeItem: (k: string) => { delete store[k] },
  }
  ;(globalThis as any).location = { pathname: '/', href: '' }
})

function mockFetch(resp: any) {
  const calls: any[] = []
  ;(globalThis as any).fetch = vi.fn(async (url: string, opts: any = {}) => {
    calls.push({ url, opts })
    return {
      ok: resp.ok ?? true,
      status: resp.status ?? 200,
      json: async () => resp.body ?? {},
      blob: async () => resp.blob ?? new Blob(['x']),
    }
  })
  return calls
}

describe('api client', () => {
  it('GET 拼接路径并带 Authorization 头', async () => {
    store.token = 'tok'
    const calls = mockFetch({ body: { version: '1' } })
    const r = await api.version()
    expect(r.version).toBe('1')
    expect(calls[0].url).toBe('/api/version')
    expect(calls[0].opts.headers.Authorization).toBe('Bearer tok')
  })

  it('POST 序列化 body，无 token 时不带 Authorization', async () => {
    const calls = mockFetch({ body: { id: 1 } })
    await api.login({ email: 'a@b.c', password: 'x' })
    expect(calls[0].opts.method).toBe('POST')
    expect(JSON.parse(calls[0].opts.body)).toEqual({ email: 'a@b.c', password: 'x' })
    expect(calls[0].opts.headers.Authorization).toBeUndefined()
  })

  it('搜索查询参数做 URL 编码', async () => {
    const calls = mockFetch({ body: { items: [] } })
    await api.list('inbox', 'a b&c', 2, 'oldest')
    expect(calls[0].url).toBe('/api/mails?folder=inbox&q=a%20b%26c&page=2&sort=oldest')
  })

  it('非 2xx 抛出后端 error', async () => {
    mockFetch({ ok: false, status: 400, body: { error: 'bad' } })
    await expect(api.version()).rejects.toThrow('bad')
  })

  it('非 JSON 错误响应回退默认文案', async () => {
    ;(globalThis as any).fetch = vi.fn(async () => ({
      ok: false, status: 500, json: async () => { throw new Error('not json') },
    }))
    await expect(api.version()).rejects.toThrow('请求失败')
  })

  it('401 清除 token 并跳转登录', async () => {
    store.token = 'tok'
    mockFetch({ ok: false, status: 401, body: {} })
    await expect(api.version()).rejects.toThrow()
    expect(store.token).toBeUndefined()
    expect((globalThis as any).location.href).toBe('/login')
  })

  it('backupDownload 触发浏览器下载', async () => {
    const click = vi.fn()
    ;(globalThis as any).document = { createElement: () => ({ click }) }
    ;(globalThis as any).URL = { createObjectURL: () => 'blob:x', revokeObjectURL: vi.fn() }
    const calls = mockFetch({ blob: new Blob(['x']) })
    await api.backupDownload('mailserver-1.tar.gz')
    expect(calls[0].url).toContain('/api/admin/backups?action=download&name=mailserver-1.tar.gz')
    expect(click).toHaveBeenCalled()
  })
})
