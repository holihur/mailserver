// 邮件列表 / 阅读区的纯格式化工具（无 React 依赖）。

// 未读数超过 99 显示 99+（#38）。
export const fmtUnread = (n: number) => (n > 99 ? '99+' : String(n))

// 收件账户与主邮箱不同时（别名/多地址），生成一个紧凑标识（本地部分 + 确定性颜色）。
export function recipientChip(to: string, myEmail: string): { label: string; color: string } | null {
  const first = String(to || '').split(',')[0].trim()
  if (!first || !myEmail || first.toLowerCase() === myEmail.toLowerCase()) return null
  let h = 0
  for (let i = 0; i < first.length; i++) h = (h * 31 + first.charCodeAt(i)) % 360
  return { label: first.split('@')[0] || first, color: `hsl(${h} 65% 42%)` }
}

export function attList(s: any): any[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a : [] } catch { return [] }
}

export function attKind(type: string): string {
  if (!type) return ''
  if (type.startsWith('image/')) return 'image'
  if (type.startsWith('video/')) return 'video'
  if (type.startsWith('audio/')) return 'audio'
  if (type === 'application/pdf') return 'pdf'
  return ''
}

export function fmtSize(n: number) {
  if (!n) return '0B'
  if (n < 1024) return n + 'B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + 'KB'
  return (n / 1024 / 1024).toFixed(1) + 'MB'
}

export function fmtWhen(s: string) {
  if (!s) return ''
  const d = new Date(s)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return d.toLocaleDateString()
}

export function sumUnread(u: any): number {
  const vals = Object.values(u || {}) as number[]
  return vals.reduce((a, b) => a + (Number(b) || 0), 0)
}
