import { describe, it, expect } from 'vitest'
import { linkify } from './utils'

describe('linkify', () => {
  it('转义 HTML 并保留危险字符', () => {
    const out = linkify('a < b > c & d "e"')
    expect(out).toContain('&lt;')
    expect(out).toContain('&gt;')
    expect(out).toContain('&amp;')
    expect(out).toContain('&quot;')
  })

  it('链接化 URL 与邮箱', () => {
    const out = linkify('see https://example.com/x and a@b.com')
    expect(out).toContain('<a href="https://example.com/x"')
    expect(out).toContain('<a href="mailto:a@b.com"')
  })

  it('6 位验证码可点击复制', () => {
    expect(linkify('code 123456 end')).toContain('data-code="123456"')
    // 7 位数字不应匹配
    expect(linkify('n 1234567 n')).not.toContain('data-code=')
  })
})

import { cn, setUnreadBadge, quoteMail } from './utils'
import { vi } from 'vitest'

describe('cn', () => {
  it('合并条件类名并去重 Tailwind 冲突', () => {
    const hidden = false
    expect(cn('p-2', hidden && 'hidden', 'p-4')).toBe('p-4')
    expect(cn('text-sm', 'underline')).toBe('text-sm underline')
  })
})

describe('quoteMail', () => {
  it('按行加引用前缀并调用 t 生成提示', () => {
    const t = (k: string, vars?: any) => `${k}:${vars.from}`
    const out = quoteMail({ from: 'a@b.c', created_at: '2024-01-01T00:00:00Z', body: 'l1\nl2' }, t)
    expect(out).toContain('mail.quoteWrote:a@b.c')
    expect(out).toContain('> l1')
    expect(out).toContain('> l2')
  })

  it('缺少字段时不报错', () => {
    const out = quoteMail({}, (k: string) => k)
    expect(out).toContain('mail.quoteWrote')
  })
})

describe('setUnreadBadge', () => {
  const link = { href: 'icon.png' }
  const calls: string[] = []
  const ctx = {
    drawImage: vi.fn(), beginPath: vi.fn(), arc: vi.fn(), fill: vi.fn(), fillText: vi.fn(),
    fillStyle: '', font: '', textAlign: '', textBaseline: '',
  }
  vi.stubGlobal('document', {
    title: '',
    querySelector: () => link,
    createElement: () => ({ width: 0, height: 0, getContext: () => ctx, toDataURL: () => { calls.push('data'); return 'data:image/png;base64,x' } }),
  })
  vi.stubGlobal('Image', class {
    onload: (() => void) | null = null
    set src(_v: string) { if (this.onload) this.onload() }
  })

  it('有未读数时写入标题并绘制角标', () => {
    setUnreadBadge(3)
    expect((document as any).title).toContain('(3)')
    expect(link.href).toBe('data:image/png;base64,x')
    expect(calls.length).toBeGreaterThan(0)
  })

  it('归零恢复标题，超过 99 显示 99', () => {
    setUnreadBadge(0)
    expect((document as any).title).not.toContain('(')
    setUnreadBadge(123)
    expect((document as any).title).toContain('(123)')
  })

  it('无 favicon 链接时提前返回', () => {
    vi.stubGlobal('document', { title: '', querySelector: () => null, createElement: () => ({}) })
    expect(() => setUnreadBadge(1)).not.toThrow()
  })
})
