import { useQueryErrorResetBoundary, type QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Outlet, useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { FileQuestion } from 'lucide-react'
import { useEffect } from 'react'
import { configQuery } from '../api/queries'
import { useText } from '../i18n'
import { Shell } from '../shell/shell'
import { EmptyState, LinkButton, Loading, LoadFailed, useDocumentTitle } from '../ui'
import { paths } from './paths'

export type RouterContext = { queryClient: QueryClient }

export function NotFound() {
  const { t } = useText()
  useDocumentTitle(t('aiGateway.notFound.title'))
  return (
    <div className="mx-auto max-w-md py-12">
      <EmptyState
        icon={<FileQuestion />}
        heading="h1"
        title={t('aiGateway.notFound.title')}
        body={t('aiGateway.notFound.body')}
        action={
          <LinkButton href={paths.overview} tone="primary">
            {t('aiGateway.notFound.home')}
          </LinkButton>
        }
      />
    </div>
  )
}

/** In-page load failure (inside the shell). Retry re-runs the loaders and resets failed suspense queries. */
export function RouteError({ error }: ErrorComponentProps) {
  const router = useRouter()
  const queryErrorResetBoundary = useQueryErrorResetBoundary()
  useEffect(() => queryErrorResetBoundary.reset(), [queryErrorResetBoundary])
  return (
    <div className="mx-auto max-w-xl py-8">
      <LoadFailed error={error} onRetry={() => void router.invalidate()} />
    </div>
  )
}

export function RoutePending() {
  return <Loading />
}

/** `/config` could not be loaded: there is no shell yet. */
function RootError(props: ErrorComponentProps) {
  const { t } = useText()
  useDocumentTitle(t('aiGateway.common.loadFailed'))
  return (
    <div className="mx-auto max-w-xl px-6 py-16">
      <RouteError {...props} />
    </div>
  )
}

function RootPending() {
  return (
    <div className="mx-auto flex max-w-xl justify-center px-6 py-24">
      <Loading />
    </div>
  )
}

export const rootRoute = createRootRouteWithContext<RouterContext>()({
  // The Project and the Dashboard origin every deep link needs.
  beforeLoad: ({ context }) => context.queryClient.ensureQueryData(configQuery),
  component: () => (
    <Shell>
      <Outlet />
    </Shell>
  ),
  errorComponent: RootError,
  pendingComponent: RootPending,
  notFoundComponent: NotFound,
})
