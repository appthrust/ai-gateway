import { describe, expect, test } from 'bun:test'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nProvider } from '../../i18n'
import { dashboardLinks } from '../../model/dashboard'
import { outcomeOf } from '../../model/error-codes'
import { ErrorExplanation } from './error-explanation'

const render = (node: ReactNode, language: 'ja' | 'en' = 'ja') => renderToStaticMarkup(<I18nProvider language={language}>{node}</I18nProvider>)
const links = dashboardLinks({ projectId: 'pj-1', publicUrl: '', dashboardUrl: 'https://dashboard.example/' })

describe('gateway error codes', () => {
  test('ai_model_denied names the model, provider and rule, and links the policy', () => {
    const html = render(<ErrorExplanation code="ai_model_denied" status={403} message="denied" context={{ provider: 'openai', model: 'gpt-4o', patterns: ['gpt-5*'] }} links={links} />)
    expect(html).toContain('role="alert"')
    expect(html).toContain('このモデルは適用されたポリシーにありません')
    expect(html).toContain('openai の gpt-4o は AI ポリシーの範囲外です(ルール: gpt-5*)')
    expect(html).toContain('href="https://dashboard.example/dashboard/projects/pj-1/settings"')
    expect(html).toContain('ai_model_denied · HTTP 403')
  })

  test('each documented code has its own product meaning in both languages', () => {
    const cases: [string, number, string, string][] = [
      ['ai_policy_pending', 403, 'ポリシーがまだ反映されていません', 'The policy is not applied yet'],
      ['ai_policy_logging_unavailable', 403, '会話の全文記録ができません', 'Full conversation logging is unavailable'],
      ['ai_provider_error', 502, 'プロバイダーでエラーが起きました', 'The provider failed'],
      ['identity_unavailable', 503, 'サインインの準備ができていません', 'Sign-in is not ready yet'],
      ['ai_unauthorized', 401, 'セッションが受け付けられませんでした', 'Your session was not accepted'],
    ]
    for (const [code, status, ja, en] of cases) {
      const node = <ErrorExplanation code={code} status={status} context={{ provider: 'openai' }} links={links} />
      expect(render(node), code).toContain(ja)
      expect(render(node, 'en'), code).toContain(en)
    }
    expect(render(<ErrorExplanation code="ai_provider_error" status={502} context={{ provider: 'openai' }} links={links} />)).toContain('openai から正常な応答がありませんでした')
  })

  test('without a Dashboard origin the fix is named as text; unknown and unanswered calls stay honest', () => {
    const noLink = render(<ErrorExplanation code="ai_policy_pending" status={403} context={{}} links={{}} />)
    expect(noLink).not.toContain('href=')
    expect(noLink).toContain('Dashboard › Project 設定 › AI')
    expect(render(<ErrorExplanation code="ai_something_new" status={418} context={{}} links={links} />)).toContain('呼び出しに失敗しました(ai_something_new)')
    expect(render(<ErrorExplanation code="unknown" status={0} transient context={{}} links={links} />)).toContain('AI Gateway から応答がありません')
  })

  test('outcomes follow the gateway: policy refusals are Denied, other failures Error', () => {
    expect(outcomeOf(200, undefined)).toBe('Allowed')
    expect(outcomeOf(403, 'ai_model_denied')).toBe('Denied')
    expect(outcomeOf(403, 'ai_policy_pending')).toBe('Denied')
    expect(outcomeOf(502, 'ai_provider_error')).toBe('Error')
    expect(outcomeOf(503, 'identity_unavailable')).toBe('Error')
  })
})
