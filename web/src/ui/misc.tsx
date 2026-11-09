import { Check, Copy, ExternalLink } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useText } from '../i18n'
import { Button } from './button'
import { cn } from './cn'

/** Copies `value` to the clipboard and confirms in place (announced politely). `label` names what is copied for screen readers. */
export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useText()
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(timer)
  }, [copied])
  return (
    <Button
      size="sm"
      icon={copied ? <Check className="text-accent" /> : <Copy />}
      aria-label={label ? `${t('aiGateway.common.copy')}: ${label}` : undefined}
      onClick={() => void navigator.clipboard.writeText(value).then(() => setCopied(true))}
    >
      <span aria-live="polite">{copied ? t('aiGateway.common.copied') : t('aiGateway.common.copy')}</span>
    </Button>
  )
}

export type Tone = 'ok' | 'busy' | 'bad' | 'neutral' | 'accent'

const TONE: Record<Tone, string> = {
  ok: 'bg-ok-soft text-accent-strong ring-accent-line',
  accent: 'bg-accent-soft text-accent-strong ring-accent-line',
  busy: 'bg-busy-soft text-busy-ink ring-amber-200',
  bad: 'bg-bad-soft text-bad-ink ring-red-200',
  neutral: 'bg-stone-100 text-stone-600 ring-stone-200',
}

const DOT: Record<Tone, string> = { ok: 'bg-ok', accent: 'bg-accent', busy: 'bg-busy', bad: 'bg-bad', neutral: 'bg-stone-400' }

/** Small label; status tones carry a dot so the state never depends on color alone (the text names it). */
export function Badge({ children, tone = 'neutral', title, dot = false }: { children: ReactNode; tone?: Tone; title?: string; dot?: boolean }) {
  return (
    <span
      title={title}
      className={cn('inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium whitespace-nowrap ring-1 ring-inset', TONE[tone])}
    >
      {dot ? <span aria-hidden className={cn('size-1.5 rounded-full', DOT[tone])} /> : null}
      {children}
    </span>
  )
}

/** A link out of the app (the Dashboard owns the change); opens in the same tab, marked with an icon. */
export function ExternalAnchor({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  return (
    <a href={href} rel="noreferrer" className={cn('inline-flex items-center gap-1 font-medium text-accent-strong underline-offset-2 hover:underline', className)}>
      {children}
      <ExternalLink aria-hidden className="size-3.5 shrink-0" />
    </a>
  )
}

/** Monospace value with a copy button (a base URL, a digest). */
export function CopyField({ value, label }: { value: string; label: string }) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <code className="min-w-0 flex-1 truncate rounded-md border border-line bg-canvas px-2.5 py-1.5 font-mono text-xs text-stone-800" title={value}>
        {value}
      </code>
      <CopyButton value={value} label={label} />
    </div>
  )
}

/** A copyable code sample. */
export function CodeBlock({ code, label }: { code: string; label: string }) {
  return (
    <div className="relative">
      <pre className="overflow-x-auto rounded-lg border border-line bg-stone-900 px-4 py-3 pr-24 font-mono text-xs leading-relaxed text-stone-100">
        <code>{code}</code>
      </pre>
      <div className="absolute top-2 right-2">
        <CopyButton value={code} label={label} />
      </div>
    </div>
  )
}

/** A bordered white block (status, cards, panels). */
export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('rounded-lg border border-line bg-white', className)}>{children}</div>
}
