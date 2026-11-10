// Mirror of docs/ui-api.md. Keep identical by hand; the Go server is authoritative.

export type Mode = 'Unrestricted' | 'Enforced' | 'Pending'
export type HealthState = 'Serving' | 'Degraded' | 'Failing' | 'Unknown'
export type Outcome = 'Allowed' | 'Denied' | 'Error'
export type Route = 'chat' | 'embeddings' | 'models'

export interface Config {
  projectId: string
  publicUrl: string
  dashboardUrl?: string
}

export interface PolicyRef {
  name: string
  uid: string
  generation?: number
}

export type RequirementCandidate =
  | { kind: 'CredentialSlot'; name: string; satisfied: boolean }
  | { kind: 'Link'; name: string; satisfied: boolean }

export interface Health {
  state: HealthState
  reason: string
  message: string
  observedAt: string | null
}

export type RouteSource = 'Policy' | 'EnvOverride' | 'Capability' | 'Credential'

export interface DefaultRoute {
  provider: string
  model: string
  source: RouteSource
}

export type NextStep =
  | { kind: 'BindCredential'; slot: string }
  | { kind: 'InstallApp'; definitionId: string }
  | { kind: 'BindPolicy' }
  | { kind: 'FixProvider'; provider: string; slot?: string }
  | { kind: 'ContactAdmin'; reason: string }

export interface GatewayStatus {
  mode: Mode
  policy: PolicyRef | null
  configured: { satisfied: boolean; candidates: RequirementCandidate[] }
  health: Health
  defaultRoute: DefaultRoute | null
  endpoints: { public: string; internal: string | null }
  release: { version: string; artifactDigest: string }
  logging: { conversation: string; toolCalls: string; retention: string } | null
  nextSteps: NextStep[]
}

export type ProviderKind = string
export type ProviderState =
  | 'Serving'
  | 'Denied'
  | 'Unreachable'
  | 'NotConfigured'
  | 'PolicyExcluded'
  | 'NotInstalled'
  | 'Unknown'

export type ProviderSource =
  | { link: string; linkState: string }
  | { credentialSlot: string; bound: boolean }

export interface ModelView {
  id: string
  allowed: boolean
  default: boolean
  deniedBy: { provider: string; patterns: string[] } | null
}

export interface CredentialKeyView {
  slot: string
  bound: boolean
  state: 'usable' | 'rejected' | 'unreachable' | 'unchecked'
  observedAt: string | null
}

export interface ProviderView {
  name: string
  kind: ProviderKind
  source: ProviderSource
  state: ProviderState
  reason: string
  message: string
  observedAt: string | null
  embeddings: boolean
  models: ModelView[]
  keys: CredentialKeyView[]
}

export interface ProvidersResponse {
  items: ProviderView[]
}

export type ReportState =
  | { state: 'Pending' }
  | { state: 'Delivered' }
  | { state: 'Dropped'; reason: string; status?: number }

export interface InvocationEntry {
  id: string
  time: string
  route: Route
  caller: { subject: string; displayName: string }
  provider: string
  model: string
  outcome: Outcome
  status: number
  reason: string
  durationMs: number
  promptTokens?: number
  completionTokens?: number
  requestDigest?: string
  completionDigest?: string
  policy: PolicyRef | null
  stream: boolean
  report: ReportState
}

export interface InvocationPage {
  since: string
  instance: string
  capacity: number
  items: InvocationEntry[]
}

export interface InvocationQuery {
  outcome?: Outcome
  provider?: string
  model?: string
  caller?: 'me' | 'all'
  limit?: number
}

export interface ApiError {
  error: { code: string; message: string; retryAfterSeconds?: number }
}
