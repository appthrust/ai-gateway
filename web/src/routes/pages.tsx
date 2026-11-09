// Page routes (design "Information architecture"). Loaders read through React
// Query; pages read the same keys with useSuspenseQuery.
import { createRoute } from '@tanstack/react-router'
import { invocationsQuery, providersQuery, statusQuery } from '../api/queries'
import { ConnectPage } from '../features/connect/connect-page'
import { InvocationsPage } from '../features/invocations/invocations-page'
import { OverviewPage } from '../features/overview/overview-page'
import { PlaygroundPage } from '../features/playground/playground-page'
import { ProvidersPage } from '../features/providers/providers-page'
import { parseInvocationsSearch, toInvocationQuery, type InvocationsSearch } from '../model/invocations'
import { rootRoute } from './root'

export const overviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  loader: ({ context }) => context.queryClient.ensureQueryData(statusQuery),
  component: OverviewPage,
})

export const providersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/providers',
  loader: ({ context }) => context.queryClient.ensureQueryData(providersQuery),
  component: ProvidersPage,
})

// The model list (`GET /v1/models`) loads inside the page: a failing listing must not hide the Playground.
export const playgroundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/playground',
  loader: ({ context }) => Promise.all([context.queryClient.ensureQueryData(statusQuery), context.queryClient.ensureQueryData(providersQuery)]),
  component: PlaygroundPage,
})

export const invocationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/invocations',
  validateSearch: (search: Record<string, unknown>): InvocationsSearch => parseInvocationsSearch(search),
  loaderDeps: ({ search }) => search,
  // Provider and model filter choices come from the providers view; they may load after the table.
  loader: ({ context, deps }) =>
    Promise.all([context.queryClient.ensureQueryData(invocationsQuery(toInvocationQuery(deps))), context.queryClient.prefetchQuery(providersQuery)]),
  component: function InvocationsRoute() {
    return <InvocationsPage search={invocationsRoute.useSearch()} />
  },
})

export const connectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/connect',
  loader: ({ context }) => context.queryClient.ensureQueryData(statusQuery),
  component: ConnectPage,
})

export const routeTree = rootRoute.addChildren([overviewRoute, providersRoute, playgroundRoute, invocationsRoute, connectRoute])
