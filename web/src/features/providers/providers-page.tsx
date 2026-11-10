import { useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react'
import { Fragment, useEffect, useState } from 'react'
import { gatewayApi, isGatewayError } from '../../api/client'
import { configQuery, providersQuery, statusQuery } from '../../api/queries'
import type { CredentialKeyView, ProviderState, ProviderView } from '../../api/types'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { dashboardLinks, type DashboardLinks } from '../../model/dashboard'
import { formatAge, formatTime } from '../../model/format'
import { isStaticProvider, providerAction, PROVIDER_STATE_MESSAGE, PROVIDER_STATE_TONE } from '../../model/providers'
import { Badge, Button, Card, EmptyState, PageHeader, Section, useDocumentTitle } from '../../ui'
import { FixLink } from '../common/fix-link'


const LEGEND: { state: ProviderState; meaning: MessageId }[] = [
  { state: 'Serving', meaning: 'aiGateway.providers.legend.Serving' },
  { state: 'Denied', meaning: 'aiGateway.providers.legend.Denied' },
  { state: 'Unreachable', meaning: 'aiGateway.providers.legend.Unreachable' },
  { state: 'NotConfigured', meaning: 'aiGateway.providers.legend.NotConfigured' },
  { state: 'PolicyExcluded', meaning: 'aiGateway.providers.legend.PolicyExcluded' },
  { state: 'NotInstalled', meaning: 'aiGateway.providers.legend.NotInstalled' },
  { state: 'Unknown', meaning: 'aiGateway.providers.legend.Unknown' },
]

export function ProvidersPage() {
  const { t } = useText()
  const { data: providers } = useSuspenseQuery(providersQuery)
  const { data: config } = useSuspenseQuery(configQuery)
  const links = dashboardLinks(config)
  const [open, setOpen] = useState<Record<string, boolean>>({})
  useDocumentTitle(t('aiGateway.nav.providers'))
  return (
    <>
      <PageHeader title={t('aiGateway.nav.providers')} lead={t('aiGateway.providers.lead')} />
      <CredentialKeys providers={providers} links={links} />
      {providers.length === 0 ? (
        <EmptyState title={t('aiGateway.providers.empty.title')} body={t('aiGateway.providers.empty.body')} />
      ) : (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[52rem] text-left text-sm">
            <caption className="sr-only">{t('aiGateway.nav.providers')}</caption>
            <thead className="border-b border-line bg-canvas text-xs whitespace-nowrap text-stone-500">
              <tr>
                <th scope="col" className="px-4 py-2.5 font-medium">{t('aiGateway.field.provider')}</th>
                <th scope="col" className="px-4 py-2.5 font-medium">{t('aiGateway.providers.col.kind')}</th>
                <th scope="col" className="px-4 py-2.5 font-medium">{t('aiGateway.providers.col.source')}</th>
                <th scope="col" className="px-4 py-2.5 font-medium">{t('aiGateway.providers.col.state')}</th>
                <th scope="col" className="px-4 py-2.5 text-right font-medium">{t('aiGateway.providers.col.models')}</th>
                <th scope="col" className="px-4 py-2.5 font-medium">{t('aiGateway.providers.col.checked')}</th>
                <th scope="col" className="px-4 py-2.5 font-medium">
                  <span className="sr-only">{t('aiGateway.providers.col.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {providers.map((provider) => (
                <ProviderRows
                  key={provider.name}
                  provider={provider}
                  links={links}
                  open={open[provider.name] ?? false}
                  onToggle={() => setOpen((current) => ({ ...current, [provider.name]: !current[provider.name] }))}
                />
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <Section title={t('aiGateway.providers.legend.title')}>
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
          {LEGEND.map((entry) => (
            <div key={entry.state} className="flex items-start gap-2">
              <dt className="shrink-0">
                <Badge tone={PROVIDER_STATE_TONE[entry.state]} dot>
                  {t(PROVIDER_STATE_MESSAGE[entry.state])}
                </Badge>
              </dt>
              <dd className="text-stone-600">{t(entry.meaning)}</dd>
            </div>
          ))}
        </dl>
      </Section>
    </>
  )
}
export function CredentialKeys({ providers, links }: { providers: ProviderView[]; links: DashboardLinks }) {
  const { t, language } = useText()
  return (
    <Section title={t('aiGateway.keys.title')}>
      <p className="mb-3 text-sm text-stone-600">{t('aiGateway.keys.description')}</p>
      <FixLink text={t('aiGateway.keys.manage')} place="installation" href={links.installation} />
      <div className="mt-3 overflow-x-auto">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">{t('aiGateway.keys.title')}</caption>
          <thead className="border-b border-line text-xs text-stone-500">
            <tr>
              <th scope="col" className="p-3">{t('aiGateway.field.provider')}</th>
              <th scope="col" className="p-3">{t('aiGateway.keys.slot')}</th>
              <th scope="col" className="p-3">{t('aiGateway.providers.col.state')}</th>
              <th scope="col" className="p-3">{t('aiGateway.providers.col.checked')}</th>
              <th scope="col" className="p-3">{t('aiGateway.providers.col.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {providers.flatMap((provider) => provider.keys.map((key) => (
              <tr key={key.slot}>
                <td className="p-3">{provider.kind}</td>
                <th scope="row" className="p-3 font-mono font-normal">{key.slot}</th>
                <td className="p-3">
                  <Badge tone={key.state === 'usable' ? 'ok' : key.state === 'unchecked' ? 'neutral' : 'bad'}>
                    {t(`aiGateway.keys.state.${key.state}`)}
                  </Badge>
                  {!key.bound ? <span className="ml-2 text-xs text-stone-500">{t('aiGateway.providers.source.unbound')}</span> : null}
                </td>
                <td className="p-3 whitespace-nowrap">
                  {key.observedAt ? <time dateTime={key.observedAt}>{formatTime(language, key.observedAt)}</time> : t('aiGateway.providers.neverChecked')}
                </td>
                <td className="p-3">{key.bound ? <RecheckButton provider={provider} credentialKey={key} /> : null}</td>
              </tr>
            )))}
          </tbody>
        </table>
      </div>
    </Section>
  )
}


function ProviderRows({ provider, links, open, onToggle }: { provider: ProviderView; links: DashboardLinks; open: boolean; onToggle: () => void }) {
  const { t, language, locale } = useText()
  const action = providerAction(provider, links)
  const detailsId = `provider-${provider.name}-models`
  const allowed = provider.models.filter((model) => model.allowed).length
  return (
    <Fragment>
      <tr className="align-top">
        <th scope="row" className="px-4 py-3 font-medium">
          <button
            type="button"
            aria-expanded={open}
            aria-controls={detailsId}
            onClick={onToggle}
            className="-mx-1 inline-flex min-h-8 items-center gap-1 rounded px-1 font-mono text-stone-900 hover:bg-stone-100"
          >
            {open ? <ChevronDown aria-hidden className="size-4 text-stone-500" /> : <ChevronRight aria-hidden className="size-4 text-stone-500" />}
            {provider.name}
            <span className="sr-only">{t(open ? 'aiGateway.providers.hideModels' : 'aiGateway.providers.showModels')}</span>
          </button>
        </th>
        <td className="px-4 py-3 text-stone-700">{provider.kind === 'InProjectApp' ? t('aiGateway.providers.kind.InProjectApp') : provider.kind}</td>
        <td className="px-4 py-3 text-stone-700">
          {'link' in provider.source ? (
            <>
              <span>{t('aiGateway.providers.source.link', { link: provider.source.link })}</span>
              <span className="block text-xs text-stone-500">{provider.source.linkState}</span>
            </>
          ) : (
            <>
              <span>{t('aiGateway.providers.source.slot', { slot: provider.source.credentialSlot })}</span>
              <span className="block text-xs text-stone-500">{t(provider.source.bound ? 'aiGateway.providers.source.bound' : 'aiGateway.providers.source.unbound')}</span>
            </>
          )}
        </td>
        <td className="max-w-64 px-4 py-3">
          <Badge tone={PROVIDER_STATE_TONE[provider.state]} dot>
            {t(PROVIDER_STATE_MESSAGE[provider.state])}
          </Badge>
          {provider.reason || provider.message ? (
            <p className="mt-1 text-xs text-stone-500">
              {provider.reason ? <span className="font-mono">{provider.reason}</span> : null}
              {provider.reason && provider.message ? ' — ' : null}
              {provider.message}
            </p>
          ) : null}
        </td>
        <td className="px-4 py-3 text-right whitespace-nowrap text-stone-700 tabular-nums">
          {provider.models.length === 0 ? '—' : t('aiGateway.providers.modelCount', { allowed, total: provider.models.length })}
        </td>
        <td className="px-4 py-3 whitespace-nowrap text-stone-700">
          {provider.observedAt ? (
            <time dateTime={provider.observedAt} title={formatTime(language, provider.observedAt)}>
              {formatAge(locale, provider.observedAt)}
            </time>
          ) : (
            <span className="text-stone-500">{t('aiGateway.providers.neverChecked')}</span>
          )}
        </td>
        <td className="px-4 py-3">
          <div className="flex flex-col items-start gap-2">
            {action ? <FixLink text={t(action.message, action.values)} place={action.place} href={action.href} className="text-xs" /> : null}
            {isStaticProvider(provider) ? <RecheckButton provider={provider} /> : null}
          </div>
        </td>
      </tr>
      {open ? (
        <tr id={detailsId} className="bg-canvas/60">
          <td colSpan={7} className="px-4 py-3">
            <ModelList provider={provider} />
          </td>
        </tr>
      ) : null}
    </Fragment>
  )
}

function ModelList({ provider }: { provider: ProviderView }) {
  const { t } = useText()
  if (provider.models.length === 0) return <p className="text-sm text-stone-500">{t('aiGateway.providers.noModels')}</p>
  return (
    <div>
      <p className="mb-2 text-xs text-stone-500">{t(provider.embeddings ? 'aiGateway.providers.embeddings.yes' : 'aiGateway.providers.embeddings.no')}</p>
      <ul className="grid gap-1.5 sm:grid-cols-2">
        {provider.models.map((model) => (
          <li key={model.id} className="flex flex-wrap items-center gap-2 rounded-md border border-line bg-white px-3 py-2">
            <span className="font-mono text-xs text-stone-900">{model.id}</span>
            {model.default ? <Badge tone="accent">{t('aiGateway.models.default')}</Badge> : null}
            <Badge tone={model.allowed ? 'ok' : 'bad'} dot>
              {t(model.allowed ? 'aiGateway.models.allowed' : 'aiGateway.models.denied')}
            </Badge>
            {!model.allowed && model.deniedBy ? (
              <span className="w-full text-xs text-stone-500">
                {t('aiGateway.models.deniedBy', { provider: model.deniedBy.provider, patterns: model.deniedBy.patterns.join(', ') || '—' })}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}

/**
 * Forces a probe. A 429 means the last probe is younger than 30 s: the button says how old the shown result is and
 * when a new check is possible, instead of pretending to re-check.
 */
function RecheckButton({ provider, credentialKey }: { provider: ProviderView; credentialKey?: CredentialKeyView }) {
  const { t, locale } = useText()
  const client = useQueryClient()
  const [waitUntil, setWaitUntil] = useState<number>()
  const [now, setNow] = useState(() => Date.now())
  const check = useMutation({
    mutationFn: () => gatewayApi.check(provider.name, credentialKey?.slot),
    onSuccess: (view) => {
      setWaitUntil(undefined)
      client.setQueryData(providersQuery.queryKey, (current) => current?.map((item) => (item.name === view.name ? view : item)))
      void client.invalidateQueries({ queryKey: statusQuery.queryKey })
    },
    onError: (error) => {
      if (isGatewayError(error, 'ai_check_rate_limited')) setWaitUntil(Date.now() + (error.retryAfterSeconds ?? 30) * 1000)
    },
  })
  const waiting = waitUntil !== undefined && waitUntil > now
  useEffect(() => {
    if (!waiting) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [waiting])
  const failed = check.isError && !isGatewayError(check.error, 'ai_check_rate_limited')
  return (
    <div className="flex flex-col items-start gap-1">
      <Button size="sm" icon={<RefreshCw />} loading={check.isPending} disabled={waiting} onClick={() => check.mutate()}>
        {t('aiGateway.providers.recheck')}
      </Button>
      <span aria-live="polite" className="text-xs text-stone-500">
        {waiting
          ? t('aiGateway.providers.recheckCached', {
              age: (credentialKey?.observedAt ?? provider.observedAt) ? formatAge(locale, (credentialKey?.observedAt ?? provider.observedAt)!, now) : '—',
              seconds: Math.ceil((waitUntil - now) / 1000),
            })
          : failed
            ? t('aiGateway.providers.recheckFailed')
            : null}
      </span>
    </div>
  )
}
