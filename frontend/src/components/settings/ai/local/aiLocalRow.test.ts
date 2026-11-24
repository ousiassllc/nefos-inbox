import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { createPinia } from 'pinia'
import { usePlatformStore } from '../../../../stores/platform'
import { useUiStore } from '../../../../stores/ui'
import { setup } from '../aiTestSupport'
import AIProviders from '../providers/AIProviders.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

// 「AI のつなぎ先」の一覧のローカルAI（Ollama）の行（#458）。capability（supportsLocalAi）のある OS（Windows）だけに出す。

function open(supportsLocalAi: boolean, cfg = {}) {
  const s = setup({ apiKeyProviders: [], consentedProviders: [], ...cfg })
  usePlatformStore().capabilities = {
    os: supportsLocalAi ? 'windows' : 'android',
    supportsMcp: true,
    supportsTray: true,
    supportsAutoUpdate: true,
    supportsWindowState: true,
    singleInstance: true,
    touchUi: !supportsLocalAi,
    supportsLocalAi,
  }
  useUiStore().aiTab = 'connect'
  const w = mount(AIProviders, {
    global: { plugins: [s.pinia as ReturnType<typeof createPinia>] },
    attachTo: document.body,
  })
  return { ...s, w }
}
const ids = (w: VueWrapper) =>
  w.findAll('[data-testid^="ai-provider-"][data-state]').map((r) => r.attributes('data-testid'))

describe('「AI のつなぎ先」のローカルAI（Ollama）の行（#458）', () => {
  test('Windows（supportsLocalAi）では行が出る。順序は OpenAI 互換のあと・TypeSafe AI の前', async () => {
    const { w } = open(true)
    await flushPromises()
    expect(ids(w)).toEqual([
      'ai-provider-chatgpt',
      'ai-provider-anthropic',
      'ai-provider-openai',
      'ai-provider-local',
      'ai-provider-typesafe',
    ])
    const row = w.get('[data-testid="ai-provider-local"]')
    expect(row.text()).toContain('ローカルAI（Ollama）')
    expect(row.text()).toContain('この PC で動かす')
    expect(row.find('[data-provider-icon="local"]').exists()).toBe(true)
  })

  test('Android・macOS（supportsLocalAi が偽）では行を出さない', async () => {
    const { w } = open(false)
    await flushPromises()
    expect(ids(w)).not.toContain('ai-provider-local')
    expect(w.text()).not.toContain('ローカルAI')
  })

  test('状態のバッジは local.ready で決める（API キーの有無・同意では決めない）', async () => {
    const off = open(true)
    expect(off.w.get('[data-testid="ai-provider-status-local"]').text()).toBe('未接続')
    off.w.unmount()

    const on = open(true, {
      local: { mode: 'managed', baseUrl: '', ready: true },
      consentedProviders: ['local'],
    })
    expect(on.w.get('[data-testid="ai-provider-status-local"]').text()).toBe('接続済み')
    // ログインも API キーも無いので、「接続を外す」は無い。常に「設定」
    expect(on.w.find('[data-testid="ai-provider-disconnect-local"]').exists()).toBe(false)
    expect(on.w.find('[data-testid="ai-provider-open-local"]').text()).toBe('設定')
  })

  test('機能で選んでいるのに ready でなければ「つなぎ直しが必要」。「設定」でページを開く（ui.aiProviderPage）', async () => {
    const { w, store } = open(true)
    store.aiConfig = {
      ...(store.aiConfig as NonNullable<typeof store.aiConfig>),
      features: {
        draft: { provider: 'local', model: 'gemma4:e4b' },
        threadAi: { provider: '', model: '' },
        inboxZero: { provider: '', model: '' },
      },
    }
    await flushPromises()
    expect(w.get('[data-testid="ai-provider-status-local"]').text()).toBe('つなぎ直しが必要')
    expect(w.find('[data-testid="ai-provider-connect-local"]').exists()).toBe(false)
    const ui = useUiStore()
    await w.get('[data-testid="ai-provider-open-local"]').trigger('click')
    expect(ui.aiProviderPage).toBe('local')
  })
})
