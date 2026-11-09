import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import type { NextStep } from '../../api/types'
import { I18nProvider } from '../../i18n'
import { dashboardLinks } from '../../model/dashboard'
import { nextStepRows } from '../../model/next-steps'
import { NextSteps } from './overview-page'

const steps: NextStep[] = [
  { kind: 'BindCredential', slot: 'openai' },
  { kind: 'InstallApp', definitionId: 'inference' },
  { kind: 'BindPolicy' },
  { kind: 'FixProvider', provider: 'openai' },
  { kind: 'ContactAdmin', reason: 'FullLogging' },
]
const withDashboard = dashboardLinks({ projectId: 'pj-eee', publicUrl: '', dashboardUrl: 'https://dashboard.appthrust.ink' })
const installation = 'https://dashboard.appthrust.ink/dashboard/projects/pj-eee/marketplace/ai-gateway'
const settings = 'https://dashboard.appthrust.ink/dashboard/projects/pj-eee/settings'
const render = (links: typeof withDashboard, list: NextStep[] = steps, language: 'ja' | 'en' = 'ja') =>
  renderToStaticMarkup(
    <I18nProvider language={language}>
      <NextSteps status={{ nextSteps: list }} links={links} />
    </I18nProvider>
  )

describe('overview next steps', () => {
  test('each step deep-links to the Dashboard place that owns its fix, in server order', () => {
    expect(nextStepRows(steps, withDashboard).map((row) => [row.message, row.href])).toEqual([
      ['aiGateway.nextStep.bindCredential', installation],
      ['aiGateway.nextStep.installApp', installation],
      ['aiGateway.nextStep.bindPolicy', settings],
      ['aiGateway.nextStep.fixProvider', installation],
      ['aiGateway.nextStep.contactAdmin', settings],
    ])
  })

  test('the list shows each step as a link with its Dashboard place', () => {
    const html = render(withDashboard)
    expect(html).toContain(`href="${installation}"`)
    expect(html).toContain(`href="${settings}"`)
    expect(html).toContain('AI Gateway の認証情報に openai の API キーを設定する')
    expect(html).toContain('Project 内のモデルアプリ inference をインストールする')
    expect(html).toContain('この Project に AI ポリシーを割り当てるよう、プラットフォーム管理者に依頼する')
    expect(html).toContain('AI ポリシーについてプラットフォーム管理者に確認する(FullLogging)')
    expect(render(withDashboard, steps, 'en')).toContain('Fix provider openai: check its API key or app')
  })

  test('without a Dashboard origin the steps are plain text naming the place', () => {
    const html = render(dashboardLinks({ projectId: 'pj-eee', publicUrl: '' }))
    expect(html).not.toContain('href=')
    expect(html).toContain('Dashboard › マーケットプレイス › AI Gateway')
    expect(html).toContain('Dashboard › Project 設定 › AI')
  })

  test('no steps reads as nothing to fix', () => {
    expect(render(withDashboard, [])).toContain('直すことはありません。設定済みで、稼働しています。')
  })
})
