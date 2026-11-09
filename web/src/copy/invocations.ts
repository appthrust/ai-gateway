// Invocations: the process-local ring (UI5), its filters and row detail.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const invocationMessages = defineDashboardMessages({
  'aiGateway.invocations.scope': {
    defaultMessage: 'Since {since} · this instance only ({instance}) · keeps the latest {capacity}.',
    translations: { ja: '{since} 以降 · このインスタンス({instance})のみ · 最新 {capacity} 件まで保持。' },
  },
  'aiGateway.invocations.scopeNoInstance': {
    defaultMessage: 'Since {since} · this instance only · keeps the latest {capacity}.',
    translations: { ja: '{since} 以降 · このインスタンスのみ · 最新 {capacity} 件まで保持。' },
  },
  'aiGateway.invocations.audit': { defaultMessage: 'Durable history: Dashboard › Audit', translations: { ja: '過去の記録: Dashboard › 監査' } },
  'aiGateway.invocations.auditText': {
    defaultMessage: 'The durable history is in Dashboard › Audit (team admins).',
    translations: { ja: '過去の記録は Dashboard › 監査 にあります(チーム管理者向け)。' },
  },
  'aiGateway.invocations.any': { defaultMessage: 'Any', translations: { ja: 'すべて' } },
  'aiGateway.invocations.caller.all': { defaultMessage: 'Everyone', translations: { ja: '全員' } },
  'aiGateway.invocations.caller.me': { defaultMessage: 'Only me', translations: { ja: '自分のみ' } },
  'aiGateway.invocations.clear': { defaultMessage: 'Clear filters', translations: { ja: '絞り込みを解除' } },
  'aiGateway.invocations.empty.title': { defaultMessage: 'No calls since this instance started', translations: { ja: 'このインスタンスの起動後、まだ呼び出しはありません' } },
  'aiGateway.invocations.empty.body': {
    defaultMessage: 'The first call from an app or the Playground appears here.',
    translations: { ja: 'アプリやお試し画面から最初に呼び出すと、ここに表示されます。' },
  },
  'aiGateway.invocations.noMatch.title': { defaultMessage: 'No calls match these filters', translations: { ja: '条件に合う呼び出しはありません' } },
  'aiGateway.invocations.noMatch.body': { defaultMessage: 'Change or clear the filters.', translations: { ja: '絞り込みを変えるか、解除してください。' } },
  'aiGateway.invocations.showDetail': { defaultMessage: 'Show details', translations: { ja: '詳細を表示' } },
  'aiGateway.invocations.hideDetail': { defaultMessage: 'Hide details', translations: { ja: '詳細を隠す' } },
  'aiGateway.invocations.unnamed': { defaultMessage: 'Unnamed caller', translations: { ja: '名前なし' } },
  'aiGateway.invocations.stream': { defaultMessage: 'streamed', translations: { ja: 'ストリーミング' } },
  'aiGateway.invocations.noDigest': {
    defaultMessage: 'None (kept only when the policy logs conversations as hash only)',
    translations: { ja: 'なし(会話の記録が「ハッシュのみ」のポリシーで残します)' },
  },
  'aiGateway.invocations.noPolicy': { defaultMessage: 'None (unrestricted)', translations: { ja: 'なし(制限なし)' } },
})
