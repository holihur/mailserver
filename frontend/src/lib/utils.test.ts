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
