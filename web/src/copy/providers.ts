// Providers table, model lists, state legend and re-check.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const providerMessages = defineDashboardMessages({
  'aiGateway.providers.lead': {
    defaultMessage: 'Every provider the gateway knows, whether it can serve right now, and which of its models the policy allows.',
    translations: { ja: 'AI Gateway が把握しているプロバイダーと、いま使えるかどうか、ポリシーで許可しているモデルです。' },
  },
  'aiGateway.providers.col.kind': { defaultMessage: 'Kind', translations: { ja: '種類' } },
  'aiGateway.providers.col.source': { defaultMessage: 'Source', translations: { ja: '接続元' } },
  'aiGateway.providers.col.state': { defaultMessage: 'State', translations: { ja: '状態' } },
  'aiGateway.providers.col.models': { defaultMessage: 'Allowed models', translations: { ja: '許可モデル' } },
  'aiGateway.providers.col.checked': { defaultMessage: 'Last checked', translations: { ja: '最終確認' } },
  'aiGateway.providers.col.actions': { defaultMessage: 'Actions', translations: { ja: '操作' } },
  'aiGateway.providers.kind.InProjectApp': { defaultMessage: 'In-Project app', translations: { ja: 'Project 内のアプリ' } },
  'aiGateway.providers.kind.OpenAI': { defaultMessage: 'OpenAI', translations: { ja: 'OpenAI' } },
  'aiGateway.providers.kind.Anthropic': { defaultMessage: 'Anthropic', translations: { ja: 'Anthropic' } },
  'aiGateway.providers.source.link': { defaultMessage: 'Connected app {link}', translations: { ja: '連携アプリ {link}' } },
  'aiGateway.providers.source.slot': { defaultMessage: 'API key “{slot}”', translations: { ja: 'API キー「{slot}」' } },
  'aiGateway.providers.source.bound': { defaultMessage: 'Set', translations: { ja: '設定済み' } },
  'aiGateway.providers.source.unbound': { defaultMessage: 'Not set', translations: { ja: '未設定' } },
  'aiGateway.providers.modelCount': { defaultMessage: '{allowed} of {total}', translations: { ja: '{total} 件中 {allowed} 件' } },
  'aiGateway.providers.neverChecked': { defaultMessage: 'Never', translations: { ja: '未確認' } },
  'aiGateway.providers.showModels': { defaultMessage: 'Show models', translations: { ja: 'モデルを表示' } },
  'aiGateway.providers.hideModels': { defaultMessage: 'Hide models', translations: { ja: 'モデルを隠す' } },
  'aiGateway.providers.noModels': {
    defaultMessage: 'No models listed. The provider could not be asked for its models, or it has none.',
    translations: { ja: 'モデルがありません。プロバイダーからモデル一覧を取得できなかったか、モデルがありません。' },
  },
  'aiGateway.providers.embeddings.yes': { defaultMessage: 'Supports chat completions and embeddings.', translations: { ja: 'チャット補完と埋め込みに対応しています。' } },
  'aiGateway.providers.embeddings.no': { defaultMessage: 'Supports chat completions only.', translations: { ja: 'チャット補完のみに対応しています。' } },
  'aiGateway.providers.empty.title': { defaultMessage: 'No providers yet', translations: { ja: 'プロバイダーがまだありません' } },
  'aiGateway.providers.empty.body': {
    defaultMessage: 'Install an in-Project model app such as LLM, or set an OpenAI or Anthropic API key in the Dashboard.',
    translations: { ja: 'LLM など Project 内のモデルアプリをインストールするか、Dashboard で OpenAI または Anthropic の API キーを設定してください。' },
  },
  'aiGateway.providers.action.replaceCredential': { defaultMessage: 'Replace the API key', translations: { ja: 'API キーを差し替える' } },
  'aiGateway.providers.action.checkApp': { defaultMessage: 'Check the provider app', translations: { ja: 'プロバイダーのアプリを確認する' } },
  'aiGateway.providers.action.bindCredential': { defaultMessage: 'Set the API key', translations: { ja: 'API キーを設定する' } },
  'aiGateway.providers.action.viewPolicy': { defaultMessage: 'View the policy', translations: { ja: 'ポリシーを見る' } },
  'aiGateway.providers.action.install': { defaultMessage: 'Install {app}', translations: { ja: '{app} をインストールする' } },
  'aiGateway.providers.recheck': { defaultMessage: 'Re-check now', translations: { ja: 'いま確認する' } },
  'aiGateway.providers.recheckCached': {
    defaultMessage: 'Shown result is from {age}; a new check is possible in {seconds} s.',
    translations: { ja: '表示中の結果は{age}のものです。{seconds} 秒後に再確認できます。' },
  },
  'aiGateway.providers.recheckFailed': { defaultMessage: 'The check could not run. Try again later.', translations: { ja: '確認できませんでした。しばらくしてからお試しください。' } },
  'aiGateway.providers.legend.title': { defaultMessage: 'What the states mean', translations: { ja: '状態の意味' } },
  'aiGateway.providers.legend.Serving': {
    defaultMessage: 'The health check or a real call succeeded in the last 5 minutes.',
    translations: { ja: '直近 5 分以内に、ヘルスチェックか実際の呼び出しが成功しました。' },
  },
  'aiGateway.providers.legend.Denied': {
    defaultMessage: 'The provider rejected the API key (401/403). Replace it in the Dashboard.',
    translations: { ja: 'プロバイダーが API キーを拒否しました(401/403)。Dashboard で差し替えてください。' },
  },
  'aiGateway.providers.legend.Unreachable': {
    defaultMessage: 'The provider could not be reached or answered with a server error.',
    translations: { ja: 'プロバイダーに接続できないか、サーバーエラーが返りました。' },
  },
  'aiGateway.providers.legend.NotConfigured': { defaultMessage: 'No API key is set for this provider.', translations: { ja: 'このプロバイダーの API キーが設定されていません。' } },
  'aiGateway.providers.legend.PolicyExcluded': {
    defaultMessage: 'The effective policy does not list this provider, so requests to it are refused.',
    translations: { ja: '適用されたポリシーにこのプロバイダーがないため、リクエストは拒否されます。' },
  },
  'aiGateway.providers.legend.NotInstalled': {
    defaultMessage: 'The policy or catalog names this app, but it is not installed and connected.',
    translations: { ja: 'ポリシーやカタログにこのアプリがありますが、インストール・連携されていません。' },
  },
  'aiGateway.providers.legend.Unknown': { defaultMessage: 'The provider has not been checked since the gateway started.', translations: { ja: 'AI Gateway の起動後、まだ確認していません。' } },
})
