// The Overview's "what to fix next" rows: the server derives and orders the
// steps (GatewayStatus.nextSteps); this maps each to its wording and the
// Dashboard place that owns the fix.
import type { NextStep } from '../api/types'
import type { MessageId } from '../copy'
import type { DashboardLinks } from './dashboard'

export type DashboardPlace = 'installation' | 'settings'

export type NextStepRow = {
  key: string
  message: MessageId
  values: Record<string, string>
  place: DashboardPlace
  /** Absent without a Dashboard origin: the row names the place as text. */
  href: string | undefined
}

/** Where in the Dashboard a fix lives, as text (shown with or without a link). */
export const PLACE_MESSAGE: Record<DashboardPlace, MessageId> = {
  installation: 'aiGateway.place.installation',
  settings: 'aiGateway.place.settings',
}

export function nextStepRows(steps: readonly NextStep[], links: DashboardLinks): NextStepRow[] {
  return steps.map((step, index) => {
    const row = (message: MessageId, values: Record<string, string>, place: DashboardPlace): NextStepRow => ({
      key: `${index}-${step.kind}`,
      message,
      values,
      place,
      href: links[place],
    })
    switch (step.kind) {
      case 'BindCredential':
        return row('aiGateway.nextStep.bindCredential', { slot: step.slot }, 'installation')
      case 'InstallApp':
        return row('aiGateway.nextStep.installApp', { app: step.definitionId }, 'installation')
      case 'BindPolicy':
        return row('aiGateway.nextStep.bindPolicy', {}, 'settings')
      case 'FixProvider':
        return step.slot
          ? row('aiGateway.keys.fix', { provider: step.provider, slot: step.slot }, 'installation')
          : row('aiGateway.nextStep.fixProvider', { provider: step.provider }, 'installation')
      case 'ContactAdmin':
        return row('aiGateway.nextStep.contactAdmin', { reason: step.reason }, 'settings')
    }
  })
}
