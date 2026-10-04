import { useRef, useState } from 'react'
import { X } from 'lucide-react'

export type Suggestion = { email: string; name?: string; note?: string }

// 多收件人输入：已选为标签，支持下拉从联系人/站内用户中选择；逗号/分号/回车确认，退格删末尾。
export function RecipientInput({
  value,
  onChange,
  placeholder,
  suggestions,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  suggestions: Suggestion[]
}) {
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)

  const emails = value ? value.split(',').map(s => s.trim()).filter(Boolean) : []
  const setEmails = (list: string[]) => onChange(list.join(', '))

  function add(email: string) {
    const e = email.trim().replace(/[,;]+$/, '')
    if (!e) return
    if (!emails.some(x => x.toLowerCase() === e.toLowerCase())) setEmails([...emails, e])
    setQ('')
  }
  function remove(e: string) {
    setEmails(emails.filter(x => x !== e))
  }

  const filtered = (q
    ? suggestions.filter(s => `${s.email} ${s.name || ''} ${s.note || ''}`.toLowerCase().includes(q.toLowerCase()))
    : suggestions
  )
    .filter(s => !emails.some(e => e.toLowerCase() === s.email.toLowerCase()))
    .slice(0, 8)

  return (
    <div ref={boxRef} className="relative">
      <div className="flex flex-wrap items-center gap-1 min-h-9 w-full rounded-md border border-border bg-background px-2 py-1 text-sm focus-within:ring-2 focus-within:ring-primary/30">
        {emails.map(e => (
          <span key={e} className="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs">
            {e}
            <button type="button" onClick={() => remove(e)} aria-label="remove">
              <X size={11} />
            </button>
          </span>
        ))}
        <input
          className="flex-1 min-w-[8rem] bg-transparent outline-none py-1"
          placeholder={emails.length ? '' : placeholder}
          value={q}
          onChange={e => {
            setQ(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => setTimeout(() => setOpen(false), 150)}
          onKeyDown={e => {
            if (e.key === 'Enter' || e.key === ',' || e.key === ';') {
              e.preventDefault()
              if (q) add(filtered[0]?.email || q)
            } else if (e.key === 'Backspace' && !q && emails.length) {
              remove(emails[emails.length - 1])
            }
          }}
        />
      </div>
      {open && filtered.length > 0 && (
        <div className="absolute z-50 mt-1 w-full max-h-56 overflow-auto rounded-md border border-border bg-card shadow-lg p-1">
          {filtered.map(s => (
            <button
              key={s.email}
              type="button"
              onMouseDown={e => e.preventDefault()}
              onClick={() => add(s.email)}
              className="w-full flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-left hover:bg-muted"
            >
              <span className="font-medium truncate">{s.name || s.email}</span>
              {s.name && <span className="text-xs text-muted-foreground truncate">{s.email}</span>}
              {s.note && <span className="text-xs text-muted-foreground truncate ml-auto">📝 {s.note}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
