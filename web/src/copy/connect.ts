// Connect: base URLs, how callers authenticate, and code samples.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const connectMessages = defineDashboardMessages({
  'aiGateway.connect.lead': {
    defaultMessage: 'How an app or script reaches this gateway with the OpenAI-compatible API.',
    translations: { ja: 'アプリやスクリプトから、OpenAI 互換 API でこの AI Gateway を呼び出す方法です。' },
  },
  'aiGateway.connect.urls.title': { defaultMessage: 'Base URLs', translations: { ja: 'ベース URL' } },
  'aiGateway.connect.auth.title': { defaultMessage: 'Authentication', translations: { ja: '認証' } },
  'aiGateway.connect.auth.noKey': {
    defaultMessage: 'There is no gateway API key. Send the signed-in user’s bearer token as Authorization: Bearer <token>.',
    translations: { ja: 'AI Gateway 用の API キーはありません。サインインしたユーザーのトークンを Authorization: Bearer <token> として送ります。' },
  },
  'aiGateway.connect.auth.inside': {
    defaultMessage: 'Apps in this Project forward the token of the user they act for; provider API keys stay in the gateway’s credentials.',
    translations: { ja: 'Project 内のアプリは、操作しているユーザーのトークンをそのまま渡します。プロバイダーの API キーは AI Gateway の認証情報だけが持ちます。' },
  },
  'aiGateway.connect.auth.policy': {
    defaultMessage: 'Every call is checked against the effective AI policy and recorded in the Project audit.',
    translations: { ja: 'すべての呼び出しは、適用された AI ポリシーで確認され、Project の監査記録に残ります。' },
  },
  'aiGateway.connect.snippets.title': { defaultMessage: 'Code samples', translations: { ja: 'コード例' } },
  'aiGateway.connect.snippets.lead': {
    defaultMessage: 'The model shown is this gateway’s default route; leave it out to use the default.',
    translations: { ja: '例のモデルはこの AI Gateway の既定の送り先です。省略すると既定のモデルを使います。' },
  },
  'aiGateway.connect.snippets.label.curl': { defaultMessage: 'curl', translations: { ja: 'curl' } },
  'aiGateway.connect.snippets.label.sdk': { defaultMessage: 'OpenAI SDK (JavaScript)', translations: { ja: 'OpenAI SDK(JavaScript)' } },
  'aiGateway.connect.snippets.label.env': { defaultMessage: 'Environment variable for an in-Project app', translations: { ja: 'Project 内のアプリの環境変数' } },
})
