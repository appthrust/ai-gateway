import { useText } from '../../i18n'
import { PLACE_MESSAGE, type DashboardPlace } from '../../model/next-steps'
import { cn, ExternalAnchor } from '../../ui'

/**
 * A fix that lives in the Dashboard: the action as a link when the Dashboard origin is known, else as text; the
 * Dashboard place is always named, so the person can find it either way.
 */
export function FixLink({ text, place, href, className }: { text: string; place: DashboardPlace; href: string | undefined; className?: string }) {
  const { t } = useText()
  return (
    <span className={cn('inline-flex flex-wrap items-baseline gap-x-2 gap-y-0.5', className)}>
      {href ? <ExternalAnchor href={href}>{text}</ExternalAnchor> : <span className="font-medium text-stone-800">{text}</span>}
      <span className="text-xs text-stone-500">{t(PLACE_MESSAGE[place])}</span>
    </span>
  )
}
