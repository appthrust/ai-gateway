// Invocations filters: the URL (`/invocations?outcome=&provider=&model=&caller=me`)
// is the state; the server filters (`GET /api/ai-gateway/v1/invocations`).
// `filterInvocations` is the same rule on the client side, used by the mock
// server so dev and tests filter exactly like the contract says.
import type { InvocationEntry, InvocationQuery, Outcome } from '../api/types'

export type InvocationsSearch = { outcome?: Outcome; provider?: string; model?: string; caller?: 'me' }

export const OUTCOMES: readonly Outcome[] = ['Allowed', 'Denied', 'Error']

/** The ring holds at most 500 entries; ask for all of them. */
export const INVOCATION_LIMIT = 500

export function parseInvocationsSearch(search: Record<string, unknown>): InvocationsSearch {
  const text = (value: unknown) => (typeof value === 'string' && value ? value : undefined)
  const outcome = OUTCOMES.find((candidate) => candidate === search.outcome)
  const provider = text(search.provider)
  const model = text(search.model)
  return {
    ...(outcome ? { outcome } : {}),
    ...(provider ? { provider } : {}),
    ...(model ? { model } : {}),
    ...(search.caller === 'me' ? { caller: 'me' as const } : {}),
  }
}

export function toInvocationQuery(search: InvocationsSearch): InvocationQuery {
  return { ...search, caller: search.caller ?? 'all', limit: INVOCATION_LIMIT }
}

export const hasFilters = (search: InvocationsSearch) => Boolean(search.outcome || search.provider || search.model || search.caller)

/** Contract filtering: exact outcome/provider/model, `caller=me` keeps `subject`'s entries; newest first, at most `limit` (default 100, max 500). */
export function filterInvocations(items: readonly InvocationEntry[], query: InvocationQuery, subject: string): InvocationEntry[] {
  const limit = Math.min(Math.max(query.limit ?? 100, 1), 500)
  return items
    .filter(
      (item) =>
        (!query.outcome || item.outcome === query.outcome) &&
        (!query.provider || item.provider === query.provider) &&
        (!query.model || item.model === query.model) &&
        (query.caller !== 'me' || item.caller.subject === subject)
    )
    .sort((a, b) => b.time.localeCompare(a.time))
    .slice(0, limit)
}

/**
 * The ring entry for a Playground call: the caller's newest entry on `route` that started at or after `startedAt`
 * (less clock skew) and names the same model when one was asked for. The public routes return no invocation id,
 * so this is a match, not a lookup.
 */
export function matchInvocation(items: readonly InvocationEntry[], route: InvocationEntry['route'], startedAt: number, model: string | undefined): InvocationEntry | undefined {
  const SKEW_MS = 2_000
  return items.find((item) => item.route === route && Date.parse(item.time) >= startedAt - SKEW_MS && (!model || item.model === model))
}
