import type { InvocationEntry, Outcome } from '../../api/types'
import { useText } from '../../i18n'
import { Badge, type Tone } from '../../ui'

const OUTCOME_TONE: Record<Outcome, Tone> = { Allowed: 'ok', Denied: 'busy', Error: 'bad' }

export function OutcomeBadge({ outcome }: { outcome: Outcome }) {
  const { t } = useText()
  return (
    <Badge tone={OUTCOME_TONE[outcome]} dot>
      {t(`aiGateway.outcome.${outcome}`)}
    </Badge>
  )
}

/** The reporter's delivery of the record to the Project audit: pending, delivered, or dropped with its reason (`compact`: badge only). */
export function ReportState({ report, compact = false }: { report: InvocationEntry['report']; compact?: boolean }) {
  const { t } = useText()
  if (report.state === 'Dropped') {
    const reason = report.status ? t('aiGateway.report.droppedStatus', { reason: report.reason, status: report.status }) : t('aiGateway.report.droppedReason', { reason: report.reason })
    return (
      <span title={compact ? reason : undefined}>
        <Badge tone="bad" dot>
          {t('aiGateway.report.Dropped')}
        </Badge>
        {compact ? null : <span className="mt-0.5 block text-xs text-stone-500">{reason}</span>}
      </span>
    )
  }
  return (
    <Badge tone={report.state === 'Delivered' ? 'ok' : 'neutral'} dot>
      {t(`aiGateway.report.${report.state}`)}
    </Badge>
  )
}
