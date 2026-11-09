// Overview: status strip, default route, endpoints and the derived next steps.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const overviewMessages = defineDashboardMessages({
  'aiGateway.overview.lead': {
    defaultMessage: 'What this Project’s AI Gateway does right now: the policy it follows, the providers it can reach, and what to fix next.',
    translations: { ja: 'この Project の AI Gateway のいまの状態です。従っているポリシー、使えるプロバイダー、次に直すことを確認できます。' },
  },
  'aiGateway.overview.status.title': { defaultMessage: 'At a glance', translations: { ja: '現在の状態' } },
  'aiGateway.overview.status.configured': { defaultMessage: 'Chat provider', translations: { ja: 'チャットのプロバイダー' } },
  'aiGateway.overview.status.health': { defaultMessage: 'Health', translations: { ja: 'ヘルス' } },
  'aiGateway.overview.status.release': { defaultMessage: 'Release', translations: { ja: 'リリース' } },
  'aiGateway.overview.policy.Unrestricted': {
    defaultMessage: 'No AI policy is bound to this Project. Requests go to the default provider.',
    translations: { ja: 'この Project には AI ポリシーが割り当てられていません。リクエストは既定のプロバイダーに送ります。' },
  },
  'aiGateway.overview.policy.Enforced': {
    defaultMessage: 'Every request follows the effective policy “{policy}” (generation {generation}). Providers and models outside it are refused.',
    translations: { ja: 'すべてのリクエストに、適用されたポリシー「{policy}」(世代 {generation})を当てはめます。範囲外のプロバイダーやモデルは拒否します。' },
  },
  'aiGateway.overview.policy.Pending': {
    defaultMessage: 'A policy is bound but has not reached the gateway yet. Until it does, requests are refused with ai_policy_pending.',
    translations: { ja: 'ポリシーは割り当て済みですが、まだ AI Gateway に反映されていません。反映されるまで、リクエストは ai_policy_pending で拒否されます。' },
  },
  'aiGateway.overview.policy.logging': {
    defaultMessage: 'Conversation logging: {conversation} · tool calls: {toolCalls} · retention: {retention}',
    translations: { ja: '会話の記録: {conversation} · ツール呼び出しの記録: {toolCalls} · 保持期間: {retention}' },
  },
  'aiGateway.overview.policy.manage': { defaultMessage: 'Manage the AI policy', translations: { ja: 'AI ポリシーを管理する' } },
  'aiGateway.overview.configured.byLink': {
    defaultMessage: 'Met by the in-Project model app {name}.',
    translations: { ja: 'Project 内のモデルアプリ {name} で満たしています。' },
  },
  'aiGateway.overview.configured.byCredential': {
    defaultMessage: 'Met by the {name} API key in the credentials.',
    translations: { ja: '認証情報の {name} の API キーで満たしています。' },
  },
  'aiGateway.overview.configured.anyOf': {
    defaultMessage: 'The gateway has no chat provider yet. Any one of these is enough:',
    translations: { ja: 'チャットに使えるプロバイダーがまだありません。次のどれか 1 つで使えるようになります。' },
  },
  'aiGateway.overview.configured.installLink': { defaultMessage: 'Install and connect {name}', translations: { ja: '{name} をインストールして連携する' } },
  'aiGateway.overview.route.providerDefault': { defaultMessage: '{provider} · provider default', translations: { ja: '{provider} · プロバイダー既定' } },
  'aiGateway.overview.configured.bindCredential': { defaultMessage: 'Set the {name} API key', translations: { ja: '{name} の API キーを設定する' } },
  'aiGateway.overview.health.Serving': { defaultMessage: 'The gateway answers requests normally.', translations: { ja: 'リクエストに正常に応答しています。' } },
  'aiGateway.overview.health.Degraded': {
    defaultMessage: 'Some providers are failing; requests to the others still work.',
    translations: { ja: '一部のプロバイダーで問題が起きています。ほかのプロバイダーへのリクエストは使えます。' },
  },
  'aiGateway.overview.health.Failing': { defaultMessage: 'Requests cannot be served right now.', translations: { ja: 'いまはリクエストに応答できません。' } },
  'aiGateway.overview.health.Unknown': { defaultMessage: 'The first health check has not run yet.', translations: { ja: '最初のヘルスチェックがまだ終わっていません。' } },
  'aiGateway.overview.release.noDigest': { defaultMessage: 'No artifact digest reported', translations: { ja: 'アーティファクトのダイジェストがありません' } },
  'aiGateway.overview.route.title': { defaultMessage: 'Default route', translations: { ja: '既定の送り先' } },
  'aiGateway.overview.route.lead': {
    defaultMessage: 'A request that leaves the model unset (or asks for “auto”) goes to:',
    translations: { ja: 'モデルが未指定または「auto」のリクエストは、次に送ります。' },
  },
  'aiGateway.overview.route.source.Policy': { defaultMessage: 'The policy’s default model', translations: { ja: 'ポリシーの既定のモデル' } },
  'aiGateway.overview.route.source.EnvOverride': { defaultMessage: 'Set by the gateway’s environment', translations: { ja: 'AI Gateway の環境設定で指定' } },
  'aiGateway.overview.route.source.Capability': { defaultMessage: 'The first in-Project model app', translations: { ja: 'Project 内の最初のモデルアプリ' } },
  'aiGateway.overview.route.source.Credential': { defaultMessage: 'The provider’s default for the configured API key', translations: { ja: '設定した API キーのプロバイダーの既定' } },
  'aiGateway.overview.route.none': { defaultMessage: 'No provider is available', translations: { ja: '使えるプロバイダーがありません' } },
  'aiGateway.overview.route.noneHint': {
    defaultMessage: 'A request without a model is refused until a provider is set up. See the next steps below.',
    translations: { ja: 'プロバイダーを用意するまで、モデルを指定しないリクエストは拒否されます。下の「次にすること」を確認してください。' },
  },
  'aiGateway.overview.endpoints.title': { defaultMessage: 'Endpoints', translations: { ja: 'エンドポイント' } },
  'aiGateway.overview.endpoints.connect': { defaultMessage: 'How to connect', translations: { ja: '接続のしかた' } },
  'aiGateway.overview.next.title': { defaultMessage: 'Next steps', translations: { ja: '次にすること' } },
  'aiGateway.overview.next.lead': {
    defaultMessage: 'Changes are made in the Dashboard; the gateway follows them.',
    translations: { ja: '変更は Dashboard で行います。AI Gateway はその内容に従います。' },
  },
  'aiGateway.overview.next.none': {
    defaultMessage: 'Nothing to fix. The gateway is configured and serving.',
    translations: { ja: '直すことはありません。設定済みで、稼働しています。' },
  },
  'aiGateway.nextStep.bindCredential': {
    defaultMessage: 'Set the {slot} API key in the AI Gateway credentials',
    translations: { ja: 'AI Gateway の認証情報に {slot} の API キーを設定する' },
  },
  'aiGateway.nextStep.installApp': { defaultMessage: 'Install the in-Project model app {app}', translations: { ja: 'Project 内のモデルアプリ {app} をインストールする' } },
  'aiGateway.nextStep.bindPolicy': {
    defaultMessage: 'Ask a platform admin to bind an AI policy to this Project',
    translations: { ja: 'この Project に AI ポリシーを割り当てるよう、プラットフォーム管理者に依頼する' },
  },
  'aiGateway.nextStep.fixProvider': {
    defaultMessage: 'Fix provider {provider}: check its API key or app',
    translations: { ja: 'プロバイダー {provider} を直す(API キーまたはアプリを確認)' },
  },
  'aiGateway.nextStep.contactAdmin': {
    defaultMessage: 'Ask a platform admin about the AI policy ({reason})',
    translations: { ja: 'AI ポリシーについてプラットフォーム管理者に確認する({reason})' },
  },
})
