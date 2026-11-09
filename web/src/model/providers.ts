// Provider state → the action offered (design "Providers" state table). Every
// action is a Dashboard deep link; the gateway never writes a credential,
// installation or policy (UI1).
import type { ProviderState, ProviderView } from '../api/types'
import type { MessageId } from '../copy'
import type { Tone } from '../ui/misc'
import type { DashboardLinks } from './dashboard'
import type { DashboardPlace } from './next-steps'

export type ProviderAction = { message: MessageId; values: Record<string, string>; place: DashboardPlace; href: string | undefined }

export function providerAction(provider: ProviderView, links: DashboardLinks): ProviderAction | undefined {
  const action = (message: MessageId, place: DashboardPlace, values: Record<string, string> = {}): ProviderAction => ({ message, values, place, href: links[place] })
  switch (provider.state) {
    case 'Denied':
      return action('aiGateway.providers.action.replaceCredential', 'installation')
    case 'Unreachable':
      return action('aiGateway.providers.action.checkApp', 'installation')
    case 'NotConfigured':
      return action('aiGateway.providers.action.bindCredential', 'installation')
    case 'PolicyExcluded':
      return action('aiGateway.providers.action.viewPolicy', 'settings')
    case 'NotInstalled':
      return action('aiGateway.providers.action.install', 'installation', { app: 'link' in provider.source ? provider.source.link : provider.name })
    case 'Serving':
    case 'Unknown':
      return undefined
  }
}

export const PROVIDER_STATE_TONE: Record<ProviderState, Tone> = {
  Serving: 'ok',
  Denied: 'bad',
  Unreachable: 'bad',
  NotConfigured: 'busy',
  PolicyExcluded: 'neutral',
  NotInstalled: 'neutral',
  Unknown: 'neutral',
}

export const PROVIDER_STATE_MESSAGE: Record<ProviderState, MessageId> = {
  Serving: 'aiGateway.providerState.Serving',
  Denied: 'aiGateway.providerState.Denied',
  Unreachable: 'aiGateway.providerState.Unreachable',
  NotConfigured: 'aiGateway.providerState.NotConfigured',
  PolicyExcluded: 'aiGateway.providerState.PolicyExcluded',
  NotInstalled: 'aiGateway.providerState.NotInstalled',
  Unknown: 'aiGateway.providerState.Unknown',
}

/** Probes run for the static adapters; in-Project providers answer with their link's readiness. */
export const isStaticProvider = (provider: ProviderView) => provider.kind !== 'InProjectApp'
