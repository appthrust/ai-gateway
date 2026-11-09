import { describe, expect, test } from 'bun:test'
import { invocationSearch } from '../api/client'
import type { InvocationEntry, InvocationPage } from '../api/types'
import { createMockGateway, MOCK_SUBJECT } from '../dev/mock-gateway'
import { paths } from '../routes/paths'
import { filterInvocations, hasFilters, matchInvocation, parseInvocationsSearch, toInvocationQuery } from './invocations'

const item = (id: string, minutesAgo: number, patch: Partial<InvocationEntry> = {}): InvocationEntry => ({
  id,
  time: new Date(Date.UTC(2026, 10, 9, 10, 0) - minutesAgo * 60_000).toISOString(),
  route: 'chat',
  caller: { subject: 'jane', displayName: 'Jane' },
  provider: 'llm',
  model: 'gemma3:4b',
  outcome: 'Allowed',
  status: 200,
  reason: '',
  durationMs: 100,
  policy: null,
  stream: false,
  report: { state: 'Delivered' },
  ...patch,
})

describe('invocation filters', () => {
  test('the URL keeps only valid filters; caller defaults to everyone and the whole ring is asked for', () => {
    const search = parseInvocationsSearch({ outcome: 'Denied', provider: 'openai', model: '', caller: 'me', extra: 'x' })
    expect(search).toEqual({ outcome: 'Denied', provider: 'openai', caller: 'me' })
    expect(parseInvocationsSearch({ outcome: 'denied', caller: 'all' })).toEqual({})
    expect(hasFilters({})).toBe(false)
    expect(toInvocationQuery({ model: 'gpt-4o' })).toEqual({ model: 'gpt-4o', caller: 'all', limit: 500 })
    expect(invocationSearch(toInvocationQuery(search))).toBe('?outcome=Denied&provider=openai&caller=me&limit=500')
    expect(paths.invocationsWith(search)).toBe('/invocations?outcome=Denied&provider=openai&caller=me')
  })

  test('filters are exact and combine; caller=me keeps the signed-in subject; newest first within the limit', () => {
    const items = [
      item('a', 5),
      item('b', 1, { outcome: 'Denied', provider: 'openai', model: 'gpt-4o', reason: 'ai_model_denied' }),
      item('c', 3, { caller: { subject: 'me', displayName: 'Me' } }),
      item('d', 2, { outcome: 'Error', provider: 'openai', model: 'gpt-5-mini' }),
    ]
    expect(filterInvocations(items, { provider: 'openai' }, 'me').map((entry) => entry.id)).toEqual(['b', 'd'])
    expect(filterInvocations(items, { provider: 'openai', outcome: 'Error' }, 'me').map((entry) => entry.id)).toEqual(['d'])
    expect(filterInvocations(items, { model: 'gemma3:4b', caller: 'me' }, 'me').map((entry) => entry.id)).toEqual(['c'])
    expect(filterInvocations(items, { caller: 'all', limit: 2 }, 'me').map((entry) => entry.id)).toEqual(['b', 'd'])
  })

  test('the mock route filters like the contract', async () => {
    const gateway = createMockGateway()
    const read = async (query: string) => {
      const url = new URL(`http://mock/api/ai-gateway/v1/invocations${query}`)
      return (await (await gateway(new Request(url), url))!.json()) as InvocationPage
    }
    const all = await read('')
    expect(all.capacity).toBe(500)
    expect(new Set(all.items.map((entry) => entry.outcome))).toEqual(new Set(['Allowed', 'Denied', 'Error']))
    expect((await read('?outcome=Denied')).items.every((entry) => entry.outcome === 'Denied')).toBe(true)
    expect((await read('?provider=openai&outcome=Error')).items.map((entry) => entry.model)).toEqual(['gpt-5-mini'])
    expect((await read('?caller=me')).items.every((entry) => entry.caller.subject === MOCK_SUBJECT)).toBe(true)
  })

  test('a Playground call matches the newest record on its route that started after the send', () => {
    const started = Date.UTC(2026, 10, 9, 10, 0) - 90_000
    const items = [item('new', 1, { route: 'embeddings' }), item('mine', 1), item('old', 10)]
    expect(matchInvocation(items, 'chat', started, 'gemma3:4b')?.id).toBe('mine')
    expect(matchInvocation(items, 'chat', started, 'other')).toBeUndefined()
    expect(matchInvocation(items, 'models', started, undefined)).toBeUndefined()
  })
})
