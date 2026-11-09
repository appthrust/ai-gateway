import { useSuspenseQuery } from '@tanstack/react-query'
import { ArrowRight, CheckCircle2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { configQuery, statusQuery } from '../../api/queries'
import type { GatewayStatus, HealthState, Mode, RouteSource } from '../../api/types'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { dashboardLinks, type DashboardLinks } from '../../model/dashboard'
import { formatAge, formatTime, shortDigest } from '../../model/format'
import { nextStepRows } from '../../model/next-steps'
import { Badge, Card, CopyField, HrefLink, PageHeader, Section, useDocumentTitle, type Tone } from '../../ui'
import { paths } from '../../routes/paths'
import { FixLink } from '../common/fix-link'

const MODE_TONE: Record<Mode, Tone> = { Unrestricted: 'neutral', Enforced: 'ok', Pending: 'busy' }
const HEALTH_TONE: Record<HealthState, Tone> = { Serving: 'ok', Degraded: 'busy', Failing: 'bad', Unknown: 'neutral' }
const SOURCE_MESSAGE: Record<RouteSource, MessageId> = {
  Policy: 'aiGateway.overview.route.source.Policy',
  EnvOverride: 'aiGateway.overview.route.source.EnvOverride',
  Capability: 'aiGateway.overview.route.source.Capability',
  Credential: 'aiGateway.overview.route.source.Credential',
}

export function OverviewPage() {
  const { t } = useText()
  const { data: status } = useSuspenseQuery(statusQuery)
  const { data: config } = useSuspenseQuery(configQuery)
  const links = dashboardLinks(config)
  useDocumentTitle(t('aiGateway.nav.overview'))
  return (
    <>
      <PageHeader title={t('aiGateway.nav.overview')} lead={t('aiGateway.overview.lead')} />
      <Section title={t('aiGateway.overview.status.title')}>
        <StatusStrip status={status} links={links} />
      </Section>
      {/* Each section sits in its own grid cell, so both are their cell's first child and start at the same height. */}
      <div className="mt-8 grid gap-6 md:grid-cols-2">
        <div>
          <Section title={t('aiGateway.overview.route.title')}>
            <DefaultRouteCard status={status} />
          </Section>
        </div>
        <div>
          <Section
            title={t('aiGateway.overview.endpoints.title')}
            actions={
              <HrefLink href={paths.connect} className="inline-flex items-center gap-1 text-sm font-medium text-accent-strong hover:underline">
                {t('aiGateway.overview.endpoints.connect')}
                <ArrowRight aria-hidden className="size-3.5" />
              </HrefLink>
            }
          >
            <Card className="space-y-3 p-4">
              <EndpointRow label={t('aiGateway.endpoints.public')} hint={t('aiGateway.endpoints.publicHint')} value={status.endpoints.public} />
              <EndpointRow label={t('aiGateway.endpoints.internal')} hint={t('aiGateway.endpoints.internalHint')} value={status.endpoints.internal} />
            </Card>
          </Section>
        </div>
      </div>
      <Section title={t('aiGateway.overview.next.title')} lead={t('aiGateway.overview.next.lead')}>
        <NextSteps status={status} links={links} />
      </Section>
    </>
  )
}

function StatusRow({ label, badge, children }: { label: string; badge: ReactNode; children: ReactNode }) {
  return (
    <div className="grid gap-x-4 gap-y-1 px-4 py-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
      <dt className="text-sm font-medium text-stone-500">{label}</dt>
      <dd className="flex min-w-0 flex-col gap-1.5 text-sm text-stone-700 sm:flex-row sm:items-start sm:gap-3">
        <span className="shrink-0">{badge}</span>
        <div className="min-w-0 flex-1 space-y-1">{children}</div>
      </dd>
    </div>
  )
}

function StatusStrip({ status, links }: { status: GatewayStatus; links: DashboardLinks }) {
  const { t, language, locale } = useText()
  const satisfiedBy = status.configured.candidates.find((candidate) => candidate.satisfied)
  const { health } = status
  return (
    <Card>
      <dl className="divide-y divide-line">
        <StatusRow
          label={t('aiGateway.field.policy')}
          badge={
            <Badge tone={MODE_TONE[status.mode]} dot>
              {status.mode === 'Enforced' && status.policy ? t('aiGateway.mode.enforcedWith', { policy: status.policy.name }) : t(`aiGateway.mode.${status.mode}`)}
            </Badge>
          }
        >
          <p>
            {status.mode === 'Enforced' && status.policy
              ? t('aiGateway.overview.policy.Enforced', { policy: status.policy.name, generation: status.policy.generation ?? '—' })
              : t(`aiGateway.overview.policy.${status.mode}`)}
          </p>
          {status.logging ? (
            <p className="text-xs text-stone-500">
              {t('aiGateway.overview.policy.logging', { conversation: status.logging.conversation, toolCalls: status.logging.toolCalls, retention: status.logging.retention })}
            </p>
          ) : null}
          <FixLink text={t('aiGateway.overview.policy.manage')} place="settings" href={links.settings} />
        </StatusRow>
        <StatusRow
          label={t('aiGateway.overview.status.configured')}
          badge={
            <Badge tone={status.configured.satisfied ? 'ok' : 'busy'} dot>
              {t(status.configured.satisfied ? 'aiGateway.configured.yes' : 'aiGateway.configured.no')}
            </Badge>
          }
        >
          {status.configured.satisfied && satisfiedBy ? (
            <p>{t(satisfiedBy.kind === 'Link' ? 'aiGateway.overview.configured.byLink' : 'aiGateway.overview.configured.byCredential', { name: satisfiedBy.name })}</p>
          ) : (
            <>
              <p>{t('aiGateway.overview.configured.anyOf')}</p>
              <ul className="space-y-1">
                {status.configured.candidates.map((candidate) => (
                  <li key={`${candidate.kind}/${candidate.name}`} className="flex gap-2">
                    <span aria-hidden className="text-stone-400">
                      ・
                    </span>
                    <FixLink
                      text={t(candidate.kind === 'Link' ? 'aiGateway.overview.configured.installLink' : 'aiGateway.overview.configured.bindCredential', { name: candidate.name })}
                      place="installation"
                      href={links.installation}
                    />
                  </li>
                ))}
              </ul>
            </>
          )}
        </StatusRow>
        <StatusRow
          label={t('aiGateway.overview.status.health')}
          badge={
            <Badge tone={HEALTH_TONE[health.state]} dot>
              {health.reason ? `${t(`aiGateway.health.${health.state}`)} · ${health.reason}` : t(`aiGateway.health.${health.state}`)}
            </Badge>
          }
        >
          <p>{health.message || t(`aiGateway.overview.health.${health.state}`)}</p>
          {health.observedAt ? (
            <p className="text-xs text-stone-500">
              <time dateTime={health.observedAt} title={formatTime(language, health.observedAt)}>
                {t('aiGateway.common.observed', { age: formatAge(locale, health.observedAt) })}
              </time>
            </p>
          ) : null}
        </StatusRow>
        <StatusRow label={t('aiGateway.overview.status.release')} badge={<Badge tone="neutral">{status.release.version ? `v${status.release.version.replace(/^v/, '')}` : '—'}</Badge>}>
          <p className="font-mono text-xs text-stone-600" title={status.release.artifactDigest}>
            {status.release.artifactDigest ? shortDigest(status.release.artifactDigest) : t('aiGateway.overview.release.noDigest')}
          </p>
        </StatusRow>
      </dl>
    </Card>
  )
}

function DefaultRouteCard({ status }: { status: GatewayStatus }) {
  const { t } = useText()
  const route = status.defaultRoute
  return (
    <Card className="p-4">
      {route ? (
        <>
          <p className="text-sm text-stone-700">{t('aiGateway.overview.route.lead')}</p>
          <p className="mt-2 font-mono text-base font-semibold text-stone-900">
            {route.model ? `${route.provider} / ${route.model}` : t('aiGateway.overview.route.providerDefault', { provider: route.provider })}
          </p>
          <p className="mt-1 text-xs text-stone-500">{t(SOURCE_MESSAGE[route.source])}</p>
        </>
      ) : (
        <>
          <p className="font-medium text-stone-900">{t('aiGateway.overview.route.none')}</p>
          <p className="mt-1 text-sm text-stone-500">{t('aiGateway.overview.route.noneHint')}</p>
        </>
      )}
    </Card>
  )
}

function EndpointRow({ label, hint, value }: { label: string; hint: string; value: string | null }) {
  const { t } = useText()
  return (
    <div>
      <p className="text-sm font-medium text-stone-800">{label}</p>
      <p className="mb-1.5 text-xs text-stone-500">{hint}</p>
      {value ? <CopyField value={value} label={label} /> : <p className="text-sm text-stone-500">{t('aiGateway.endpoints.internalUnknown')}</p>}
    </div>
  )
}

/** The derived steps, numbered, each a Dashboard deep link (or the Dashboard place as text without an origin). */
export function NextSteps({ status, links }: { status: Pick<GatewayStatus, 'nextSteps'>; links: DashboardLinks }) {
  const { t } = useText()
  const rows = nextStepRows(status.nextSteps, links)
  if (rows.length === 0) {
    return (
      <Card className="flex items-center gap-3 p-4 text-sm text-stone-700">
        <CheckCircle2 aria-hidden className="size-5 shrink-0 text-accent" />
        {t('aiGateway.overview.next.none')}
      </Card>
    )
  }
  return (
    <Card>
      <ol className="divide-y divide-line">
        {rows.map((row, index) => (
          <li key={row.key} className="flex gap-3 px-4 py-3">
            <span aria-hidden className="grid size-6 shrink-0 place-items-center rounded-full bg-accent-soft text-xs font-semibold text-accent-strong">
              {index + 1}
            </span>
            <FixLink text={t(row.message, row.values)} place={row.place} href={row.href} className="pt-0.5" />
          </li>
        ))}
      </ol>
    </Card>
  )
}
