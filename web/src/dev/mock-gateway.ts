// Dev-only gateway: every docs/ui-api.md route plus the public `/v1/models`,
// `/v1/chat/completions` (JSON and SSE) and `/v1/embeddings`, over in-memory
// fixtures that cover Serving, Denied, Unreachable, NotConfigured and
// NotInstalled providers and Allowed, Denied and Error invocations. Playground
// calls are resolved like the gateway does (policy, provider state) and
// appended to the ring. Message keywords force the policy errors:
// "pending" → ai_policy_pending, "logging" → ai_policy_logging_unavailable,
// "identity" → identity_unavailable.
import type { ApiError, Config, GatewayStatus, InvocationEntry, InvocationQuery, Outcome, ProviderView, Route } from '../api/types'
import { filterInvocations } from '../model/invocations'

/** The signed-in mock member; `caller=me` keeps their entries. */
export const MOCK_SUBJECT = 'user-demo'
const MOCK_NAME = 'Demo User'
const PROJECT = 'pj-eee7fa8eabbc'
const CHECK_INTERVAL_MS = 30_000

const ago = (ms: number) => new Date(Date.now() - ms).toISOString()
const digest = (seed: string) => `sha256:${new Bun.CryptoHasher('sha256').update(seed).digest('hex')}`

export const mockConfig: Config = {
  projectId: PROJECT,
  publicUrl: `https://ai-gateway.${PROJECT}.appthrust.ink`,
  ...(process.env.MOCK_NO_DASHBOARD ? {} : { dashboardUrl: 'https://dashboard.appthrust.ink' }),
}

export function createMockGateway() {
  const startedAt = ago(3 * 3600_000)
  const providers: ProviderView[] = [
    {
      name: 'llm',
      kind: 'InProjectApp',
      source: { link: 'llm', linkState: 'Ready' },
      state: 'Serving',
      reason: '',
      message: '',
      observedAt: ago(40_000),
      embeddings: true,
      models: [
        { id: 'gemma3:4b', allowed: true, default: true, deniedBy: null },
        { id: 'gemma3:12b', allowed: true, default: false, deniedBy: null },
        { id: 'nomic-embed-text', allowed: true, default: false, deniedBy: null },
        { id: 'llama3.1:70b', allowed: false, default: false, deniedBy: { provider: 'llm', patterns: ['gemma3:*', 'nomic-embed-text'] } },
      ],
    },
    {
      name: 'vision',
      kind: 'InProjectApp',
      source: { link: 'vision', linkState: 'Ready' },
      state: 'Unreachable',
      reason: 'UpstreamUnavailable',
      message: 'Provider answered 503',
      observedAt: ago(90_000),
      embeddings: false,
      models: [],
    },
    {
      name: 'openai',
      kind: 'OpenAI',
      source: { credentialSlot: 'openai', bound: true },
      state: 'Denied',
      reason: 'UpstreamDenied',
      message: 'Provider rejected the configured credential',
      observedAt: ago(120_000),
      embeddings: true,
      models: [
        { id: 'gpt-5-mini', allowed: true, default: false, deniedBy: null },
        { id: 'text-embedding-3-small', allowed: true, default: false, deniedBy: null },
        { id: 'gpt-4o', allowed: false, default: false, deniedBy: { provider: 'openai', patterns: ['gpt-5*', 'text-embedding-3-*'] } },
      ],
    },
    {
      name: 'anthropic',
      kind: 'Anthropic',
      source: { credentialSlot: 'anthropic', bound: false },
      state: 'NotConfigured',
      reason: '',
      message: '',
      observedAt: null,
      embeddings: false,
      models: [],
    },
    {
      name: 'inference',
      kind: 'InProjectApp',
      source: { link: 'inference', linkState: 'NotReady' },
      state: 'NotInstalled',
      reason: '',
      message: '',
      observedAt: null,
      embeddings: false,
      models: [],
    },
  ]
  const policy = { name: 'default', uid: '6b0c2a9e-policy', generation: 3 }
  const status: GatewayStatus = {
    mode: 'Enforced',
    policy,
    configured: {
      satisfied: true,
      candidates: [
        { kind: 'CredentialSlot', name: 'openai', satisfied: false },
        { kind: 'CredentialSlot', name: 'anthropic', satisfied: false },
        { kind: 'Link', name: 'llm', satisfied: true },
      ],
    },
    health: { state: 'Degraded', reason: 'UpstreamDenied', message: 'Provider openai rejected the configured credential; llm is serving.', observedAt: ago(40_000) },
    defaultRoute: { provider: 'llm', model: 'gemma3:4b', source: 'Policy' },
    endpoints: { public: `${mockConfig.publicUrl}/v1`, internal: `http://ai-gateway-web.${PROJECT}.svc/v1` },
    release: { version: '0.3.0', artifactDigest: digest('release-0.3.0') },
    logging: { conversation: 'HashOnly', toolCalls: 'Off', retention: '90d' },
    nextSteps: [{ kind: 'FixProvider', provider: 'openai' }, { kind: 'BindCredential', slot: 'anthropic' }, { kind: 'InstallApp', definitionId: 'inference' }],
  }
  const policyRef = { name: policy.name, uid: policy.uid }
  const ring: InvocationEntry[] = [
    entry('01JD0A', 2 * 60_000, 'chat', { subject: MOCK_SUBJECT, displayName: MOCK_NAME }, 'llm', 'gemma3:4b', 'Allowed', 200, '', 1840, true, { state: 'Delivered' }, 212, 96),
    entry('01JD0B', 7 * 60_000, 'chat', { subject: 'user-jane', displayName: 'Jane Doe' }, 'openai', 'gpt-4o', 'Denied', 403, 'ai_model_denied', 3, false, { state: 'Delivered' }),
    entry('01JD0C', 12 * 60_000, 'chat', { subject: 'user-kenji', displayName: 'Kenji Sato' }, 'openai', 'gpt-5-mini', 'Error', 502, 'ai_provider_error', 412, false, { state: 'Dropped', reason: 'rejected', status: 400 }),
    entry('01JD0D', 18 * 60_000, 'embeddings', { subject: 'user-jane', displayName: 'Jane Doe' }, 'llm', 'nomic-embed-text', 'Allowed', 200, '', 96, false, { state: 'Delivered' }, 14),
    entry('01JD0E', 31 * 60_000, 'models', { subject: MOCK_SUBJECT, displayName: MOCK_NAME }, '', '', 'Allowed', 200, '', 220, false, { state: 'Delivered' }),
    entry('01JD0F', 47 * 60_000, 'chat', { subject: 'svc-knowledge', displayName: '' }, 'llm', 'gemma3:12b', 'Allowed', 200, '', 5230, true, { state: 'Dropped', reason: 'queue_full' }, 1480, 402),
    entry('01JD0G', 65 * 60_000, 'chat', { subject: 'user-kenji', displayName: 'Kenji Sato' }, 'vision', 'qwen2.5-vl', 'Error', 502, 'ai_provider_error', 30_011, false, { state: 'Delivered' }),
  ]
  const lastCheck = new Map<string, number>()
  let sequence = 0

  function entry(
    id: string,
    age: number,
    route: Route,
    caller: InvocationEntry['caller'],
    provider: string,
    model: string,
    outcome: Outcome,
    httpStatus: number,
    reason: string,
    durationMs: number,
    stream: boolean,
    report: InvocationEntry['report'],
    promptTokens?: number,
    completionTokens?: number
  ): InvocationEntry {
    return {
      id,
      time: ago(age),
      route,
      caller,
      provider,
      model,
      outcome,
      status: httpStatus,
      reason,
      durationMs,
      ...(promptTokens !== undefined ? { promptTokens } : {}),
      ...(completionTokens !== undefined ? { completionTokens } : {}),
      ...(outcome === 'Allowed' && route !== 'models' ? { requestDigest: digest(`${id}-request`) } : {}),
      ...(outcome === 'Allowed' && route === 'chat' ? { completionDigest: digest(`${id}-completion`) } : {}),
      policy: policyRef,
      stream,
      report,
    }
  }

  const json = (body: unknown, init: ResponseInit = {}) => Response.json(body, { ...init, headers: { 'Cache-Control': 'no-store', ...init.headers } })
  const fail = (httpStatus: number, code: string, message: string, extra: Partial<ApiError['error']> = {}) => json({ error: { code, message, ...extra } }, { status: httpStatus })

  /** Ring ids are allotted before the call runs so `X-Request-ID` can name the entry, like the server. */
  function nextId(): string {
    sequence += 1
    return `01JDMOCK${String(sequence).padStart(4, '0')}`
  }
  function record(route: Route, provider: string, model: string, outcome: Outcome, httpStatus: number, reason: string, durationMs: number, stream: boolean, usage?: { prompt: number; completion?: number }, id = nextId()) {
    const item: InvocationEntry = {
      ...entry(id, 0, route, { subject: MOCK_SUBJECT, displayName: MOCK_NAME }, provider, model, outcome, httpStatus, reason, durationMs, stream, { state: 'Pending' }, usage?.prompt, usage?.completion),
      time: new Date(Date.now() - durationMs).toISOString(),
    }
    ring.unshift(item)
    ring.splice(500)
    setTimeout(() => {
      item.report = { state: 'Delivered' }
    }, 1500)
  }

  type Failure = { status: number; code: string; message: string }
  type Resolved = { provider: ProviderView; model: string } | { failure: Failure; provider: string; model: string }

  /** Model resolution as the gateway does it: default route on an omitted model, then policy, then provider state. */
  function resolve(body: { provider?: string; model?: string }, route: 'chat' | 'embeddings', text: string): Resolved {
    const model = !body.model || body.model === 'auto' ? (status.defaultRoute?.model ?? '') : body.model
    const named = body.provider ?? (body.model ? providers.find((provider) => provider.models.some((candidate) => candidate.id === model))?.name : status.defaultRoute?.provider)
    const name = named ?? ''
    const refuse = (failure: Failure): Resolved => ({ failure, provider: name, model })
    if (/\bidentity\b/i.test(text)) return refuse({ status: 503, code: 'identity_unavailable', message: 'OIDC discovery is not yet available' })
    if (/\bpending\b/i.test(text)) return refuse({ status: 403, code: 'ai_policy_pending', message: 'The Project AI policy is bound but not projected yet' })
    if (/\blogging\b/i.test(text)) return refuse({ status: 403, code: 'ai_policy_logging_unavailable', message: 'The policy requires Full conversation logging' })
    const provider = providers.find((candidate) => candidate.name === name)
    const allowed = provider?.models.find((candidate) => candidate.id === model)
    if (!provider || !allowed || !allowed.allowed) return refuse({ status: 403, code: 'ai_model_denied', message: 'The model is not allowed by the effective AI policy' })
    if (provider.state !== 'Serving' || (route === 'embeddings' && !provider.embeddings)) return refuse({ status: 502, code: 'ai_provider_error', message: 'The AI provider request failed' })
    return { provider, model }
  }

  /** Records a refused call like the gateway (policy refusals are Denied) and answers with the error envelope. */
  function refused(route: Route, resolved: { failure: Failure; provider: string; model: string }, durationMs: number, stream: boolean): Response {
    const { failure } = resolved
    record(route, resolved.provider, resolved.model, failure.status === 403 ? 'Denied' : 'Error', failure.status, failure.code, durationMs, stream)
    return fail(failure.status, failure.code, failure.message)
  }

  async function chat(request: Request): Promise<Response> {
    const started = Date.now()
    const id = nextId()
    const body = (await request.json()) as { provider?: string; model?: string; messages?: { role: string; content: string }[]; stream?: boolean }
    const text = (body.messages ?? []).map((message) => message.content).join('\n')
    const resolved = resolve(body, 'chat', text)
    if ('failure' in resolved) return refused('chat', resolved, Date.now() - started, Boolean(body.stream))
    const reply = `This is ${resolved.provider.name} / ${resolved.model} answering through the AI Gateway. Your message had ${text.length} characters; the policy "${policy.name}" allowed this model, and this call is now in the invocation ring.`
    const usage = { prompt_tokens: Math.max(8, Math.round(text.length / 4)), completion_tokens: Math.round(reply.length / 4) }
    if (!body.stream) {
      await Bun.sleep(600)
      record('chat', resolved.provider.name, resolved.model, 'Allowed', 200, '', Date.now() - started, false, { prompt: usage.prompt_tokens, completion: usage.completion_tokens }, id)
      return json({ id: 'chatcmpl-mock', object: 'chat.completion', model: resolved.model, choices: [{ index: 0, message: { role: 'assistant', content: reply }, finish_reason: 'stop' }], usage }, { headers: { 'X-Request-ID': id } })
    }
    const words = reply.split(/(?<= )/)
    const encoder = new TextEncoder()
    const model = resolved.model
    const providerName = resolved.provider.name
    const stream = new ReadableStream<Uint8Array>({
      async start(controller) {
        const send = (data: unknown) => controller.enqueue(encoder.encode(`data: ${typeof data === 'string' ? data : JSON.stringify(data)}\n\n`))
        try {
          for (const word of words) {
            await Bun.sleep(45)
            send({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: { content: word }, finish_reason: null }] })
          }
          send({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: {}, finish_reason: 'stop' }], usage })
          send('[DONE]')
          controller.close()
        } catch {
          // The browser stopped reading (Stop); nothing to close.
        } finally {
          record('chat', providerName, model, 'Allowed', 200, '', Date.now() - started, true, { prompt: usage.prompt_tokens, completion: usage.completion_tokens }, id)
        }
      },
    })
    return new Response(stream, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store', 'X-Request-ID': id } })
  }

  async function embeddings(request: Request): Promise<Response> {
    const started = Date.now()
    const body = (await request.json()) as { provider?: string; model?: string; input?: string }
    const input = body.input ?? ''
    const resolved = resolve(body.model ? body : { ...body, model: 'nomic-embed-text', provider: body.provider ?? 'llm' }, 'embeddings', input)
    if ('failure' in resolved) return refused('embeddings', resolved, Date.now() - started, false)
    await Bun.sleep(250)
    const embedding = Array.from({ length: 768 }, (_, index) => Math.sin(index * 0.37 + input.length) * 0.08)
    const prompt = Math.max(1, Math.round(input.length / 4))
    record('embeddings', resolved.provider.name, resolved.model, 'Allowed', 200, '', Date.now() - started, false, { prompt })
    return json({ object: 'list', model: resolved.model, data: [{ object: 'embedding', index: 0, embedding }], usage: { prompt_tokens: prompt, total_tokens: prompt } })
  }

  /** Enforced listing: allowed models of providers that could list, plus the partial failures. */
  function models(): Response {
    const data = providers
      .filter((provider) => provider.state === 'Serving')
      .flatMap((provider) => provider.models.filter((model) => model.allowed).map((model) => ({ id: model.id, object: 'model', created: 0, owned_by: provider.name })))
    const errors = providers
      .filter((provider) => provider.state === 'Denied' || provider.state === 'Unreachable')
      .map((provider) => ({ provider: provider.name, code: 'ai_provider_error', message: 'The AI provider could not list models' }))
    record('models', '', '', 'Allowed', 200, '', 180, false)
    return json({ object: 'list', data, ...(errors.length ? { errors } : {}) })
  }

  function check(name: string): Response {
    const provider = providers.find((candidate) => candidate.name === name)
    if (!provider) return fail(404, 'ai_provider_unknown', 'Unknown provider')
    if (provider.kind === 'InProjectApp') return json(provider)
    const last = lastCheck.get(name) ?? (provider.observedAt ? Date.parse(provider.observedAt) : 0)
    const wait = CHECK_INTERVAL_MS - (Date.now() - last)
    if (wait > 0) return fail(429, 'ai_check_rate_limited', 'The last check is younger than 30 seconds', { retryAfterSeconds: Math.ceil(wait / 1000) })
    lastCheck.set(name, Date.now())
    provider.observedAt = new Date().toISOString()
    return json(provider)
  }

  const invocations = (url: URL): Response => {
    const outcome = url.searchParams.get('outcome')
    const caller = url.searchParams.get('caller')
    const limit = url.searchParams.get('limit')
    const query: InvocationQuery = {
      ...(outcome === 'Allowed' || outcome === 'Denied' || outcome === 'Error' ? { outcome } : {}),
      ...(url.searchParams.get('provider') ? { provider: url.searchParams.get('provider')! } : {}),
      ...(url.searchParams.get('model') ? { model: url.searchParams.get('model')! } : {}),
      caller: caller === 'me' ? 'me' : 'all',
      ...(limit ? { limit: Number(limit) } : {}),
    }
    const items = process.env.MOCK_EMPTY_RING ? [] : filterInvocations(ring, query, MOCK_SUBJECT)
    return json({ since: startedAt, instance: 'ai-gateway-web-00007-deployment-abc12', capacity: 500, items })
  }

  /** The response for a gateway route, else undefined. */
  return async function handle(request: Request, url: URL): Promise<Response | undefined> {
    const { method } = request
    const path = url.pathname
    if (method === 'GET' && path === '/config') return json(mockConfig)
    if (method === 'GET' && path === '/api/ai-gateway/v1/status') return json(status)
    if (method === 'GET' && path === '/api/ai-gateway/v1/providers') return json({ items: providers })
    const checkMatch = /^\/api\/ai-gateway\/v1\/providers\/([^/]+)\/check$/.exec(path)
    if (method === 'POST' && checkMatch) return check(decodeURIComponent(checkMatch[1]!))
    if (method === 'GET' && path === '/api/ai-gateway/v1/invocations') return invocations(url)
    if (method === 'GET' && path === '/v1/models') return models()
    if (method === 'POST' && path === '/v1/chat/completions') return chat(request)
    if (method === 'POST' && path === '/v1/embeddings') return embeddings(request)
    return undefined
  }
}
