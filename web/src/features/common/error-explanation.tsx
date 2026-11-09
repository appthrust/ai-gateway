import type { DashboardLinks } from '../../model/dashboard'
import { explainError } from '../../model/error-codes'
import { useText } from '../../i18n'
import { Notice } from '../../ui'
import { FixLink } from './fix-link'

export type ErrorContext = {
  /** The provider the call went to (the picked one, else the default route's). */
  provider?: string
  model?: string
  /** `ai_model_denied`: the policy rule's patterns that excluded the model, when the providers view names them. */
  patterns?: string[]
}

/**
 * A gateway error as its product meaning (design "Playground" error table): what happened, what to do, where the
 * fix lives, and the code itself for a support conversation. The gateway's message is shown as a hint; it never
 * carries a credential, an upstream URL or a body (UI3).
 */
export function ErrorExplanation({ code, status, message, transient = false, context, links }: { code: string; status: number; message?: string; transient?: boolean; context: ErrorContext; links: DashboardLinks }) {
  const { t } = useText()
  const explanation = explainError(code, transient)
  const values = {
    code,
    status,
    provider: context.provider ?? t('aiGateway.common.defaultProvider'),
    model: context.model ?? t('aiGateway.common.defaultModel'),
    rule: context.patterns && context.patterns.length > 0 ? context.patterns.join(', ') : t('aiGateway.error.modelDenied.noRule'),
  }
  return (
    <Notice
      tone="error"
      title={t(explanation.title, values)}
      actions={explanation.place ? <FixLink text={t(explanation.place === 'settings' ? 'aiGateway.error.fix.policy' : 'aiGateway.error.fix.provider')} place={explanation.place} href={links[explanation.place]} /> : undefined}
    >
      <p>{t(explanation.body, values)}</p>
      <p className="mt-1 text-xs opacity-80">
        <span className="font-mono">{transient ? `HTTP ${status}` : `${code} · HTTP ${status}`}</span>
        {message && !transient ? <span> — {message}</span> : null}
      </p>
    </Notice>
  )
}
