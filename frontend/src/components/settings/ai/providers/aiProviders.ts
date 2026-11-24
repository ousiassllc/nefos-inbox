// AI のプロバイダの一覧（#315）。表示順どおり。`kind` は設定ページの種類:
//   draft = 文章を書くプロバイダ（同意・API キーまたはログイン・OpenAI 互換は Base URL。AISettings）、
//   key   = API キーだけを登録するプロバイダ（文章を書かないモデル。AITypeSafePage）、
//   local = ローカルAI（Ollama。Windows のみ・#458。AILocalPage。API キーは要らない。一覧には platform.supportsLocalAi のときだけ出す）。
// どちらも「つなぐ」ためのページで、モデルは選ばない（モデルは「機能ごとに使う AI」で選ぶ。#376）。
import type { AIConfig } from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { hasProviderCredential } from '../../../../lib/aiConfig'

export type AIProviderId = 'chatgpt' | 'anthropic' | 'openai' | 'local' | 'typesafe'

export interface AIProviderEntry {
  id: AIProviderId
  kind: 'draft' | 'key' | 'local'
}

export const AI_PROVIDERS: readonly AIProviderEntry[] = [
  { id: 'chatgpt', kind: 'draft' },
  { id: 'anthropic', kind: 'draft' },
  { id: 'openai', kind: 'draft' },
  { id: 'local', kind: 'local' },
  { id: 'typesafe', kind: 'key' },
]

export function providerKind(id: string): AIProviderEntry['kind'] | undefined {
  return AI_PROVIDERS.find((p) => p.id === id)?.kind
}

/** つなぎ先の状態。connected = 接続済み、disconnected = 未接続、reconnect = つなぎ直しが必要。 */
export type ConnectionState = 'connected' | 'disconnected' | 'reconnect'

/**
 * 資格情報（ログイン・API キー）があり、送信にも同意している＝そのプロバイダの AI を使える。
 * ローカルAI（local）は API キーを要らないので、`AIConfig.local.ready` だけで決める。送信への同意は、loopback 以外の送信先のときだけ
 * 使うときに求める（同意の有無はつなぎ先の行の補足にだけ出す）。
 */
export function isConnected(id: string, cfg: AIConfig | undefined): boolean {
  if (id === 'local') return hasProviderCredential(id, cfg)
  return hasProviderCredential(id, cfg) && (cfg?.consentedProviders ?? []).includes(id)
}

/** 資格情報はあるが送信への同意がまだ（つなぐ操作の途中）。 */
export function needsConsent(id: string, cfg: AIConfig | undefined): boolean {
  return hasProviderCredential(id, cfg) && !(cfg?.consentedProviders ?? []).includes(id)
}

/**
 * つなぎ先の行の状態。つながっていないのに、機能で選んでいる（外された）・ChatGPT に保存済みのアカウントがある（ログアウトした）ときは
 * 「つなぎ直しが必要」。選んでいない・初めてのときは「未接続」。
 */
export function connectionState(id: string, cfg: AIConfig | undefined): ConnectionState {
  if (isConnected(id, cfg)) return 'connected'
  const selected = [cfg?.features.draft, cfg?.features.threadAi, cfg?.features.inboxZero].some(
    (f) => f?.provider === id,
  )
  const hadAccount = id === 'chatgpt' && (cfg?.chatgptAccounts ?? []).length > 0
  return selected || hadAccount ? 'reconnect' : 'disconnected'
}
