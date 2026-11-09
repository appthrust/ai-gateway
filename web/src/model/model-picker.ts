// The Playground's model picker: what `GET /v1/models` lists (what consumer
// apps see) plus every model the providers view knows, so a model the policy
// excludes or a provider that cannot serve stays visible, greyed, with its
// reason instead of silently missing.
import type { OpenAIModel } from '../api/client'
import type { DefaultRoute, ProviderState, ProviderView } from '../api/types'

export type BlockReason =
  /** `allowed=false` in the effective policy; `patterns` are the first failing rule's (may be empty). */
  | { kind: 'policy'; provider: string; patterns: string[] }
  /** The provider resolved but cannot serve right now (Denied, Unreachable, …). */
  | { kind: 'provider'; provider: string; state: ProviderState }

export type PickerOption = {
  /** Stable option value (provider and model); see `optionValue`. */
  value: string
  /** Sent as `provider`; null when no provider claims the listed model (the gateway resolves it). */
  provider: string | null
  model: string
  isDefault: boolean
  blocked: BlockReason | null
}

/** JSON keeps any character a model id may hold (`:`, `/`, `|`). */
export const optionValue = (provider: string | null, model: string) => JSON.stringify([provider, model])

// States that make a provider's models unusable now. `Unknown` (not probed yet) is not a reason to block.
const UNSERVABLE: Record<ProviderState, boolean> = {
  Serving: false,
  Unknown: false,
  Denied: true,
  Unreachable: true,
  NotConfigured: true,
  PolicyExcluded: true,
  NotInstalled: true,
}

/**
 * Options for `route`: chat uses every provider, embeddings only providers with `embeddings`. Order: the default
 * route first, then usable models, then blocked ones; each in providers-view order, then listing order.
 */
export function pickerOptions(
  listed: readonly OpenAIModel[],
  providers: readonly ProviderView[],
  defaultRoute: DefaultRoute | null,
  route: 'chat' | 'embeddings' = 'chat'
): PickerOption[] {
  const usable = route === 'chat' ? providers : providers.filter((provider) => provider.embeddings)
  const rank = new Map(usable.map((provider, index) => [provider.name, index]))
  const options: PickerOption[] = []
  const seen = new Set<string>()
  // An empty `defaultRoute.model` means "the provider's own default": the model that provider marks `default`.
  const isDefault = (provider: ProviderView | undefined, providerName: string | null, model: string) => {
    if (!defaultRoute) return false
    if (defaultRoute.model) return defaultRoute.model === model && (providerName === null || defaultRoute.provider === providerName)
    return providerName === defaultRoute.provider && (provider?.models.some((candidate) => candidate.id === model && candidate.default) ?? false)
  }
  const add = (provider: ProviderView | undefined, providerName: string | null, model: string, blocked: BlockReason | null) => {
    const value = optionValue(providerName, model)
    if (seen.has(value)) return
    seen.add(value)
    options.push({
      value,
      provider: providerName,
      model,
      isDefault: isDefault(provider, providerName, model),
      blocked: blocked ?? (provider && UNSERVABLE[provider.state] ? { kind: 'provider', provider: provider.name, state: provider.state } : null),
    })
  }
  // The listing first: these ids are what a consumer may ask for. The owner is the provider whose models hold
  // the id (in-Project lists report their own `owned_by`), else the provider named by `owned_by`.
  for (const entry of listed) {
    const owner =
      usable.find((provider) => provider.models.some((model) => model.id === entry.id)) ?? usable.find((provider) => provider.name === entry.owned_by)
    if (!owner && route === 'embeddings' && providers.some((provider) => provider.name === entry.owned_by || provider.models.some((model) => model.id === entry.id))) continue
    add(owner, owner?.name ?? null, entry.id, null)
  }
  for (const provider of usable) {
    for (const model of provider.models) {
      add(provider, provider.name, model.id, model.allowed ? null : { kind: 'policy', provider: model.deniedBy?.provider ?? provider.name, patterns: model.deniedBy?.patterns ?? [] })
    }
  }
  // Stable sort: ties keep listing order.
  const group = (option: PickerOption) => (option.blocked ? 2 : option.isDefault ? 0 : 1)
  const position = (option: PickerOption) => (option.provider === null ? usable.length : (rank.get(option.provider) ?? usable.length))
  return options.sort((a, b) => group(a) - group(b) || position(a) - position(b))
}

/** The option selected when the tab opens: the default route if usable, else the first usable model. */
export const initialOption = (options: readonly PickerOption[]) => options.find((option) => option.isDefault && !option.blocked) ?? options.find((option) => !option.blocked)
