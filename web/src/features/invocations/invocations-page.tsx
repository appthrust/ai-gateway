import { formatDashboardDateTime } from '@appthrust/platform-dashboard-i18n'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ChevronDown, ChevronRight, Inbox } from 'lucide-react'
import { Fragment, useState, type ReactNode } from 'react'
import { configQuery, invocationsQuery, providersQuery } from '../../api/queries'
import type { InvocationEntry, InvocationPage, ProviderView } from '../../api/types'
import { useText } from '../../i18n'
import { dashboardLinks } from '../../model/dashboard'
import { explainError } from '../../model/error-codes'
import { formatDuration, formatTime, shortDigest } from '../../model/format'
import { hasFilters, OUTCOMES, toInvocationQuery, type InvocationsSearch } from '../../model/invocations'
import { paths } from '../../routes/paths'
import { Button, Card, EmptyState, ExternalAnchor, Field, PageHeader, SegmentedControl, Select, useDocumentTitle } from '../../ui'
import { OutcomeBadge, ReportState } from '../common/invocation'

// Radix Select values must be non-empty: this stands for "no filter".
const ANY = '*'

export function InvocationsPage({ search }: { search: InvocationsSearch }) {
  const { t, language } = useText()
  const { data: page } = useSuspenseQuery(invocationsQuery(toInvocationQuery(search)))
  const { data: config } = useSuspenseQuery(configQuery)
  const providers = useQuery(providersQuery).data
  const links = dashboardLinks(config)
  const navigate = useNavigate()
  useDocumentTitle(t('aiGateway.nav.invocations'))
  const apply = (next: InvocationsSearch) => void navigate({ to: paths.invocations, search: next as never, replace: true })
  return (
    <>
      <PageHeader
        title={t('aiGateway.nav.invocations')}
        lead={
          <>
            <span>
              {page.instance
                ? t('aiGateway.invocations.scope', { since: formatTime(language, page.since), instance: page.instance, capacity: page.capacity })
                : t('aiGateway.invocations.scopeNoInstance', { since: formatTime(language, page.since), capacity: page.capacity })}
            </span>{' '}
            {links.audit ? (
              <ExternalAnchor href={links.audit}>{t('aiGateway.invocations.audit')}</ExternalAnchor>
            ) : (
              <span>{t('aiGateway.invocations.auditText')}</span>
            )}
          </>
        }
      />
      <Filters search={search} page={page} providers={providers} onChange={apply} />
      {page.items.length === 0 ? (
        hasFilters(search) ? (
          <EmptyState
            icon={<Inbox />}
            title={t('aiGateway.invocations.noMatch.title')}
            body={t('aiGateway.invocations.noMatch.body')}
            action={<Button onClick={() => apply({})}>{t('aiGateway.invocations.clear')}</Button>}
          />
        ) : (
          <EmptyState icon={<Inbox />} title={t('aiGateway.invocations.empty.title')} body={t('aiGateway.invocations.empty.body')} />
        )
      ) : (
        <InvocationTable items={page.items} />
      )}
    </>
  )
}

function Filters({ search, page, providers, onChange }: { search: InvocationsSearch; page: InvocationPage; providers: ProviderView[] | undefined; onChange: (next: InvocationsSearch) => void }) {
  const { t } = useText()
  // Choices: every provider and model the providers view knows, plus what the shown records name and the current filter.
  const providerNames = unique([...(providers ?? []).map((provider) => provider.name), ...page.items.map((item) => item.provider), search.provider])
  const modelIds = unique([
    ...(providers ?? []).filter((provider) => !search.provider || provider.name === search.provider).flatMap((provider) => provider.models.map((model) => model.id)),
    ...page.items.map((item) => item.model),
    search.model,
  ])
  const set = (patch: Partial<InvocationsSearch>) => {
    const next: InvocationsSearch = { ...search, ...patch }
    for (const key of Object.keys(next) as (keyof InvocationsSearch)[]) if (!next[key]) delete next[key]
    onChange(next)
  }
  return (
    <div className="mb-4 flex flex-wrap items-end gap-3">
      <Field id="filter-outcome" label={t('aiGateway.field.outcome')} className="w-40">
        <Select
          id="filter-outcome"
          value={search.outcome ?? ANY}
          onChange={(value) => set({ outcome: value === ANY ? undefined : OUTCOMES.find((outcome) => outcome === value) })}
          options={[{ value: ANY, label: t('aiGateway.invocations.any') }, ...OUTCOMES.map((outcome) => ({ value: outcome, label: t(`aiGateway.outcome.${outcome}`) }))]}
        />
      </Field>
      <Field id="filter-provider" label={t('aiGateway.field.provider')} className="w-44">
        <Select
          id="filter-provider"
          value={search.provider ?? ANY}
          onChange={(value) => set({ provider: value === ANY ? undefined : value })}
          options={[{ value: ANY, label: t('aiGateway.invocations.any') }, ...providerNames.map((name) => ({ value: name, label: name }))]}
        />
      </Field>
      <Field id="filter-model" label={t('aiGateway.field.model')} className="w-52">
        <Select
          id="filter-model"
          value={search.model ?? ANY}
          onChange={(value) => set({ model: value === ANY ? undefined : value })}
          options={[{ value: ANY, label: t('aiGateway.invocations.any') }, ...modelIds.map((id) => ({ value: id, label: id }))]}
        />
      </Field>
      <div>
        <p className="mb-1.5 text-sm font-medium text-stone-800">{t('aiGateway.field.caller')}</p>
        <SegmentedControl
          label={t('aiGateway.field.caller')}
          value={search.caller ?? 'all'}
          onChange={(value) => set({ caller: value === 'me' ? 'me' : undefined })}
          options={[
            { value: 'all', label: t('aiGateway.invocations.caller.all') },
            { value: 'me', label: t('aiGateway.invocations.caller.me') },
          ]}
        />
      </div>
      {hasFilters(search) ? (
        <Button tone="ghost" onClick={() => onChange({})}>
          {t('aiGateway.invocations.clear')}
        </Button>
      ) : null}
    </div>
  )
}

const unique = (values: (string | undefined)[]) => [...new Set(values.filter((value): value is string => Boolean(value)))].sort()

export function InvocationTable({ items }: { items: readonly InvocationEntry[] }) {
  const { t, language, locale } = useText()
  const [open, setOpen] = useState<string>()
  return (
    <Card className="overflow-x-auto">
      <table className="w-full min-w-[60rem] text-left text-sm">
        <caption className="sr-only">{t('aiGateway.nav.invocations')}</caption>
        <thead className="border-b border-line bg-canvas text-xs whitespace-nowrap text-stone-500">
          <tr>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.time')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.caller')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.route')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.provider')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.model')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.outcome')}</th>
            <th scope="col" className="px-2 py-2.5 text-right font-medium">{t('aiGateway.field.status')}</th>
            <th scope="col" className="px-2 py-2.5 text-right font-medium">{t('aiGateway.field.duration')}</th>
            <th scope="col" className="px-2 py-2.5 text-right font-medium whitespace-normal">{t('aiGateway.field.tokensInOut')}</th>
            <th scope="col" className="px-2 py-2.5 font-medium">{t('aiGateway.field.audit')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {items.map((item) => {
            const expanded = open === item.id
            const detailsId = `invocation-${item.id}`
            return (
              <Fragment key={item.id}>
                <tr className="align-top">
                  <th scope="row" className="px-2 py-2.5 font-normal whitespace-nowrap">
                    <button
                      type="button"
                      aria-expanded={expanded}
                      aria-controls={detailsId}
                      onClick={() => setOpen(expanded ? undefined : item.id)}
                      className="-mx-1 inline-flex min-h-8 items-center gap-1 rounded px-1 text-stone-800 hover:bg-stone-100"
                    >
                      {expanded ? <ChevronDown aria-hidden className="size-4 text-stone-500" /> : <ChevronRight aria-hidden className="size-4 text-stone-500" />}
                      <time dateTime={item.time} title={formatTime(language, item.time)}>
                        {formatDashboardDateTime(language, item.time, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                      </time>
                      <span className="sr-only">{t(expanded ? 'aiGateway.invocations.hideDetail' : 'aiGateway.invocations.showDetail')}</span>
                    </button>
                  </th>
                  <td className="max-w-32 truncate px-2 py-2.5 text-stone-700" title={item.caller.subject}>
                    {item.caller.displayName || <span className="text-stone-500">{t('aiGateway.invocations.unnamed')}</span>}
                  </td>
                  <td className="px-2 py-2.5 text-stone-700">
                    {t(`aiGateway.route.${item.route}`)}
                    {item.stream ? <span className="block text-xs text-stone-500">{t('aiGateway.invocations.stream')}</span> : null}
                  </td>
                  <td className="px-2 py-2.5 font-mono text-xs whitespace-nowrap text-stone-800">{item.provider || '—'}</td>
                  <td className="px-2 py-2.5 font-mono text-xs whitespace-nowrap text-stone-800">{item.model || '—'}</td>
                  {/* The error code and the drop reason are in the row detail; the title keeps the code a hover away. */}
                  <td className="px-2 py-2.5" title={item.reason || undefined}>
                    <OutcomeBadge outcome={item.outcome} />
                  </td>
                  <td className="px-2 py-2.5 text-right text-stone-700 tabular-nums">{item.status || '—'}</td>
                  <td className="px-2 py-2.5 text-right whitespace-nowrap text-stone-700 tabular-nums">{formatDuration(locale, item.durationMs)}</td>
                  <td className="px-2 py-2.5 text-right whitespace-nowrap text-stone-700 tabular-nums">
                    {item.promptTokens !== undefined || item.completionTokens !== undefined ? `${item.promptTokens ?? '—'} / ${item.completionTokens ?? '—'}` : '—'}
                  </td>
                  <td className="px-2 py-2.5">
                    <ReportState report={item.report} compact />
                  </td>
                </tr>
                {expanded ? (
                  <tr id={detailsId} className="bg-canvas/60">
                    <td colSpan={10} className="px-4 py-3">
                      <InvocationDetail item={item} />
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </Card>
  )
}

function InvocationDetail({ item }: { item: InvocationEntry }) {
  const { t } = useText()
  const explanation = item.reason ? explainError(item.reason) : undefined
  const row = (label: string, value: ReactNode) => (
    <div className="flex gap-3 py-1">
      <dt className="w-36 shrink-0 text-stone-500">{label}</dt>
      <dd className="min-w-0 break-all text-stone-800">{value}</dd>
    </div>
  )
  return (
    <div className="grid gap-x-8 text-sm md:grid-cols-2">
      <dl>
        {row(t('aiGateway.field.requestDigest'), item.requestDigest ? <span className="font-mono text-xs" title={item.requestDigest}>{shortDigest(item.requestDigest)}</span> : t('aiGateway.invocations.noDigest'))}
        {row(t('aiGateway.field.completionDigest'), item.completionDigest ? <span className="font-mono text-xs" title={item.completionDigest}>{shortDigest(item.completionDigest)}</span> : t('aiGateway.invocations.noDigest'))}
        {row(t('aiGateway.field.policy'), item.policy ? <span className="font-mono text-xs">{item.policy.name}</span> : t('aiGateway.invocations.noPolicy'))}
      </dl>
      <dl>
        {row(t('aiGateway.field.report'), <ReportState report={item.report} />)}
        {row(t('aiGateway.field.id'), <span className="font-mono text-xs">{item.id}</span>)}
        {explanation ? (
          row(
            t('aiGateway.field.reason'),
            <span>
              <span className="font-mono text-xs">{item.reason}</span> — {t(explanation.title, { code: item.reason, status: item.status, provider: item.provider || '—', model: item.model || '—', rule: '—' })}
            </span>
          )
        ) : null}
      </dl>
    </div>
  )
}
