// Playground: tabs, model picker, result panel, and the error-code table shared with Invocations.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const playgroundMessages = defineDashboardMessages({
  'aiGateway.playground.lead': {
    defaultMessage: 'Try chat completions, embeddings and the model list against this gateway, to see what an app would get.',
    translations: { ja: 'チャット補完、埋め込み、モデル一覧をこの AI Gateway に送って、アプリが受け取る結果を確かめられます。' },
  },
  'aiGateway.playground.notice.title': { defaultMessage: 'These are real calls made as you', translations: { ja: 'あなたとして実際に呼び出します' } },
  'aiGateway.playground.notice.body': {
    defaultMessage: 'Each call goes through the public API with your session and the effective policy, and is recorded in the Project audit like any app’s call.',
    translations: { ja: '呼び出しは、あなたのセッションで公開 API を通り、適用されたポリシーで確認されます。アプリからの呼び出しと同じく、Project の監査記録に残ります。' },
  },
  'aiGateway.playground.tabs': { defaultMessage: 'Call type', translations: { ja: '呼び出しの種類' } },
  'aiGateway.playground.tab.chat': { defaultMessage: 'Chat', translations: { ja: 'チャット' } },
  'aiGateway.playground.tab.embeddings': { defaultMessage: 'Embeddings', translations: { ja: '埋め込み' } },
  'aiGateway.playground.tab.models': { defaultMessage: 'Models', translations: { ja: 'モデル一覧' } },
  'aiGateway.playground.system': { defaultMessage: 'System message', translations: { ja: 'システムメッセージ' } },
  'aiGateway.playground.systemHint': { defaultMessage: 'Optional. Instructions sent before your message.', translations: { ja: '任意。メッセージの前に送る指示です。' } },
  'aiGateway.playground.user': { defaultMessage: 'Message', translations: { ja: 'メッセージ' } },
  'aiGateway.playground.userPlaceholder': { defaultMessage: 'Write a short test prompt', translations: { ja: '短いテスト用のプロンプトを入力' } },
  'aiGateway.playground.stream': { defaultMessage: 'Stream the reply', translations: { ja: '応答をストリーミングで受け取る' } },
  'aiGateway.playground.input': { defaultMessage: 'Text to embed', translations: { ja: '埋め込むテキスト' } },
  'aiGateway.playground.send': { defaultMessage: 'Send', translations: { ja: '送信' } },
  'aiGateway.playground.stop': { defaultMessage: 'Stop', translations: { ja: '停止' } },
  'aiGateway.playground.modelsLead': {
    defaultMessage: 'Calls GET /v1/models: the models a consumer app sees under the effective policy.',
    translations: { ja: 'GET /v1/models を呼び出します。適用されたポリシーのもとで、アプリから見えるモデルの一覧です。' },
  },
  'aiGateway.playground.listModels': { defaultMessage: 'List models', translations: { ja: 'モデル一覧を取得' } },
  'aiGateway.playground.idle.title': { defaultMessage: 'No call yet', translations: { ja: 'まだ呼び出していません' } },
  'aiGateway.playground.idle.body': {
    defaultMessage: 'Send a call to see the reply and its invocation record here.',
    translations: { ja: '呼び出すと、応答とその呼び出しの記録がここに表示されます。' },
  },
  'aiGateway.playground.output': { defaultMessage: 'Reply', translations: { ja: '応答' } },
  'aiGateway.playground.waiting': { defaultMessage: 'Waiting for the reply…', translations: { ja: '応答を待っています…' } },
  'aiGateway.playground.noText': { defaultMessage: 'The reply had no text.', translations: { ja: '応答にテキストがありませんでした。' } },
  'aiGateway.playground.stopped': { defaultMessage: 'Stopped before the reply finished.', translations: { ja: '応答の途中で停止しました。' } },
  'aiGateway.playground.vector': { defaultMessage: '{count} vector(s) of {dimensions} dimensions', translations: { ja: '{dimensions} 次元のベクトル {count} 件' } },
  'aiGateway.playground.modelsCount': { defaultMessage: '{count} models listed', translations: { ja: '{count} 件のモデル' } },
  'aiGateway.playground.modelsPartial': {
    defaultMessage: 'Some providers could not list their models',
    translations: { ja: '一部のプロバイダーからモデル一覧を取得できませんでした' },
  },
  'aiGateway.playground.result': { defaultMessage: 'Invocation record', translations: { ja: '呼び出しの記録' } },
  'aiGateway.playground.durationHint': { defaultMessage: 'Measured in this browser', translations: { ja: 'このブラウザで計測' } },
  'aiGateway.playground.record.loading': { defaultMessage: 'Looking up this instance’s record…', translations: { ja: 'このインスタンスの記録を探しています…' } },
  'aiGateway.playground.record.found': { defaultMessage: 'Matched this instance’s record from {time}.', translations: { ja: 'このインスタンスの {time} の記録と照合しました。' } },
  'aiGateway.playground.record.missing': {
    defaultMessage: 'No matching record on this instance; another instance may have served the call.',
    translations: { ja: 'このインスタンスに該当する記録がありません。別のインスタンスが応答した可能性があります。' },
  },
  'aiGateway.playground.record.open': { defaultMessage: 'Open invocations', translations: { ja: '呼び出し一覧を開く' } },

  'aiGateway.picker.hint': { defaultMessage: 'The default route is selected first.', translations: { ja: '既定の送り先を最初に選んでいます。' } },
  'aiGateway.picker.loading': { defaultMessage: 'Loading the model list…', translations: { ja: 'モデル一覧を読み込んでいます…' } },
  'aiGateway.picker.blockedCount': {
    defaultMessage: '{count} model(s) are greyed out; each shows why it cannot be used.',
    translations: { ja: '{count} 件のモデルは使えないため灰色で表示し、理由を添えています。' },
  },
  'aiGateway.picker.placeholder': { defaultMessage: 'Choose a model', translations: { ja: 'モデルを選択' } },
  'aiGateway.picker.empty': {
    defaultMessage: 'No models listed; the call uses the default route',
    translations: { ja: 'モデルがありません。既定の送り先で呼び出します' },
  },
  'aiGateway.picker.listFailed': {
    defaultMessage: 'The model list could not be loaded. Calls without a model still go to the default route.',
    translations: { ja: 'モデル一覧を読み込めませんでした。モデルを指定しない呼び出しは、既定の送り先に送ります。' },
  },
  'aiGateway.picker.blocked.policy': { defaultMessage: 'Not allowed by the effective policy', translations: { ja: '適用されたポリシーで許可されていません' } },
  'aiGateway.picker.blocked.policyRule': {
    defaultMessage: 'Not allowed: excluded by the {provider} rule {patterns}',
    translations: { ja: '使えません: {provider} のルール {patterns} で除外' },
  },
  'aiGateway.picker.blocked.provider': { defaultMessage: 'Provider {provider}: {state}', translations: { ja: 'プロバイダー {provider}: {state}' } },

  'aiGateway.error.fix.policy': { defaultMessage: 'Check the AI policy', translations: { ja: 'AI ポリシーを確認する' } },
  'aiGateway.error.fix.provider': { defaultMessage: 'Check the provider', translations: { ja: 'プロバイダーを確認する' } },
  'aiGateway.error.modelDenied.title': { defaultMessage: 'The model is not in the effective policy', translations: { ja: 'このモデルは適用されたポリシーにありません' } },
  'aiGateway.error.modelDenied.body': {
    defaultMessage: '{model} on {provider} is outside the AI policy (rule: {rule}). Pick an allowed model, or ask an admin to allow it.',
    translations: { ja: '{provider} の {model} は AI ポリシーの範囲外です(ルール: {rule})。許可されたモデルを選ぶか、管理者に許可を依頼してください。' },
  },
  'aiGateway.error.modelDenied.noRule': { defaultMessage: 'not listed', translations: { ja: '記載なし' } },
  'aiGateway.error.policyPending.title': { defaultMessage: 'The policy is not applied yet', translations: { ja: 'ポリシーがまだ反映されていません' } },
  'aiGateway.error.policyPending.body': {
    defaultMessage: 'A policy is bound to the Project but has not reached the gateway. Ask an admin to check the policy binding.',
    translations: { ja: 'Project にポリシーは割り当て済みですが、AI Gateway に反映されていません。管理者にポリシーの割り当てを確認してもらってください。' },
  },
  'aiGateway.error.loggingUnavailable.title': { defaultMessage: 'Full conversation logging is unavailable', translations: { ja: '会話の全文記録ができません' } },
  'aiGateway.error.loggingUnavailable.body': {
    defaultMessage: 'The policy requires full conversation logging, but no conversation store is configured, so the gateway refuses rather than send unrecorded.',
    translations: { ja: 'ポリシーが会話の全文記録を求めていますが、保存先が設定されていません。記録できないまま送ることはせず、リクエストを拒否しています。' },
  },
  'aiGateway.error.providerError.title': { defaultMessage: 'The provider failed', translations: { ja: 'プロバイダーでエラーが起きました' } },
  'aiGateway.error.providerError.body': {
    defaultMessage: '{provider} did not answer successfully. Check its API key or app on the Providers page.',
    translations: { ja: '{provider} から正常な応答がありませんでした。プロバイダー画面で API キーやアプリを確認してください。' },
  },
  'aiGateway.error.identityUnavailable.title': { defaultMessage: 'Sign-in is not ready yet', translations: { ja: 'サインインの準備ができていません' } },
  'aiGateway.error.identityUnavailable.body': {
    defaultMessage: 'The gateway cannot verify callers until the identity provider is reachable. Try again shortly.',
    translations: { ja: 'ID プロバイダーにつながるまで、AI Gateway は呼び出し元を確認できません。少し待ってからお試しください。' },
  },
  'aiGateway.error.unauthorized.title': { defaultMessage: 'Your session was not accepted', translations: { ja: 'セッションが受け付けられませんでした' } },
  'aiGateway.error.unauthorized.body': {
    defaultMessage: 'Sign in again. If it keeps happening, your account may not be a member of this Project.',
    translations: { ja: 'サインインし直してください。繰り返す場合は、この Project のメンバーでない可能性があります。' },
  },
  'aiGateway.error.unknown.title': { defaultMessage: 'The call failed ({code})', translations: { ja: '呼び出しに失敗しました({code})' } },
  'aiGateway.error.unknown.body': {
    defaultMessage: 'The gateway answered HTTP {status} with an error this page does not explain.',
    translations: { ja: 'AI Gateway が HTTP {status} を返しました。この画面では説明していないエラーです。' },
  },
  'aiGateway.error.unanswered.title': { defaultMessage: 'No answer from the gateway', translations: { ja: 'AI Gateway から応答がありません' } },
  'aiGateway.error.unanswered.body': {
    defaultMessage: 'The network or a proxy in front of the gateway failed. Try again in a moment.',
    translations: { ja: 'ネットワークか、AI Gateway の手前のプロキシで問題が起きました。少し待ってからお試しください。' },
  },
})
