// AI 送信先の表示用ロジック（NF §2: OpenAI 互換の Base URL を含め、送信先は常に表示する）。
import { isLoopbackInput } from '../../../lib/localAi/baseUrl'
import { settings as msg } from '../../../lib/messages'

const ANTHROPIC_URL = 'https://api.anthropic.com'
const OPENAI_DEFAULT_URL = 'https://api.openai.com'
const CHATGPT_URL = 'https://api.openai.com'
const TYPESAFE_URL = 'https://api.typesafe.ai'

/** AI の設定をまだ保存していないときのプロバイダ（利用者自身の ChatGPT プラン。#104）。Go の `defaultProvider`（aicore）と同じ値。 */
export const DEFAULT_AI_PROVIDER = 'chatgpt'

export interface AIDestination {
  providerLabel: string
  url: string
  /** Base URL をユーザーが指定している（既定ではない） */
  custom: boolean
  /** 送信先の補足（既定・Base URL の注記の代わりに出す）。 */
  note?: string
}

export function providerLabel(provider: string): string {
  return msg.ai.providerOptions[provider] ?? provider
}

/** 保存済み（または入力中）の Base URL から送信先を決める。Base URL は openai・local（external）のみ。loopback は保存済みの local の Go の値（入力中は渡さず文字列で判定）。 */
export function aiDestination(
  provider: string,
  baseUrl?: string,
  loopback?: boolean,
): AIDestination {
  const label = providerLabel(provider)
  if (provider === 'chatgpt') {
    return {
      providerLabel: label,
      url: CHATGPT_URL,
      custom: false,
      note: msg.ai.destinationChatGPTNote,
    }
  }
  if (provider === 'openai') {
    const custom = (baseUrl ?? '').trim()
    return custom
      ? { providerLabel: label, url: custom, custom: true }
      : { providerLabel: label, url: OPENAI_DEFAULT_URL, custom: false }
  }
  if (provider === 'local') {
    // ローカルAI（external）の送信先は保存した Base URL だけ（既定は無い）。managed は PC の外へ送らない。
    const url = (baseUrl ?? '').trim()
    const t = msg.ai.local
    const note =
      url === ''
        ? t.externalUnsetNote
        : (loopback ?? isLoopbackInput(url))
          ? t.externalLoopbackNote
          : t.externalRemoteNote
    return { providerLabel: label, url: url || t.destinationUnset, custom: url !== '', note }
  }
  if (provider === 'typesafe') return { providerLabel: label, url: TYPESAFE_URL, custom: false }
  return { providerLabel: label, url: ANTHROPIC_URL, custom: false }
}
