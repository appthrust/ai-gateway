import { useQuery, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { Send, Square } from 'lucide-react'
import { Tabs } from 'radix-ui'
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { gatewayApi, isGatewayError, type ChatRequest, type ModelList, type OpenAIModel, type Usage } from '../../api/client'
import { configQuery, GATEWAY_KEY, modelsQuery, providersQuery, statusQuery } from '../../api/queries'
import type { InvocationEntry, Outcome, ProviderView, Route } from '../../api/types'
import { useText } from '../../i18n'
import { dashboardLinks } from '../../model/dashboard'
import { outcomeOf } from '../../model/error-codes'
import { formatDuration, formatTime, shortDigest } from '../../model/format'
import { matchInvocation } from '../../model/invocations'
import { initialOption, pickerOptions, type PickerOption } from '../../model/model-picker'
import { paths } from '../../routes/paths'
import { Button, Card, cn, EmptyState, Field, HrefLink, Notice, PageHeader, TextArea, useDocumentTitle } from '../../ui'
import { ErrorExplanation } from '../common/error-explanation'
import { OutcomeBadge, ReportState } from '../common/invocation'
import { ModelPicker } from './model-picker'

type Tab = 'chat' | 'embeddings' | 'models'

const NO_MODELS: readonly OpenAIModel[] = []

/** One Playground call as the result panel shows it: what was asked, what came back, and the instance's record. */
type Run = {
  route: Route
  provider: string | null
  model: string | null
  startedAt: number
  pending: boolean
  stopped: boolean
  durationMs?: number
  status?: number
  outcome?: Outcome
  usage?: Usage | null
  text: string
  embedding?: { count: number; dimensions: number; preview: number[] }
  models?: ModelList
  error?: { code: string; status: number; message: string; transient: boolean }
  /** `X-Request-ID` of the call; exact key into this instance's ring. */
  invocationId?: string
  record?: InvocationEntry
  recordState: 'none' | 'loading' | 'found' | 'missing'
}

export function PlaygroundPage() {
  const { t } = useText()
  const client = useQueryClient()
  const { data: status } = useSuspenseQuery(statusQuery)
  const { data: providers } = useSuspenseQuery(providersQuery)
  const { data: config } = useSuspenseQuery(configQuery)
  const models = useQuery(modelsQuery)
  const links = dashboardLinks(config)
  useDocumentTitle(t('aiGateway.nav.playground'))

  const [tab, setTab] = useState<Tab>('chat')
  const listed = models.data?.data ?? NO_MODELS
  const chatOptions = useMemo(() => pickerOptions(listed, providers, status.defaultRoute, 'chat'), [listed, providers, status.defaultRoute])
  const embeddingOptions = useMemo(() => pickerOptions(listed, providers, status.defaultRoute, 'embeddings'), [listed, providers, status.defaultRoute])
  const [chatValue, setChatValue] = useState<string>()
  const [embeddingValue, setEmbeddingValue] = useState<string>()
  // Pre-select the default route once the listing arrives; keep a choice the person made.
  const chatSelected = chatOptions.find((option) => option.value === chatValue && !option.blocked) ?? initialOption(chatOptions)
  const embeddingSelected = embeddingOptions.find((option) => option.value === embeddingValue && !option.blocked) ?? initialOption(embeddingOptions)

  const [system, setSystem] = useState('')
  const [user, setUser] = useState('')
  const [stream, setStream] = useState(true)
  const [input, setInput] = useState('')
  const [run, setRun] = useState<Run>()
  const abort = useRef<AbortController | null>(null)
  useEffect(() => () => abort.current?.abort(), [])

  const begin = (route: Route, option: PickerOption | undefined): { controller: AbortController; started: number; t0: number } => {
    abort.current?.abort()
    const controller = new AbortController()
    abort.current = controller
    const started = Date.now()
    setRun({
      route,
      provider: option?.provider ?? (route === 'models' ? null : (status.defaultRoute?.provider ?? null)),
      model: option?.model ?? null,
      startedAt: started,
      pending: true,
      stopped: false,
      text: '',
      recordState: 'none',
    })
    return { controller, started, t0: performance.now() }
  }

  /** Ends the call, then reads this instance's ring: exact `X-Request-ID` match first, time/model heuristic as fallback. */
  const finish = async (route: Route, started: number, t0: number, patch: Partial<Run>, model: string | undefined) => {
    const durationMs = performance.now() - t0
    const error = patch.error
    setRun((current) => current && { ...current, ...patch, pending: false, durationMs, outcome: patch.status === undefined ? undefined : outcomeOf(patch.status, error?.code), recordState: error?.transient ? 'none' : 'loading' })
    void client.invalidateQueries({ queryKey: [...GATEWAY_KEY, 'invocations'] })
    if (error?.transient) return
    try {
      const page = await gatewayApi.invocations({ caller: 'me', limit: 20 })
      const record = (patch.invocationId && page.items.find((item) => item.id === patch.invocationId)) || matchInvocation(page.items, route, started, model)
      setRun((current) => current && current.startedAt === started ? { ...current, record, recordState: record ? 'found' : 'missing', ...(record ? { outcome: record.outcome } : {}) } : current)
    } catch {
      setRun((current) => current && current.startedAt === started ? { ...current, recordState: 'missing' } : current)
    }
  }

  const failure = (error: unknown): Partial<Run> =>
    isGatewayError(error)
      ? { status: error.status || undefined, invocationId: error.invocationId, error: { code: error.code, status: error.status, message: error.message, transient: error.transient } }
      : { error: { code: 'unknown', status: 0, message: '', transient: true } }

  const sendChat = async (event: FormEvent) => {
    event.preventDefault()
    if (!user.trim()) return
    const option = chatSelected
    const { controller, started, t0 } = begin('chat', option)
    const request: ChatRequest = {
      ...(option?.provider ? { provider: option.provider } : {}),
      ...(option ? { model: option.model } : {}),
      messages: [...(system.trim() ? [{ role: 'system' as const, content: system }] : []), { role: 'user' as const, content: user }],
    }
    try {
      if (stream) {
        const answer = await gatewayApi.chatStream(
          request,
          (chunk) => {
            const delta = chunk.choices?.[0]?.delta?.content ?? ''
            setRun((current) => current && current.startedAt === started ? { ...current, text: current.text + delta, usage: chunk.usage ?? current.usage } : current)
          },
          controller.signal
        )
        await finish('chat', started, t0, { status: answer.status, invocationId: answer.invocationId }, option?.model)
      } else {
        const answer = await gatewayApi.chat(request, controller.signal)
        await finish('chat', started, t0, { status: answer.status, invocationId: answer.invocationId, text: answer.body.choices?.[0]?.message?.content ?? '', usage: answer.body.usage }, option?.model)
      }
    } catch (error) {
      if (controller.signal.aborted) {
        await finish('chat', started, t0, { stopped: true }, option?.model)
        return
      }
      await finish('chat', started, t0, failure(error), option?.model)
    }
  }

  const sendEmbeddings = async (event: FormEvent) => {
    event.preventDefault()
    if (!input.trim()) return
    const option = embeddingSelected
    const { controller, started, t0 } = begin('embeddings', option)
    try {
      const answer = await gatewayApi.embeddings({ ...(option?.provider ? { provider: option.provider } : {}), ...(option ? { model: option.model } : {}), input }, controller.signal)
      const vectors = answer.body.data ?? []
      const first = vectors[0]?.embedding ?? []
      await finish('embeddings', started, t0, { status: answer.status, invocationId: answer.invocationId, usage: answer.body.usage, embedding: { count: vectors.length, dimensions: first.length, preview: first.slice(0, 8) } }, option?.model)
    } catch (error) {
      await finish('embeddings', started, t0, controller.signal.aborted ? { stopped: true } : failure(error), option?.model)
    }
  }

  const listModels = async () => {
    const { controller, started, t0 } = begin('models', undefined)
    try {
      const answer = await gatewayApi.models()
      if (controller.signal.aborted) return
      client.setQueryData(modelsQuery.queryKey, answer.body)
      await finish('models', started, t0, { status: answer.status, invocationId: answer.invocationId, models: answer.body }, undefined)
    } catch (error) {
      await finish('models', started, t0, failure(error), undefined)
    }
  }

  const deniedPatterns = run?.model ? deniedBy(providers, run.provider, run.model) : undefined
  const busy = run?.pending ?? false

  return (
    <>
      <PageHeader title={t('aiGateway.nav.playground')} lead={t('aiGateway.playground.lead')} />
      <Notice className="mb-6" title={t('aiGateway.playground.notice.title')}>
        {t('aiGateway.playground.notice.body')}
      </Notice>
      <div className="grid items-start gap-6 lg:grid-cols-2">
        <Card className="p-4">
          <Tabs.Root value={tab} onValueChange={(value) => setTab(value as Tab)}>
            <Tabs.List aria-label={t('aiGateway.playground.tabs')} className="mb-4 inline-flex rounded-md border border-stone-300 bg-white p-0.5 shadow-xs">
              {(['chat', 'embeddings', 'models'] as const).map((value) => (
                <Tabs.Trigger
                  key={value}
                  value={value}
                  className="inline-flex h-9 items-center rounded px-3 text-xs font-medium text-stone-600 hover:bg-stone-100 hover:text-stone-900 data-[state=active]:bg-accent data-[state=active]:text-white md:h-7"
                >
                  {t(`aiGateway.playground.tab.${value}`)}
                </Tabs.Trigger>
              ))}
            </Tabs.List>
            <Tabs.Content value="chat">
              <form onSubmit={(event) => void sendChat(event)} className="space-y-4">
                <ModelPicker
                  id="chat-model"
                  options={chatOptions}
                  value={chatSelected?.value}
                  onChange={setChatValue}
                  loading={models.isPending}
                  listFailed={models.isError}
                />
                <Field id="chat-system" label={t('aiGateway.playground.system')} hint={t('aiGateway.playground.systemHint')}>
                  <TextArea id="chat-system" rows={2} className="min-h-16" value={system} onChange={(event) => setSystem(event.target.value)} />
                </Field>
                <Field id="chat-user" label={t('aiGateway.playground.user')} required>
                  <TextArea id="chat-user" rows={4} value={user} required onChange={(event) => setUser(event.target.value)} placeholder={t('aiGateway.playground.userPlaceholder')} />
                </Field>
                <label className="flex items-center gap-2 text-sm text-stone-700">
                  <input type="checkbox" checked={stream} onChange={(event) => setStream(event.target.checked)} className="size-4 accent-(--color-accent)" />
                  {t('aiGateway.playground.stream')}
                </label>
                <SendRow busy={busy} disabled={!user.trim()} onStop={() => abort.current?.abort()} />
              </form>
            </Tabs.Content>
            <Tabs.Content value="embeddings">
              <form onSubmit={(event) => void sendEmbeddings(event)} className="space-y-4">
                <ModelPicker
                  id="embedding-model"
                  options={embeddingOptions}
                  value={embeddingSelected?.value}
                  onChange={setEmbeddingValue}
                  loading={models.isPending}
                  listFailed={models.isError}
                />
                <Field id="embedding-input" label={t('aiGateway.playground.input')} required>
                  <TextArea id="embedding-input" rows={4} value={input} required onChange={(event) => setInput(event.target.value)} />
                </Field>
                <SendRow busy={busy} disabled={!input.trim()} onStop={() => abort.current?.abort()} />
              </form>
            </Tabs.Content>
            <Tabs.Content value="models" className="space-y-4">
              <p className="text-sm text-stone-600">{t('aiGateway.playground.modelsLead')}</p>
              <Button tone="primary" icon={<Send />} loading={busy} onClick={() => void listModels()}>
                {t('aiGateway.playground.listModels')}
              </Button>
            </Tabs.Content>
          </Tabs.Root>
        </Card>
        <div className="space-y-4" aria-live="polite" aria-busy={busy}>
          {run ? (
            <>
              <Output run={run} />
              {run.error ? (
                <ErrorExplanation
                  code={run.error.code}
                  status={run.error.status}
                  message={run.error.message}
                  transient={run.error.transient}
                  context={{ provider: run.provider ?? undefined, model: run.model ?? undefined, patterns: deniedPatterns }}
                  links={links}
                />
              ) : null}
              {run.pending ? null : <ResultPanel run={run} />}
            </>
          ) : (
            <EmptyState title={t('aiGateway.playground.idle.title')} body={t('aiGateway.playground.idle.body')} />
          )}
        </div>
      </div>
    </>
  )
}

/** The policy rule that excludes `model` on `provider` (any provider when unknown), from the providers view. */
function deniedBy(providers: readonly ProviderView[], provider: string | null, model: string): string[] | undefined {
  for (const view of providers) {
    if (provider && view.name !== provider) continue
    const entry = view.models.find((candidate) => candidate.id === model && !candidate.allowed)
    if (entry) return entry.deniedBy?.patterns ?? []
  }
  return undefined
}

function SendRow({ busy, disabled, onStop }: { busy: boolean; disabled: boolean; onStop: () => void }) {
  const { t } = useText()
  return (
    <div className="flex items-center gap-2">
      <Button type="submit" tone="primary" icon={<Send />} loading={busy} disabled={disabled}>
        {t('aiGateway.playground.send')}
      </Button>
      {busy ? (
        <Button icon={<Square />} onClick={onStop}>
          {t('aiGateway.playground.stop')}
        </Button>
      ) : null}
    </div>
  )
}

function Output({ run }: { run: Run }) {
  const { t, locale } = useText()
  if (run.route === 'chat') {
    if (!run.text && run.error) return null
    return (
      <Card className="p-4">
        <h2 className="mb-2 text-xs font-medium text-stone-500">{t('aiGateway.playground.output')}</h2>
        <p className={cn('min-h-12 text-sm leading-relaxed whitespace-pre-wrap text-stone-900', run.pending && !run.text ? 'text-stone-400' : undefined)}>
          {run.text || (run.pending ? t('aiGateway.playground.waiting') : t('aiGateway.playground.noText'))}
          {run.pending && run.text ? <span aria-hidden className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-accent align-text-bottom" /> : null}
        </p>
        {run.stopped ? <p className="mt-2 text-xs text-stone-500">{t('aiGateway.playground.stopped')}</p> : null}
      </Card>
    )
  }
  if (run.route === 'embeddings') {
    if (!run.embedding) return run.pending ? <Card className="p-4 text-sm text-stone-400">{t('aiGateway.playground.waiting')}</Card> : null
    const format = new Intl.NumberFormat(locale, { maximumFractionDigits: 4 })
    return (
      <Card className="p-4">
        <h2 className="mb-2 text-xs font-medium text-stone-500">{t('aiGateway.playground.output')}</h2>
        <p className="text-sm text-stone-900">{t('aiGateway.playground.vector', { count: run.embedding.count, dimensions: run.embedding.dimensions })}</p>
        <p className="mt-2 font-mono text-xs break-all text-stone-600">[{run.embedding.preview.map((value) => format.format(value)).join(', ')}{run.embedding.dimensions > run.embedding.preview.length ? ', …' : ''}]</p>
      </Card>
    )
  }
  if (!run.models) return run.pending ? <Card className="p-4 text-sm text-stone-400">{t('aiGateway.playground.waiting')}</Card> : null
  return (
    <Card className="p-4">
      <h2 className="mb-2 text-xs font-medium text-stone-500">{t('aiGateway.playground.modelsCount', { count: run.models.data.length })}</h2>
      <ul className="divide-y divide-line text-sm">
        {run.models.data.map((model) => (
          <li key={model.id} className="flex items-center justify-between gap-3 py-1.5">
            <span className="font-mono text-xs text-stone-900">{model.id}</span>
            {model.owned_by ? <span className="text-xs text-stone-500">{model.owned_by}</span> : null}
          </li>
        ))}
      </ul>
      {run.models.errors && run.models.errors.length > 0 ? (
        <Notice tone="warning" className="mt-3" title={t('aiGateway.playground.modelsPartial')}>
          <ul>
            {run.models.errors.map((failure) => (
              <li key={failure.provider}>
                <span className="font-mono">{failure.provider}</span> — {failure.code}
              </li>
            ))}
          </ul>
        </Notice>
      ) : null}
    </Card>
  )
}

function ResultRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <dt className="text-stone-500">{label}</dt>
      <dd className="min-w-0 text-right text-stone-900">{children}</dd>
    </div>
  )
}

/** The invocation record of the call: client-measured duration, the answer's usage, and the ring entry's digests and delivery. */
function ResultPanel({ run }: { run: Run }) {
  const { t, language, locale } = useText()
  const record = run.record
  const prompt = run.usage?.prompt_tokens ?? record?.promptTokens
  const completion = run.usage?.completion_tokens ?? record?.completionTokens
  return (
    <Card className="p-4">
      <h2 className="mb-1 text-sm font-semibold text-stone-900">{t('aiGateway.playground.result')}</h2>
      <dl className="divide-y divide-line text-sm">
        <ResultRow label={t('aiGateway.field.provider')}>
          <span className="font-mono text-xs">{record?.provider || run.provider || '—'}</span>
        </ResultRow>
        <ResultRow label={t('aiGateway.field.model')}>
          <span className="font-mono text-xs">{record?.model || run.model || '—'}</span>
        </ResultRow>
        <ResultRow label={t('aiGateway.field.outcome')}>
          {run.outcome ? <OutcomeBadge outcome={run.outcome} /> : '—'}
        </ResultRow>
        <ResultRow label={t('aiGateway.field.status')}>
          <span className="tabular-nums">{run.status ?? '—'}</span>
        </ResultRow>
        <ResultRow label={t('aiGateway.field.duration')}>
          <span className="tabular-nums">{run.durationMs === undefined ? '—' : formatDuration(locale, run.durationMs)}</span>
          <span className="block text-xs text-stone-500">{t('aiGateway.playground.durationHint')}</span>
        </ResultRow>
        {prompt !== undefined || completion !== undefined ? (
          <ResultRow label={t('aiGateway.field.tokens')}>
            <span className="tabular-nums">{t('aiGateway.tokens', { prompt: prompt ?? '—', completion: completion ?? '—' })}</span>
          </ResultRow>
        ) : null}
        {record?.requestDigest ? (
          <ResultRow label={t('aiGateway.field.requestDigest')}>
            <span className="font-mono text-xs" title={record.requestDigest}>
              {shortDigest(record.requestDigest)}
            </span>
          </ResultRow>
        ) : null}
        {record?.completionDigest ? (
          <ResultRow label={t('aiGateway.field.completionDigest')}>
            <span className="font-mono text-xs" title={record.completionDigest}>
              {shortDigest(record.completionDigest)}
            </span>
          </ResultRow>
        ) : null}
        {record ? (
          <ResultRow label={t('aiGateway.field.report')}>
            <ReportState report={record.report} />
          </ResultRow>
        ) : null}
      </dl>
      <p className="mt-2 text-xs text-stone-500">
        {run.recordState === 'loading'
          ? t('aiGateway.playground.record.loading')
          : run.recordState === 'found' && record
            ? t('aiGateway.playground.record.found', { time: formatTime(language, record.time) })
            : run.recordState === 'missing'
              ? t('aiGateway.playground.record.missing')
              : null}{' '}
        {run.recordState === 'found' ? (
          <HrefLink href={paths.invocations} className="font-medium text-accent-strong hover:underline">
            {t('aiGateway.playground.record.open')}
          </HrefLink>
        ) : null}
      </p>
    </Card>
  )
}
