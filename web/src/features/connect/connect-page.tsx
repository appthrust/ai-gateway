import { useSuspenseQuery } from '@tanstack/react-query'
import { statusQuery } from '../../api/queries'
import type { GatewayStatus } from '../../api/types'
import { useText } from '../../i18n'
import { Card, CodeBlock, CopyField, PageHeader, Section, useDocumentTitle } from '../../ui'

type Snippet = 'curl' | 'sdk' | 'env'

/** The sample model: the default route's, else none (the gateway then resolves the default itself). */
function snippets(status: GatewayStatus): Record<Snippet, string> {
  const base = status.endpoints.public
  const inside = status.endpoints.internal ?? base
  const model = status.defaultRoute?.model
  const body = JSON.stringify({ ...(model ? { model } : {}), messages: [{ role: 'user', content: 'Hello' }] })
  return {
    curl: [
      `curl ${base}/chat/completions \\`,
      `  -H "Authorization: Bearer $TOKEN" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body}'`,
    ].join('\n'),
    sdk: [
      `import OpenAI from 'openai'`,
      ``,
      `const client = new OpenAI({`,
      `  baseURL: '${base}',`,
      `  // The signed-in user's bearer token, not a provider API key.`,
      `  apiKey: userAccessToken,`,
      `})`,
      ``,
      `const completion = await client.chat.completions.create({`,
      ...(model ? [`  model: '${model}',`] : []),
      `  messages: [{ role: 'user', content: 'Hello' }],`,
      `})`,
    ].join('\n'),
    env: [`# An app in this Project (Knowledge-style): replace MYAPP with the app's prefix.`, `MYAPP_LLM_URL=${inside}`].join('\n'),
  }
}

export function ConnectPage() {
  const { t } = useText()
  const { data: status } = useSuspenseQuery(statusQuery)
  useDocumentTitle(t('aiGateway.nav.connect'))
  const code = snippets(status)
  return (
    <>
      <PageHeader title={t('aiGateway.nav.connect')} lead={t('aiGateway.connect.lead')} />
      <Section title={t('aiGateway.connect.urls.title')}>
        <Card className="space-y-4 p-4">
          <div>
            <p className="text-sm font-medium text-stone-800">{t('aiGateway.endpoints.public')}</p>
            <p className="mb-1.5 text-xs text-stone-500">{t('aiGateway.endpoints.publicHint')}</p>
            <CopyField value={status.endpoints.public} label={t('aiGateway.endpoints.public')} />
          </div>
          <div>
            <p className="text-sm font-medium text-stone-800">{t('aiGateway.endpoints.internal')}</p>
            <p className="mb-1.5 text-xs text-stone-500">{t('aiGateway.endpoints.internalHint')}</p>
            {status.endpoints.internal ? (
              <CopyField value={status.endpoints.internal} label={t('aiGateway.endpoints.internal')} />
            ) : (
              <p className="text-sm text-stone-500">{t('aiGateway.endpoints.internalUnknown')}</p>
            )}
          </div>
        </Card>
      </Section>
      <Section title={t('aiGateway.connect.auth.title')}>
        <Card className="space-y-2 p-4 text-sm text-stone-700">
          <p>{t('aiGateway.connect.auth.noKey')}</p>
          <p>{t('aiGateway.connect.auth.inside')}</p>
          <p>{t('aiGateway.connect.auth.policy')}</p>
        </Card>
      </Section>
      <Section title={t('aiGateway.connect.snippets.title')} lead={t('aiGateway.connect.snippets.lead')}>
        <div className="space-y-5">
          {(['curl', 'sdk', 'env'] as const).map((kind) => (
            <div key={kind}>
              <h3 className="mb-1.5 text-sm font-medium text-stone-800">{t(`aiGateway.connect.snippets.label.${kind}`)}</h3>
              <CodeBlock code={code[kind]} label={t(`aiGateway.connect.snippets.label.${kind}`)} />
            </div>
          ))}
        </div>
      </Section>
    </>
  )
}
