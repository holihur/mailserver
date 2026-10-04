// 极简 Markdown 渲染（用于展示 CHANGELOG）。先转义 HTML，再附加白名单标签，
// 避免 XSS。支持：# 标题、- / * 列表、> 引用、``` 代码块、**粗体**、`行内代码`、[链接](url)。

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function inline(s: string): string {
  let t = escapeHtml(s)
  t = t.replace(/`([^`]+)`/g, '<code class="rounded bg-muted px-1 py-0.5 text-xs font-mono">$1</code>')
  t = t.replace(/\*\*([^*]+)\*\*/g, '<strong class="font-semibold">$1</strong>')
  t = t.replace(
    /\[([^\]]+)\]\((https?:[^)\s]+)\)/g,
    '<a href="$2" target="_blank" rel="noopener noreferrer" class="text-primary underline underline-offset-2">$1</a>',
  )
  return t
}

export function renderMarkdown(md: string): string {
  const lines = (md || '').replace(/\r\n/g, '\n').split('\n')
  let html = ''
  let inList = false
  let inCode = false

  const closeList = () => {
    if (inList) {
      html += '</ul>'
      inList = false
    }
  }

  for (const raw of lines) {
    const line = raw
    if (line.trim().startsWith('```')) {
      if (inCode) {
        html += '</code></pre>'
        inCode = false
      } else {
        closeList()
        html += '<pre class="rounded-md bg-muted/60 p-3 overflow-x-auto text-xs font-mono my-2"><code>'
        inCode = true
      }
      continue
    }
    if (inCode) {
      html += escapeHtml(line) + '\n'
      continue
    }
    const heading = line.match(/^(#{1,6})\s+(.*)$/)
    if (heading) {
      closeList()
      const level = heading[1].length
      const sizes: Record<number, string> = {
        1: 'text-lg font-semibold mt-4 mb-2',
        2: 'text-base font-semibold mt-4 mb-1.5',
        3: 'text-sm font-semibold mt-3 mb-1',
        4: 'text-sm font-medium mt-2 mb-1',
        5: 'text-sm font-medium mt-2 mb-1',
        6: 'text-sm font-medium mt-2 mb-1',
      }
      html += `<h${level} class="${sizes[level] || sizes[6]}">${inline(heading[2])}</h${level}>`
      continue
    }
    const bullet = line.match(/^\s*[-*]\s+(.*)$/)
    if (bullet) {
      if (!inList) {
        html += '<ul class="list-disc pl-5 space-y-0.5 my-1">'
        inList = true
      }
      html += `<li>${inline(bullet[1])}</li>`
      continue
    }
    if (/^\s*>\s?/.test(line)) {
      closeList()
      html += `<blockquote class="border-l-2 border-border pl-3 text-muted-foreground my-2">${inline(line.replace(/^\s*>\s?/, ''))}</blockquote>`
      continue
    }
    if (line.trim() === '') {
      closeList()
      continue
    }
    closeList()
    html += `<p class="my-1.5 leading-relaxed">${inline(line)}</p>`
  }
  closeList()
  if (inCode) html += '</code></pre>'
  return html
}
