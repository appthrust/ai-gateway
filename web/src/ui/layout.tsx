import { useId, type ReactNode } from 'react'
import { cn } from './cn'

/** Screen title, optional lead and right-aligned actions; `children` renders under the title row. */
export function PageHeader({ title, lead, actions, children }: { title: ReactNode; lead?: ReactNode; actions?: ReactNode; children?: ReactNode }) {
  return (
    <header className="mb-6 sm:mb-8">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-x-4">
        <div className="min-w-0 sm:flex-1">
          <h1 className="text-xl font-semibold tracking-tight break-words text-stone-900 sm:text-2xl">{title}</h1>
          {lead ? <div className="mt-1.5 max-w-prose text-sm text-stone-500">{lead}</div> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {children ? <div className="mt-4">{children}</div> : null}
    </header>
  )
}

/** A titled block inside a page; sections are separated by whitespace, not boxes. */
export function Section({ title, lead, actions, className, children }: { title: ReactNode; lead?: ReactNode; actions?: ReactNode; className?: string; children: ReactNode }) {
  const headingId = useId()
  return (
    <section aria-labelledby={headingId} className={cn('mt-8 first:mt-0', className)}>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h2 id={headingId} className="text-base font-semibold text-stone-900">
            {title}
          </h2>
          {lead ? <p className="mt-0.5 text-sm text-stone-500">{lead}</p> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {children}
    </section>
  )
}
