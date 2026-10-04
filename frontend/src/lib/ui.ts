// 全局轻量 UI：Toast 通知 + 异步确认框（替代原生 alert/confirm）。
// 通过模块级事件订阅与 <UIHost/> 配合，可在任意位置调用 toast()/confirmAsync()。

export type ToastType = 'info' | 'success' | 'error'

export type ToastItem = {
  id: number
  msg: string
  type: ToastType
  action?: { label: string; onClick: () => void }
  timeout: number
}

export type ConfirmItem = { id: number; msg: string; resolve: (v: boolean) => void }

export type PromptItem = {
  id: number
  msg: string
  defaultValue: string
  password: boolean
  inputType: 'text' | 'number'
  placeholder?: string
  resolve: (v: string | null) => void
}

let toasts: ToastItem[] = []
let confirms: ConfirmItem[] = []
let prompts: PromptItem[] = []
const listeners = new Set<() => void>()
let seq = 1

function emit() {
  listeners.forEach(l => l())
}

export function subscribeUI(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export const getToasts = () => toasts
export const getConfirms = () => confirms
export const getPrompts = () => prompts

export function dismissToast(id: number) {
  toasts = toasts.filter(t => t.id !== id)
  emit()
}

// toast 支持带一个操作按钮（如「撤销」）
export function toast(msg: string, opts: { type?: ToastType; action?: ToastItem['action']; timeout?: number } = {}) {
  const t: ToastItem = {
    id: seq++,
    msg,
    type: opts.type || 'info',
    action: opts.action,
    timeout: opts.timeout ?? (opts.action ? 8000 : 3500),
  }
  toasts = [...toasts, t]
  emit()
  window.setTimeout(() => dismissToast(t.id), t.timeout)
  return t.id
}

export function confirmAsync(msg: string): Promise<boolean> {
  return new Promise(resolve => {
    const c: ConfirmItem = { id: seq++, msg, resolve }
    confirms = [...confirms, c]
    emit()
  })
}

export function resolveConfirm(id: number, v: boolean) {
  const c = confirms.find(x => x.id === id)
  if (c) c.resolve(v)
  confirms = confirms.filter(x => x.id !== id)
  emit()
}

// promptAsync：应用内输入弹窗（替代原生 prompt，移动端/无头环境更可靠）
export function promptAsync(
  msg: string,
  opts: { defaultValue?: string; password?: boolean; inputType?: 'text' | 'number'; placeholder?: string } = {},
): Promise<string | null> {
  return new Promise(resolve => {
    const p: PromptItem = {
      id: seq++,
      msg,
      defaultValue: opts.defaultValue ?? '',
      password: !!opts.password,
      inputType: opts.inputType || 'text',
      placeholder: opts.placeholder,
      resolve,
    }
    prompts = [...prompts, p]
    emit()
  })
}

export function resolvePrompt(id: number, v: string | null) {
  const p = prompts.find(x => x.id === id)
  if (p) p.resolve(v)
  prompts = prompts.filter(x => x.id !== id)
  emit()
}
