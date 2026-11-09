// Read-model loading (React Query). Every query is a `queryOptions()` factory so
// route loaders (`ensureQueryData`) and components share one key and fetcher.
import { QueryClient, queryOptions } from '@tanstack/react-query'
import { gatewayApi, GatewayApiError } from './client'
import type { InvocationQuery } from './types'

/** Retries for a call nothing answered (rollout gap, network blip). */
const TRANSIENT_RETRIES = 3

/** Any answer with a status (4xx, coded 5xx) is final; unanswered calls are retried with backoff. */
export const retryQuery = (failureCount: number, error: unknown) =>
  error instanceof GatewayApiError && !error.transient ? false : failureCount < TRANSIENT_RETRIES

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: retryQuery, retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000), staleTime: 10_000 },
  },
})

export const GATEWAY_KEY = ['ai-gateway'] as const

/** Fixed for the life of the process. */
export const configQuery = queryOptions({ queryKey: [...GATEWAY_KEY, 'config'], queryFn: () => gatewayApi.config(), staleTime: Infinity })

export const statusQuery = queryOptions({ queryKey: [...GATEWAY_KEY, 'status'], queryFn: () => gatewayApi.status(), refetchInterval: 30_000 })

export const providersQuery = queryOptions({ queryKey: [...GATEWAY_KEY, 'providers'], queryFn: () => gatewayApi.providers(), refetchInterval: 30_000 })

export const invocationsQuery = (query: InvocationQuery) =>
  queryOptions({ queryKey: [...GATEWAY_KEY, 'invocations', query], queryFn: () => gatewayApi.invocations(query), refetchInterval: 15_000 })

/**
 * The Playground's model picker. `GET /v1/models` is a real, audited consumer call (UI4), so it is read once per
 * visit, never on focus or an interval.
 */
export const modelsQuery = queryOptions({
  queryKey: [...GATEWAY_KEY, 'models'],
  queryFn: async () => (await gatewayApi.models()).body,
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false,
})
