import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { BRAND } from './brand'

export function cn(...inputs) { return twMerge(clsx(inputs)) }

// 转义 + 链接化 + 6 位数字验证码可点击复制（用于邮件正文展示）
export function linkify(text: string): string {
  let t = (text || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
  t = t.replace(/\b(https?:\/\/[^\s<]+)/g,
    '<a href="$1" target="_blank" rel="noopener noreferrer" class="text-primary underline break-all">$1</a>')
  t = t.replace(/\b([\w.+-]+@[\w-]+\.[\w.-]+)\b/g,
    '<a href="mailto:$1" class="text-primary underline">$1</a>')
  t = t.replace(/(^|[^\d])(\d{6})(?![\d])/g,
    '$1<code class="cursor-pointer rounded bg-muted px-1" data-code="$2" title="点击复制">$2</code>')
  return t
}

// 未读数写入标题与 favicon 角标
let origIcon: string | null = null
export function setUnreadBadge(n: number) {
  document.title = n > 0 ? `(${n}) ${BRAND}` : BRAND
  const link = document.querySelector("link[rel='icon']") as HTMLLinkElement | null
  if (!link) return
  if (!origIcon) origIcon = link.href
  const img = new Image()
  img.onload = () => {
    const c = document.createElement('canvas')
    c.width = 64
    c.height = 64
    const ctx = c.getContext('2d')
    if (!ctx) return
    ctx.drawImage(img, 0, 0, 64, 64)
    if (n > 0) {
      ctx.fillStyle = '#ef4444'
      ctx.beginPath()
      ctx.arc(48, 16, 14, 0, Math.PI * 2)
      ctx.fill()
      ctx.fillStyle = '#fff'
      ctx.font = 'bold 18px sans-serif'
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillText(n > 99 ? '99' : String(n), 48, 16)
    }
    link.href = c.toDataURL('image/png')
  }
  img.src = origIcon
}

// 引用原文
export function quoteMail(m: any): string {
  const when = m.created_at ? new Date(m.created_at).toLocaleString() : ''
  const lines = (m.body || '').split('\n').map((l: string) => '> ' + l).join('\n')
  return `\n\n在 ${when}，${m.from} 写道：\n${lines}`
}
