// Every gateway URL (design "Information architecture").
import type { InvocationsSearch } from '../model/invocations'

export const paths = {
  overview: '/',
  providers: '/providers',
  playground: '/playground',
  invocations: '/invocations',
  /** `/invocations` with only the set filters. */
  invocationsWith: (search: InvocationsSearch) => {
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(search)) if (value) params.set(key, value)
    const text = params.toString()
    return text ? `/invocations?${text}` : '/invocations'
  },
  connect: '/connect',
}
