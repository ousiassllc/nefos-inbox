// ローカルAI（#458）のテスト用ハーネス。LocalAIService・AIService のバインディングをモックする（`localai:status` は テスト用の購読の表へ流す）。
// ストア・コンポーネントを import する前に installLocalAiMocks() を呼び、あとは動的 import する（モックの張り前に静的 import すると本物を掴む）。
import { mock } from 'bun:test'
import * as realBindings from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app'
import type {
  AIConfig,
  LocalAIStatus,
} from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { features } from '../../../../../test/aiConfigKit'
import * as realEvents from '../../../../lib/events'

const EVENTS_MODULE = '../../../../lib/events'
const APP = '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app'

export type Call = { name: string; args: unknown[] }
export const calls: Call[] = []
export const callsOf = (name: string) => calls.filter((c) => c.name === name)

/** 応答（関数ならその戻り値。Promise 可）。failures に名前があれば、その値で reject する。 */
export const ctl: { responses: Record<string, unknown>; failures: Record<string, unknown> } = {
  responses: {},
  failures: {},
}

type Handler = (payload: unknown) => void
const handlers = new Map<string, Handler>()
/** 購読を解除した回数（ページを閉じたとき解除されるかの確認）。 */
export const unsubscribed: string[] = []

/** Go が `localai:status` を発行したことにする。 */
export function emitStatus(s: LocalAIStatus): void {
  handlers.get('localai:status')?.(s)
}
export const subscribed = (name: string) => handlers.has(name)

function binding(name: string) {
  return async (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in ctl.failures) throw ctl.failures[name]
    const r = ctl.responses[name]
    return typeof r === 'function' ? await (r as (...a: unknown[]) => unknown)(...args) : r
  }
}

const LOCAL_AI_METHODS = [
  'GetLocalAIStatus',
  'InstallLocalAI',
  'PullLocalAIModel',
  'CancelLocalAIJob',
  'StopLocalAI',
  'DeleteLocalAIModel',
  'RemoveLocalAI',
]
const AI_METHODS = [
  'SetAILocalConnection',
  'ListAIModels',
  'SetAIApiKey',
  'ClearAIApiKey',
  'RecordAIConsent',
  'GetAIConfig',
]

export function installLocalAiMocks(): void {
  mock.module(EVENTS_MODULE, () => ({
    ...realEvents,
    onEvent: (name: string, h: Handler) => {
      handlers.set(name, h)
      return () => {
        handlers.delete(name)
        unsubscribed.push(name)
      }
    },
  }))
  mock.module(APP, () => ({
    ...realBindings,
    LocalAIService: Object.fromEntries(LOCAL_AI_METHODS.map((n) => [n, binding(n)])),
    AIService: {
      ...realBindings.AIService,
      ...Object.fromEntries(AI_METHODS.map((n) => [n, binding(n)])),
    },
  }))
}

export function restoreLocalAiMocks(): void {
  mock.module(EVENTS_MODULE, () => ({ ...realEvents }))
  mock.module(APP, () => ({ ...realBindings }))
}

export const CATALOG = [
  {
    name: 'gemma4:e2b',
    displayName: 'Gemma 4 E2B',
    description: '軽い（メモリ 8 GB の PC 向け）',
    license: 'Apache-2.0',
    sizeBytes: 4587156505,
    installed: false,
  },
  {
    name: 'gemma4:e4b',
    displayName: 'Gemma 4 E4B',
    description: '標準（おすすめ）',
    license: 'Apache-2.0',
    sizeBytes: 6583656505,
    installed: false,
  },
  {
    name: 'gemma4:12b',
    displayName: 'Gemma 4 12B',
    description: '高品質（メモリ 16 GB 以上・GPU 推奨）',
    license: 'Apache-2.0',
    sizeBytes: 8021618941,
    installed: false,
  },
]

/** 使える PC・未導入の状態（既定）。差分だけ渡す。 */
export function status(over: Partial<LocalAIStatus> = {}): LocalAIStatus {
  return {
    supported: true,
    installed: false,
    running: false,
    version: 'v0.40.17',
    downloadBytes: 1618522644,
    models: CATALOG.map((m) => ({ ...m })),
    ...over,
  }
}

/** 導入済みの状態。installedModels に名前を渡すと、その取得済みにする。 */
export function installedStatus(
  installedModels: string[] = [],
  over: Partial<LocalAIStatus> = {},
): LocalAIStatus {
  return status({
    installed: true,
    models: CATALOG.map((m) => ({ ...m, installed: installedModels.includes(m.name) })),
    ...over,
  })
}

export function aiConfig(over: Partial<AIConfig> = {}): AIConfig {
  return {
    features: features(),
    apiKeyProviders: [],
    consentedProviders: ['local'],
    chatgptSignedIn: false,
    chatgptAccounts: [],
    local: { mode: 'managed', baseUrl: '', ready: false },
    ...over,
  }
}

/** 各テストの前に呼ぶ。 */
export function resetLocalAiState(): void {
  calls.length = 0
  handlers.clear()
  unsubscribed.length = 0
  ctl.failures = {}
  ctl.responses = { GetLocalAIStatus: status(), GetAIConfig: aiConfig() }
}
