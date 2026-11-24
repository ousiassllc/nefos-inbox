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

/** Base URL の送信先が、この PC の中（localhost・127.0.0.1・::1）か。Go の localconn.Conn.Loopback と同じ見方（空・解釈できない URL は false）。 */
export function isLoopbackBaseUrl(baseUrl: string): boolean {
  try {
    const host = new URL(baseUrl.trim()).hostname
    return host === 'localhost' || host === '127.0.0.1' || host === '[::1]' || host === '::1'
  } catch {
    return false
  }
}
