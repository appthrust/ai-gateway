import { AlertTriangle, CheckCircle2, Info, Loader2, RotateCw, XCircle } from 'lucide-react'
import type { ReactNode } from 'react'
import { isGatewayError } from '../api/client'
import { useText } from '../i18n'
import { Button } from './button'
import { cn } from './cn'

export type NoticeTone = 'info' | 'success' | 'warning' | 'error'

const NOTICE: Record<NoticeTone, { box: string; icon: ReactNode }> = {
  info: { box: 'border-stone-200 bg-white text-stone-700', icon: <Info className="text-accent" /> },
  success: { box: 'border-accent-line bg-accent-soft text-accent-strong', icon: <CheckCircle2 /> },
  warning: { box: 'border-amber-200 bg-busy-soft text-busy-ink', icon: <AlertTriangle /> },
  error: { box: 'border-red-200 bg-bad-soft text-bad-ink', icon: <XCircle /> },
}

export function Notice({
  tone = 'info',
  title,
  children,
  actions,
  className,
}: {
  tone?: NoticeTone
  title?: ReactNode
  children?: ReactNode
  actions?: ReactNode
  className?: string
}) {
  const style = NOTICE[tone]
  return (
    <div
      role={tone === 'error' ? 'alert' : 'status'}
      className={cn('flex gap-3 rounded-lg border px-4 py-3 text-sm [&>svg]:mt-0.5 [&>svg]:size-4 [&>svg]:shrink-0', style.box, className)}
    >
      {style.icon}
      <div className="min-w-0 flex-1">
        {title ? <p className="font-medium">{title}</p> : null}
        {children ? <div className={cn(title ? 'mt-0.5 opacity-90' : undefined)}>{children}</div> : null}
        {actions ? <div className="mt-3 flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
    </div>
  )
}

export function EmptyState({ icon, title, body, action, heading = 'p' }: { icon?: ReactNode; title: ReactNode; body?: ReactNode; action?: ReactNode; heading?: 'p' | 'h1' }) {
  const Heading = heading
  return (
    <div className="flex flex-col items-center rounded-lg border border-dashed border-stone-300 bg-white px-6 py-12 text-center">
      {icon ? (
        <span aria-hidden className="mb-3 flex size-10 items-center justify-center rounded-full bg-stone-100 text-stone-500 [&_svg]:size-5">
          {icon}
        </span>
      ) : null}
      <Heading className={cn('font-medium text-stone-800', heading === 'h1' ? 'text-xl' : 'text-sm')}>{title}</Heading>
      {body ? <div className="mt-1 max-w-sm text-sm text-stone-500">{body}</div> : null}
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  )
}

export function Loading({ className }: { className?: string }) {
  const { t } = useText()
  return (
    <p role="status" className={cn('flex items-center gap-2 py-6 text-sm text-stone-500', className)}>
      <Loader2 aria-hidden className="size-4 animate-spin text-accent" />
      {t('aiGateway.common.loading')}
    </p>
  )
}

/** A 401 cannot be fixed by retrying, so it gets the access-denied text and no retry button. */
export function LoadFailed({ error, onRetry }: { error?: unknown; onRetry: () => void }) {
  const { t } = useText()
  if (isGatewayError(error) && error.status === 401) return <Notice tone="error">{t('aiGateway.common.accessDenied')}</Notice>
  return (
    <Notice
      tone="error"
      title={t('aiGateway.common.loadFailed')}
      actions={
        <Button size="sm" icon={<RotateCw />} onClick={onRetry}>
          {t('aiGateway.common.retry')}
        </Button>
      }
    >
      {isGatewayError(error) ? (error.transient ? t('aiGateway.errors.transient') : t('aiGateway.errors.code', { code: error.code })) : null}
    </Notice>
  )
}
