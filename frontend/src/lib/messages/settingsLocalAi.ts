// 設定画面の AI の「ローカルAI（Ollama）」の文言（settings.ai.local として settingsAi.ts から参照する。#458。Windows のみ）。
// 画面は「AI のつなぎ先」の一覧の行と、そこから開くページ（つなぎ方のラジオ・managed・external）。
export const settingsLocalAi = {
  name: 'ローカルAI（Ollama）',
  rowNote: 'この PC で動かす',
  pageIntro:
    'メールの内容を外へ送らず、この PC の中の AI（Ollama）で下書き・要約・質問を作ります。使うモデルは「機能ごとに使う AI」で選びます。',
  modeLabel: 'つなぎ方',
  modeManaged: 'このアプリが用意する Ollama（おすすめ）',
  modeExternal: 'すでに動いているサーバーにつなぐ（Ollama・LM Studio など）',
  noKeyNote: 'API キーは要りません。メールの内容はこの PC の外へ送られません。',
  notInstalledNote:
    'ローカルAI（Ollama）をまだ導入していません。「AI のつなぎ先」で導入してください。',
  waitNote: 'ローカルAIは数分かかることがあります。',
  // 使えない PC。理由は Go が日本語 1 文で返す（unsupportedReason）。
  unsupportedHeading: 'この PC ではローカルAIを使えません',
  unsupportedCloud:
    '代わりに、クラウドの AI（ChatGPT プラン・Anthropic・OpenAI 互換）をつないで使えます。',
  unsupportedBack: '「AI のつなぎ先」の一覧へ戻る',
  statusLoading: 'ローカルAIの状態を確認しています…',
  statusFailed: 'ローカルAIの状態を取得できませんでした。',
  reload: '再読み込み',
  // 未導入・導入中
  notInstalledHeading: 'Ollama をまだ導入していません',
  notInstalledDetail: (version: string, size: string) =>
    `Ollama ${version} をダウンロードして、このアプリの中に導入します（ダウンロード約 ${size}）。導入したあと、使うモデルを取得します。`,
  install: 'Ollama を導入',
  cancel: '中止',
  stages: {
    download: 'ダウンロード中',
    verify: '照合中',
    extract: '展開中',
    start: '起動を確認中',
    pull: 'モデルを取得中',
  } as Record<string, string>,
  progressLabel: (stage: string) => `${stage}の進み`,
  progressPercent: (pct: number) => `${pct}%`,
  progressBytes: (done: string, total: string) => `${done} / ${total}`,
  // 導入済み
  installedHeading: 'Ollama',
  version: (v: string) => `版 ${v}`,
  running: '起動中',
  stopped: '停止中',
  stop: '停止',
  stopHint: '使うときだけ自動で起動し、しばらく使わないと止まります。',
  modelsHeading: '使えるモデル',
  modelsHint: 'モデルは取得すると、この PC の中に保存されます。',
  modelListLabel: 'ローカルAIのモデルの一覧',
  modelInstalled: '取得済み',
  modelSize: (size: string) => `約 ${size}`,
  modelLicense: (license: string) => `ライセンス: ${license}`,
  pull: '取得',
  pullFor: (name: string) => `${name} を取得`,
  cancelFor: (name: string) => `${name} の取得を中止`,
  delete: '削除',
  deleteFor: (name: string) => `${name} を削除`,
  deleteModelTitle: (name: string) => `${name} を削除しますか？`,
  deleteModelDetail:
    '取得済みのモデルをこの PC から削除します。使うときは、もう一度取得してください。',
  deleteModelConfirm: '削除する',
  remove: 'ローカルAIを削除',
  removeTitle: 'ローカルAI（Ollama）を削除しますか？',
  removeDetail:
    'Ollama と、取得したモデルをすべてこの PC から削除します。使うときは、導入からやり直してください。',
  removeConfirm: '削除する',
  failedHeading: '直前の操作に失敗しました',
  // external
  externalIntro:
    'すでに動いている Ollama や LM Studio などのサーバーにつなぎます。Base URL を保存すると使えるようになります。',
  baseUrl: 'Base URL',
  baseUrlPlaceholder: 'http://localhost:11434/v1',
  baseUrlHint:
    'Ollama は http://localhost:11434/v1、LM Studio は http://localhost:1234/v1 です。https の別ホスト、または localhost・127.0.0.1・::1 の http だけ使えます。',
  baseUrlSave: '保存',
  baseUrlSaved: 'Base URL を保存しました。',
  baseUrlRequired: 'Base URL を入力してください。',
  test: '接続テスト',
  testOk: (n: number) => `接続できました（モデル ${n} 件）`,
  testNone: '接続できましたが、モデルがありません。',
  testFailed: '接続テストに失敗しました。',
  externalSaveNote: 'このつなぎ方は、Base URL を「保存」すると有効になります。',
  externalLoopbackNote:
    '送信先がこの PC の中（localhost など）なので、メールの内容はこの PC の外へ送られません。',
  externalUnsetNote: 'Base URL を保存するまで、ローカルAIとしては使えません。',
  externalRemoteNote:
    '送信先がこの PC の外のため、メールの内容がそのサーバーへ送られます。下で送信への同意が必要です。',
  apiKeyHeading: 'API キー（任意）',
  apiKeyOptional:
    'API キーは任意です。認証が要るサーバー（外のホストなど）につなぐときだけ登録します。',
  destinationUnset: 'まだ設定していません',
} as const
