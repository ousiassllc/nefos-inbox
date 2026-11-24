import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  aiConfig,
  callsOf,
  ctl,
  installLocalAiMocks,
  resetLocalAiState,
  restoreLocalAiMocks,
} from './localAiKit'

installLocalAiMocks()
afterAll(restoreLocalAiMocks)

const { useSettingsStore } = await import('../../../../stores/settings')
const { default: AILocalPage } = await import('./AILocalPage.vue')

const mounted: VueWrapper[] = []
afterEach(() => {
  for (const w of mounted.splice(0)) w.unmount()
})
beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
  resetLocalAiState()
})

const tid = (w: Pick<VueWrapper, 'find'>, id: string) => w.find(`[data-testid="${id}"]`)
const OLLAMA = 'http://localhost:11434/v1'
/** Go の loopback と同じ（テスト用。localhost・127.0.0.1・[::1] の厳密一致）。 */
const isLoopback = (u: string) =>
  /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?(\/|$)/.test(u)
const EXTERNAL = (baseUrl = OLLAMA, ready = true) =>
  aiConfig({ local: { mode: 'external', baseUrl, ready, loopback: isLoopback(baseUrl) } })

async function open(cfg = aiConfig()) {
  useSettingsStore().aiConfig = cfg
  const w = mount(AILocalPage, { attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}
const urlInput = (w: VueWrapper) => w.get('input[type="url"]')

describe('ローカルAI（external）（#458）', () => {
  test('つなぎ方のラジオ: managed は保存する。external は入力欄を出すだけで、Base URL を保存するまで保存しない', async () => {
    const w = await open(EXTERNAL())
    expect(w.get('[role="radiogroup"]').findAll('input[type="radio"]')).toHaveLength(2)
    expect((tid(w, 'local-ai-mode-external').element as HTMLInputElement).checked).toBe(true)
    expect(tid(w, 'local-ai-external').exists()).toBe(true)

    ctl.responses.SetAILocalConnection = aiConfig()
    await tid(w, 'local-ai-mode-managed').trigger('change')
    await flushPromises()
    expect(callsOf('SetAILocalConnection').map((c) => c.args)).toEqual([['managed', '']])
    expect(tid(w, 'local-ai-managed').exists()).toBe(true)
    expect(useSettingsStore().aiConfig?.local.mode).toBe('managed')

    await tid(w, 'local-ai-mode-external').trigger('change')
    await flushPromises()
    expect(callsOf('SetAILocalConnection')).toHaveLength(1)
    expect(tid(w, 'local-ai-external').exists()).toBe(true)
    expect(urlInput(w).attributes('placeholder')).toBe(OLLAMA)
    expect(w.text()).toContain('http://localhost:1234/v1')
  })

  test('managed への切り替えに失敗したら、元のつなぎ方に戻してエラーを出す', async () => {
    const w = await open(EXTERNAL())
    ctl.failures.SetAILocalConnection = {
      code: 'UNSUPPORTED',
      message: 'この環境では使えません。',
      retryable: false,
    }
    await tid(w, 'local-ai-mode-managed').trigger('change')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toBe('この環境では使えません。')
    expect(tid(w, 'local-ai-external').exists()).toBe(true)
  })

  test('保存: SetAILocalConnection("external", baseUrl)。空は保存しない。不正は Go の VALIDATION を項目に出す', async () => {
    const w = await open()
    await tid(w, 'local-ai-mode-external').trigger('change')
    await flushPromises()
    await urlInput(w).setValue('  ')
    expect(tid(w, 'local-ai-base-url-save').attributes('disabled')).toBeDefined()

    await urlInput(w).setValue('http://example.com/v1')
    ctl.failures.SetAILocalConnection = {
      code: 'VALIDATION',
      message: 'http は localhost だけ使えます。',
      retryable: false,
    }
    await tid(w, 'local-ai-base-url-save').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('http は localhost だけ使えます。')

    delete ctl.failures.SetAILocalConnection
    await urlInput(w).setValue(`  ${OLLAMA}  `)
    ctl.responses.SetAILocalConnection = EXTERNAL()
    await tid(w, 'local-ai-base-url-save').trigger('click')
    await flushPromises()
    expect(callsOf('SetAILocalConnection').map((c) => c.args)).toEqual([
      ['external', 'http://example.com/v1'],
      ['external', OLLAMA],
    ])
    expect(useSettingsStore().aiConfig?.local).toEqual({
      mode: 'external',
      baseUrl: OLLAMA,
      ready: true,
      loopback: true,
    })
    expect(tid(w, 'local-ai-base-url-save').attributes('disabled')).toBeDefined()
  })

  test('接続テスト: 入力中の Base URL（未保存でも）で ListAIModels("local", …)。件数を出す', async () => {
    const w = await open()
    await tid(w, 'local-ai-mode-external').trigger('change')
    await urlInput(w).setValue(OLLAMA)
    ctl.responses.ListAIModels = {
      provider: 'local',
      models: [{ id: 'a' }, { id: 'b' }, { id: 'c' }],
    }
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    expect(callsOf('ListAIModels').map((c) => c.args)).toEqual([['local', OLLAMA]])
    const r = tid(w, 'local-ai-test-result')
    expect(r.text()).toBe('接続できました（モデル 3 件）')
    expect(r.attributes('role')).toBe('status')
    expect(callsOf('SetAILocalConnection')).toHaveLength(0)
  })

  test('接続テストの失敗: 返ってきた message を出す（role="alert"）。呼び出し自体の失敗も出す。空の Base URL は呼ばない', async () => {
    const w = await open()
    await tid(w, 'local-ai-mode-external').trigger('change')
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    expect(callsOf('ListAIModels')).toHaveLength(0)
    expect(tid(w, 'local-ai-test-result').text()).toBe('Base URL を入力してください。')

    await urlInput(w).setValue(OLLAMA)
    ctl.responses.ListAIModels = {
      provider: 'local',
      models: [],
      reason: 'failed',
      message: 'サーバーに接続できませんでした。',
    }
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    const r = tid(w, 'local-ai-test-result')
    expect(r.text()).toBe('サーバーに接続できませんでした。')
    expect(r.attributes('role')).toBe('alert')

    ctl.failures.ListAIModels = { code: 'INTERNAL', message: '内部エラーです。', retryable: false }
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    expect(tid(w, 'local-ai-test-result').text()).toBe('内部エラーです。')
  })

  test('API キー（任意）: SetAIApiKey("local", key)・ClearAIApiKey("local")', async () => {
    const w = await open(EXTERNAL())
    expect(w.text()).toContain('API キー（任意）')
    expect(w.text()).toContain('未設定')
    await w.get('input[type="password"]').setValue('secret-key')
    ctl.responses.SetAIApiKey = aiConfig({ ...EXTERNAL(), apiKeyProviders: ['local'] })
    await w
      .findAll('button')
      .find((b) => b.text() === 'キーを保存')
      ?.trigger('click')
    await flushPromises()
    expect(callsOf('SetAIApiKey').map((c) => c.args)).toEqual([['local', 'secret-key']])
    expect(w.text()).toContain('保存済み')

    ctl.responses.ClearAIApiKey = EXTERNAL()
    await w
      .findAll('button')
      .find((b) => b.text() === 'キーを削除')
      ?.trigger('click')
    await flushPromises()
    const confirm = [
      ...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button'),
    ].find((b) => b.textContent?.trim() === '削除する')
    confirm?.click()
    await flushPromises()
    expect(callsOf('ClearAIApiKey').map((c) => c.args)).toEqual([['local']])
  })

  test('同意の欄は loopback 以外の Base URL のときだけ。localhost・127.0.0.1・::1 は出さない', async () => {
    for (const url of [
      'http://localhost:11434/v1',
      'http://127.0.0.1:1234/v1',
      'http://[::1]:11434/v1',
    ]) {
      const w = await open(EXTERNAL(url))
      expect(w.text()).not.toContain('送信への同意')
      mounted.pop()
      w.unmount()
    }
    const w = await open(
      aiConfig({ ...EXTERNAL('https://llm.example.com/v1'), consentedProviders: [] }),
    )
    expect(w.text()).toContain('送信への同意')
    expect(w.text()).toContain('https://llm.example.com/v1')
    ctl.responses.RecordAIConsent = aiConfig({
      ...EXTERNAL('https://llm.example.com/v1'),
      consentedProviders: ['local'],
    })
    await tid(w, 'ai-consent-now').trigger('click')
    await flushPromises()
    const agree = [
      ...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button'),
    ].find((b) => b.textContent?.trim() === '同意する')
    agree?.click()
    await flushPromises()
    expect(callsOf('RecordAIConsent').map((c) => c.args)).toEqual([
      ['local', 'https://llm.example.com/v1'],
    ])
    expect(w.text()).toContain('同意済み')
  })
})
