import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import type { ProviderView } from '../../api/types'
import { I18nProvider } from '../../i18n'
import { nextStepRows } from '../../model/next-steps'
import { CredentialKeys } from './providers-page'

const provider: ProviderView = {
  name: 'openai',
  kind: 'OpenAI',
  source: { credentialSlot: 'openai-secondary', bound: true },
  state: 'Serving',
  reason: '',
  message: '',
  observedAt: '2026-11-10T01:00:00Z',
  embeddings: true,
  models: [],
  keys: [
    { slot: 'openai', bound: true, state: 'rejected', observedAt: '2026-11-10T00:59:00Z' },
    { slot: 'openai-secondary', bound: true, state: 'usable', observedAt: '2026-11-10T01:00:00Z' },
    { slot: 'unbound-slot', bound: false, state: 'unchecked', observedAt: null },
  ],
}

describe('provider key management', () => {
  test('shows independent slots, last checks and the credential owner, without value inputs', () => {
    const html = renderToStaticMarkup(
      <QueryClientProvider client={new QueryClient()}>
        <I18nProvider language="ja">
          <CredentialKeys providers={[provider]} links={{ installation: 'https://dashboard.example/installation' }} />
        </I18nProvider>
      </QueryClientProvider>,
    )
    for (const text of ['OpenAI', 'openai-secondary', 'unbound-slot', '認証拒否', '利用可能', '未確認', '未設定', '2026-11-10T01:00:00Z']) expect(html).toContain(text)
    expect(html).toContain('https://dashboard.example/installation')
    expect(html).toContain('Dashboard でキーを管理する')
    expect(html.match(/<button/g)).toHaveLength(2)
    expect(html).not.toContain('<input')
  })

  test('overview remediation identifies the failed slot even when its provider is serving', () => {
    const rows = nextStepRows([{ kind: 'FixProvider', provider: 'openai', slot: 'openai' }], {})
    expect(rows[0]?.message).toBe('aiGateway.keys.fix')
    expect(rows[0]?.values).toEqual({ provider: 'openai', slot: 'openai' })
    expect(rows[0]?.place).toBe('installation')
  })
})
