import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import type { OpenAIModel } from '../../api/client'
import type { ProviderView } from '../../api/types'
import { createMockGateway } from '../../dev/mock-gateway'
import { I18nProvider } from '../../i18n'
import { initialOption, pickerOptions, type PickerOption } from '../../model/model-picker'
import { ModelOptionLabel } from './model-picker'

const llm: ProviderView = {
  name: 'llm',
  kind: 'InProjectApp',
  source: { link: 'llm', linkState: 'Ready' },
  state: 'Serving',
  reason: '',
  message: '',
  observedAt: '2026-11-09T10:00:00Z',
  embeddings: false,
  models: [
    { id: 'gemma3:4b', allowed: true, default: true, deniedBy: null },
    { id: 'llama3.1:70b', allowed: false, default: false, deniedBy: { provider: 'llm', patterns: ['gemma3:*'] } },
  ],
}
const openai: ProviderView = {
  name: 'openai',
  kind: 'OpenAI',
  source: { credentialSlot: 'openai', bound: true },
  state: 'Denied',
  reason: 'UpstreamDenied',
  message: '',
  observedAt: '2026-11-09T09:58:00Z',
  embeddings: true,
  models: [{ id: 'gpt-5-mini', allowed: true, default: false, deniedBy: null }],
}
// What GET /v1/models returns under Enforced: only allowed models of providers that could list.
const listed: OpenAIModel[] = [{ id: 'gemma3:4b', object: 'model', owned_by: 'llm' }]

const byModel = (options: PickerOption[], model: string) => options.find((option) => option.model === model)
const label = (option: PickerOption, language: 'ja' | 'en' = 'ja') =>
  renderToStaticMarkup(
    <I18nProvider language={language}>
      <ModelOptionLabel option={option} />
    </I18nProvider>
  )

describe('playground model picker', () => {
  const options = pickerOptions(listed, [llm, openai], { provider: 'llm', model: 'gemma3:4b', source: 'Policy' })

  test('the default route is first and pre-selected; blocked models sort last', () => {
    expect(options.map((option) => [option.provider, option.model, option.blocked?.kind ?? null])).toEqual([
      ['llm', 'gemma3:4b', null],
      ['llm', 'llama3.1:70b', 'policy'],
      ['openai', 'gpt-5-mini', 'provider'],
    ])
    expect(initialOption(options)?.model).toBe('gemma3:4b')
  })

  test('a model the policy excludes is blocked with the rule that excluded it', () => {
    expect(byModel(options, 'llama3.1:70b')?.blocked).toEqual({ kind: 'policy', provider: 'llm', patterns: ['gemma3:*'] })
    expect(label(byModel(options, 'llama3.1:70b')!)).toContain('llm のルール gemma3:* で除外')
    expect(label(byModel(options, 'llama3.1:70b')!, 'en')).toContain('excluded by the llm rule gemma3:*')
  })

  test('an allowed model of a provider that rejected its key is blocked with the provider state', () => {
    expect(byModel(options, 'gpt-5-mini')?.blocked).toEqual({ kind: 'provider', provider: 'openai', state: 'Denied' })
    expect(label(byModel(options, 'gpt-5-mini')!)).toContain('プロバイダー openai: 認証エラー')
  })

  test('an empty default-route model selects the model the provider marks default', () => {
    const providerDefault = pickerOptions(listed, [llm, openai], { provider: 'llm', model: '', source: 'Capability' })
    expect(initialOption(providerDefault)?.model).toBe('gemma3:4b')
    expect(byModel(providerDefault, 'gemma3:4b')?.isDefault).toBe(true)
  })

  test('nothing usable leaves no initial option, so the call omits the model', () => {
    const none = pickerOptions([], [openai], null)
    expect(none.every((option) => option.blocked)).toBe(true)
    expect(initialOption(none)).toBeUndefined()
  })

  test('embeddings only offer providers that embed', () => {
    const embeddings = pickerOptions(listed, [llm, openai], null, 'embeddings')
    expect(embeddings.map((option) => option.provider)).toEqual(['openai'])
  })

  test('the mock fixtures give the picker usable, policy-blocked and provider-blocked rows', async () => {
    const gateway = createMockGateway()
    const call = async (path: string) => (await gateway(new Request(`http://mock${path}`), new URL(`http://mock${path}`)))!.json()
    const models = (await call('/v1/models')) as { data: OpenAIModel[] }
    const providers = ((await call('/api/ai-gateway/v1/providers')) as { items: ProviderView[] }).items
    const mockOptions = pickerOptions(models.data, providers, { provider: 'llm', model: 'gemma3:4b', source: 'Policy' })
    expect(initialOption(mockOptions)?.value).toBe(JSON.stringify(['llm', 'gemma3:4b']))
    expect(byModel(mockOptions, 'gpt-4o')?.blocked?.kind).toBe('policy')
    expect(byModel(mockOptions, 'gpt-5-mini')?.blocked?.kind).toBe('provider')
  })
})
