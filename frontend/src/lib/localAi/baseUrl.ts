// ローカルAI（Ollama。#458）まわりの Base URL の扱い。
import type { AIConfig } from '../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'

/**
 * そのプロバイダの送信先の Base URL（保存済みの設定から）。Base URL を持つのは OpenAI 互換と、つなぎ方が external のローカルAIだけ。
 * managed のローカルAI・ほかのプロバイダは空（空は、そのプロバイダの既定の送信先。managed は Go が起動した Ollama）。
 */
export function providerBaseUrl(provider: string, cfg?: AIConfig): string {
  if (provider === 'openai') return cfg?.baseUrl ?? ''
  if (provider === 'local' && cfg?.local?.mode === 'external') return cfg.local.baseUrl
  return ''
}

const LOOPBACK_HOSTS = ['localhost', '127.0.0.1', '[::1]']

/**
 * 入力中（未保存）の Base URL の送信先が、この PC の中か。Go の `ai.IsLoopbackBaseURL` と同じ厳密一致で、URL の正規化をしない
 * （`LOCALHOST`・`127.1`・`2130706433`・`[0:0:0:0:0:0:0:1]` は loopback ではない）。生の authority（http(s):// と最初の `/` の間）の host を
 * `localhost`・`127.0.0.1`・`[::1]` と大文字小文字を区別して比べる。userinfo・query・fragment つきは Go が拒否するので false。
 * 保存済みの Base URL は Go が `AILocalConnection.loopback` で返す（そちらを使う）。
 */
export function isLoopbackInput(raw: string): boolean {
  const m = /^https?:\/\/([^/?#]*)(?:[/][^?#]*)?$/i.exec(raw.trim())
  const authority = m?.[1]
  if (authority === undefined || authority.includes('@')) return false
  const hp = /^(\[[^\]]*\]|[^:]*)(?::\d*)?$/.exec(authority)
  return hp !== null && LOOPBACK_HOSTS.includes(hp[1])
}
