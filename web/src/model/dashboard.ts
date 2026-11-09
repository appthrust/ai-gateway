// Deep links to the owners of declared state (UI1): the gateway shows facts and
// sends the person to the Dashboard to change them. Without `dashboardUrl`
// (PLATFORM_DASHBOARD_URL unset) every target is undefined and screens show the
// Dashboard path as text instead of a link.
import type { Config } from '../api/types'

export type DashboardLinks = {
  /** The AI Gateway installation: credentials (API key slots), connected apps, version. */
  installation?: string
  /** Project settings; its "AI" tab binds and narrows the AI policy (ADR-0062). */
  settings?: string
  /** Durable invocation history (`projects.ai-invocations.create`), visible to team admins. */
  audit?: string
}

export function dashboardLinks(config: Config): DashboardLinks {
  const origin = config.dashboardUrl?.replace(/\/+$/, '')
  if (!origin) return {}
  const project = `${origin}/dashboard/projects/${encodeURIComponent(config.projectId)}`
  return { installation: `${project}/marketplace/ai-gateway`, settings: `${project}/settings`, audit: `${origin}/dashboard/audit` }
}
