// The gateway as the SPA sees it, all on the SPA's own origin: the read model
// (`/api/ai-gateway/v1/*`, docs/ui-api.md, shapes in ./types), `GET /config`,
// and the public OpenAI-compatible routes (`/v1/*`) the Playground calls as the
// signed-in member (UI4), so its calls are recorded and audited like any app's.
// The OIDC edge (Envoy) attaches the caller's bearer; the SPA never sees one.
import { createSSEParser } from './sse'
import type { ApiError, Config, GatewayStatus, InvocationPage, InvocationQuery, ProvidersResponse, ProviderView } from './types'

/**
 * Non-2xx answer. `code` is the gateway's error code (`ai_model_denied`, …).
 * `transient` = no error envelope came back: a 5xx from a proxy in front of the
 * server (e.g. during a rollout), or no response at all (status 0).
 */
export class GatewayApiError extends Error {
  readonly status: number
  readonly code: string
  readonly transient: boolean
  readonly retryAfterSeconds: number | undefined
  /** Ring entry id from `X-Request-ID` on `/v1/*` answers; undefined elsewhere. */
  readonly invocationId: string | undefined

  constructor(status: number, body: Partial<ApiError> | undefined, invocationId?: string) {
    const envelope = body?.error && typeof body.error === 'object' ? body.error : undefined
    const code = typeof envelope?.code === 'string' && envelope.code ? envelope.code : undefined
    super(envelope?.message || `ai gateway ${status}`)
    this.name = 'GatewayApiError'
    this.status = status
    this.code = code ?? (status === 404 ? 'not_found' : 'unknown')
    this.transient = code === undefined && (status === 0 || status >= 500)
    this.retryAfterSeconds = typeof envelope?.retryAfterSeconds === 'number' ? envelope.retryAfterSeconds : undefined
    this.invocationId = invocationId
  }
}

export function isGatewayError(error: unknown, code?: string): error is GatewayApiError {
  return error instanceof GatewayApiError && (code === undefined || error.code === code)
}

// OpenAI shapes of the public routes, only the fields the Playground reads.
export type OpenAIModel = { id: string; object?: string; owned_by?: string }
/** `errors`: providers that could not list models (a partial result, still 200). */
export type ModelList = { object: string; data: OpenAIModel[]; errors?: { provider: string; code: string; message: string }[] }
export type ChatMessage = { role: 'system' | 'user'; content: string }
/** `provider` picks the provider explicitly; `model` omitted (or `auto`) resolves to the default route. */
export type ChatRequest = { provider?: string; model?: string; messages: ChatMessage[] }
export type Usage = { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number }
export type ChatCompletion = { model?: string; choices?: { message?: { content?: string | null } }[]; usage?: Usage | null }
export type ChatChunk = { model?: string; choices?: { delta?: { content?: string | null } }[]; usage?: Usage | null }
export type EmbeddingsRequest = { provider?: string; model?: string; input: string }
export type EmbeddingsResponse = { model?: string; data?: { index: number; embedding: number[] }[]; usage?: Usage | null }
/** A Playground answer with the HTTP status the result panel shows; `invocationId` is the ring entry (`X-Request-ID`). */
export type Answer<T> = { status: number; body: T; invocationId?: string }

export const INVOCATION_ID_HEADER = 'X-Request-ID'
const invocationIdOf = (response: Response) => response.headers.get(INVOCATION_ID_HEADER) ?? undefined

type Fetcher = (input: string, init: RequestInit) => Promise<Response>

const BASE = '/api/ai-gateway/v1'
const READ_TIMEOUT_MS = 8_000
/** `GET /v1/models` asks every provider for its list; allow for a slow one. */
const MODELS_TIMEOUT_MS = 20_000
// A 401 first sends the browser to `/` so the edge re-runs sign-in. Another 401
// within this window means the edge forwards a token the server rejects;
// redirecting again would loop, so that 401 reaches the screens instead.
const REAUTH_WINDOW_MS = 60_000
const REAUTH_AT_KEY = 'ai-gateway:reauth-at'

export interface GatewayApi {
  /** Project, public origin and Dashboard origin (`dashboardUrl` absent when unknown). */
  config(): Promise<Config>
  status(): Promise<GatewayStatus>
  providers(): Promise<ProviderView[]>
  /** Forces a probe; 429 `ai_check_rate_limited` with `retryAfterSeconds` while the last probe is younger than 30 s. */
  check(name: string, slot?: string): Promise<ProviderView>
  invocations(query?: InvocationQuery): Promise<InvocationPage>
  models(): Promise<Answer<ModelList>>
  chat(request: ChatRequest, signal?: AbortSignal): Promise<Answer<ChatCompletion>>
  /** `stream: true`; `onChunk` gets each `chat.completion.chunk` as it arrives. Resolves after `[DONE]` or the end of the body. */
  chatStream(request: ChatRequest, onChunk: (chunk: ChatChunk) => void, signal?: AbortSignal): Promise<Answer<undefined>>
  embeddings(request: EmbeddingsRequest, signal?: AbortSignal): Promise<Answer<EmbeddingsResponse>>
}

/** `?outcome=&provider=&model=&caller=&limit=` with only the set fields, in that order. */
export function invocationSearch(query: InvocationQuery = {}): string {
  const params = new URLSearchParams()
  if (query.outcome) params.set('outcome', query.outcome)
  if (query.provider) params.set('provider', query.provider)
  if (query.model) params.set('model', query.model)
  if (query.caller) params.set('caller', query.caller)
  if (query.limit !== undefined) params.set('limit', String(query.limit))
  const text = params.toString()
  return text ? `?${text}` : ''
}

export function createGatewayApi(fetcher: Fetcher = (input, init) => fetch(input, init), reauth = browserReauth): GatewayApi {
  async function open(method: string, url: string, init: { body?: unknown; accept?: string; signal?: AbortSignal; timeoutMs?: number } = {}): Promise<Response> {
    const headers: Record<string, string> = { Accept: init.accept ?? 'application/json' }
    if (init.body !== undefined) headers['Content-Type'] = 'application/json'
    const signals = [init.signal, init.timeoutMs ? AbortSignal.timeout(init.timeoutMs) : undefined].filter((signal) => signal !== undefined)
    let response: Response
    try {
      response = await fetcher(url, {
        method,
        headers,
        body: init.body === undefined ? undefined : JSON.stringify(init.body),
        credentials: 'same-origin',
        signal: signals.length === 0 ? undefined : AbortSignal.any(signals),
      })
    } catch (error) {
      // A Stop the person asked for is not a failure to report.
      if (init.signal?.aborted) throw error
      // Network failure or read timeout: nothing answered.
      throw new GatewayApiError(0, undefined)
    }
    if (response.status === 401 && reauth()) return new Promise<Response>(() => {})
    if (!response.ok) {
      let parsed: Partial<ApiError> | undefined
      try {
        parsed = (await response.json()) as Partial<ApiError>
      } catch {
        parsed = undefined
      }
      throw new GatewayApiError(response.status, parsed, invocationIdOf(response))
    }
    return response
  }
  const read = async <T>(url: string, timeoutMs = READ_TIMEOUT_MS) => (await (await open('GET', url, { timeoutMs })).json()) as T
  const answer = async <T>(response: Response): Promise<Answer<T>> => ({ status: response.status, body: (await response.json()) as T, invocationId: invocationIdOf(response) })

  return {
    config: () => read<Config>('/config'),
    status: () => read<GatewayStatus>(`${BASE}/status`),
    providers: async () => (await read<ProvidersResponse>(`${BASE}/providers`)).items,
    check: async (name, slot) => (await (await open('POST', `${BASE}/providers/${encodeURIComponent(name)}${slot ? `/keys/${encodeURIComponent(slot)}` : ''}/check`)).json()) as ProviderView,
    invocations: (query) => read<InvocationPage>(`${BASE}/invocations${invocationSearch(query)}`),
    models: async () => answer<ModelList>(await open('GET', '/v1/models', { timeoutMs: MODELS_TIMEOUT_MS })),
    chat: async (request, signal) => answer<ChatCompletion>(await open('POST', '/v1/chat/completions', { body: { ...request, stream: false }, signal })),
    embeddings: async (request, signal) => answer<EmbeddingsResponse>(await open('POST', '/v1/embeddings', { body: request, signal })),
    chatStream: async (request, onChunk, signal) => {
      const response = await open('POST', '/v1/chat/completions', { body: { ...request, stream: true }, accept: 'text/event-stream', signal })
      if (!response.body) return { status: response.status, body: undefined, invocationId: invocationIdOf(response) }
      let done = false
      const parser = createSSEParser((data) => {
        if (data === '[DONE]') {
          done = true
          return
        }
        try {
          onChunk(JSON.parse(data) as ChatChunk)
        } catch {
          // A non-JSON data line (a keep-alive comment rendered as data) carries no chunk.
        }
      })
      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader()
      while (!done) {
        const next = await reader.read()
        if (next.done) break
        parser.push(next.value)
      }
      parser.flush()
      if (done) await reader.cancel().catch(() => {})
      return { status: response.status, body: undefined, invocationId: invocationIdOf(response) }
    },
  }
}

function browserReauth(): boolean {
  const lastReauthAt = Number(sessionStorage.getItem(REAUTH_AT_KEY))
  if (Date.now() - lastReauthAt < REAUTH_WINDOW_MS) return false
  sessionStorage.setItem(REAUTH_AT_KEY, String(Date.now()))
  window.location.assign('/')
  return true
}

export const gatewayApi = createGatewayApi()
