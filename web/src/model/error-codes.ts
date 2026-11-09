// Gateway error code → its product meaning (design "Playground" error table).
// The same table explains a Playground failure and an invocation's `reason`.
import type { Outcome } from '../api/types'
import type { MessageId } from '../copy'
import type { DashboardPlace } from './next-steps'

export type ErrorExplanation = {
  title: MessageId
  body: MessageId
  /** Where the fix lives, when it is in the Dashboard. */
  place?: DashboardPlace
}

const EXPLANATIONS: Record<string, ErrorExplanation> = {
  ai_model_denied: { title: 'aiGateway.error.modelDenied.title', body: 'aiGateway.error.modelDenied.body', place: 'settings' },
  ai_policy_pending: { title: 'aiGateway.error.policyPending.title', body: 'aiGateway.error.policyPending.body', place: 'settings' },
  ai_policy_logging_unavailable: { title: 'aiGateway.error.loggingUnavailable.title', body: 'aiGateway.error.loggingUnavailable.body', place: 'settings' },
  ai_provider_error: { title: 'aiGateway.error.providerError.title', body: 'aiGateway.error.providerError.body', place: 'installation' },
  identity_unavailable: { title: 'aiGateway.error.identityUnavailable.title', body: 'aiGateway.error.identityUnavailable.body' },
  ai_unauthorized: { title: 'aiGateway.error.unauthorized.title', body: 'aiGateway.error.unauthorized.body' },
}

const UNKNOWN: ErrorExplanation = { title: 'aiGateway.error.unknown.title', body: 'aiGateway.error.unknown.body' }
/** No answer at all (network, proxy 5xx without the gateway envelope). */
const UNANSWERED: ErrorExplanation = { title: 'aiGateway.error.unanswered.title', body: 'aiGateway.error.unanswered.body' }

export function explainError(code: string, transient = false): ErrorExplanation {
  if (transient) return UNANSWERED
  return Object.hasOwn(EXPLANATIONS, code) ? EXPLANATIONS[code]! : UNKNOWN
}

// The gateway records a policy refusal as Denied; anything else that is not a success is an Error.
const DENIED_CODES: Record<string, true> = { ai_model_denied: true, ai_policy_pending: true, ai_policy_logging_unavailable: true }

/** The outcome the gateway records for an answer with this status and code. */
export function outcomeOf(status: number, code: string | undefined): Outcome {
  if (status >= 200 && status < 300) return 'Allowed'
  return code !== undefined && Object.hasOwn(DENIED_CODES, code) ? 'Denied' : 'Error'
}
