import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick, ref } from 'vue'
import {
  aiConfig,
  callsOf,
  ctl,
  emitStatus,
  installedStatus,
  installLocalAiMocks,
  resetLocalAiState,
  restoreLocalAiMocks,
  status,
  subscribed,
  unsubscribed,
} from '../localAiKit'

installLocalAiMocks()
afterAll(restoreLocalAiMocks)

const { useSettingsStore } = await import('../../../../../stores/settings')
const { useLocalAiStore } = await import('../../../../../stores/localAi')
const { keepLocalAiStatus } = await import('../../../../../stores/localAi/keepStatus')
const { usePlatformStore } = await import('../../../../../stores/platform')
const { default: AILocalPage } = await import('../AILocalPage.vue')
const { default: AIConnectTab } = await import('../../providers/AIConnectTab.vue')
const { default: AIUseTab } = await import('../../features/AIUseTab.vue')
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
const urlInput = (w: VueWrapper) => w.get('input[type="url"]')
const OLLAMA = 'http://localhost:11434/v1'
const external = (baseUrl = OLLAMA, ready = true) =>
  aiConfig({ local: { mode: 'external', baseUrl, ready, loopback: true } })

async function openPage(s = installedStatus(['gemma4:e4b']), cfg = aiConfig()) {
  ctl.responses.GetLocalAIStatus = s
  useSettingsStore().aiConfig = cfg
  const w = mount(AILocalPage, { attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}

/** 手で解決できる Promise（呼び出しを保留する）。 */
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('F-M1 ローカルAIの状態の購読と設定の取り直し', () => {
  test('設定ダイアログが開いている間は、ページを開いていなくても購読を保ち、導入済み・取得済みの数が変わったら設定を取り直す', async () => {
    const open = ref(true)
    usePlatformStore().capabilities.supportsLocalAi = true
    ctl.responses.GetLocalAIStatus = status()
    const holder = mount({ setup: () => keepLocalAiStatus(open, () => true), render: () => null })
    mounted.push(holder)
    await flushPromises()
    expect(subscribed('localai:status')).toBe(true)
    expect(callsOf('GetAIConfig')).toHaveLength(0)

    emitStatus(installedStatus()) // 導入が終わった（ジョブはページの外で進んだ）
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(1)

    emitStatus(installedStatus([], { running: true })) // 変化なし
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(1)

    emitStatus(installedStatus(['gemma4:e4b']))
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(2)

    open.value = false // ダイアログを閉じたら解除する
    await nextTick()
    expect(subscribed('localai:status')).toBe(false)
    expect(unsubscribed).toEqual(['localai:status'])
  })

  test('ダイアログとページの購読は重ねて持てる。ページを閉じてもダイアログが開いている間は保ち、最後が閉じたら解除する（二重に解除しない）', async () => {
    const store = useLocalAiStore()
    const offDialog = store.start()
    const offPage = store.start()
    expect(subscribed('localai:status')).toBe(true)
    offPage()
    offPage() // 何度呼んでもよい
    expect(subscribed('localai:status')).toBe(true)
    offDialog()
    expect(subscribed('localai:status')).toBe(false)
    expect(unsubscribed).toEqual(['localai:status'])
  })

  test('「AI のつなぎ先」「機能ごとに使う AI」のタブを開いたとき、設定を取り直す', async () => {
    usePlatformStore().capabilities.supportsLocalAi = true
    useSettingsStore().aiConfig = aiConfig()
    mounted.push(mount(AIConnectTab, { attachTo: document.body }))
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(1)
    mounted.push(mount(AIUseTab, { attachTo: document.body }))
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(2)
  })

  test('ローカルAIに対応しない OS では、取り直しも購読もしない', async () => {
    usePlatformStore().capabilities.supportsLocalAi = false
    useSettingsStore().aiConfig = aiConfig()
    mounted.push(mount(AIConnectTab, { attachTo: document.body }))
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(0)
  })
})

describe('F-M2 操作の結果は、新しいイベントを上書きしない', () => {
  test('操作の呼び出し中に新しい localai:status が届いたら、遅れて返った古い結果は捨てる', async () => {
    const store = useLocalAiStore()
    ctl.responses.GetLocalAIStatus = installedStatus()
    const off = store.start()
    await flushPromises()
    const slow = deferred<unknown>()
    ctl.responses.StopLocalAI = () => slow.promise
    const run = store.stop()
    emitStatus(installedStatus(['gemma4:e4b'], { running: true })) // 新しい状態
    slow.resolve(installedStatus([], { running: false })) // 古い結果
    await run
    expect(store.status?.running).toBe(true)
    expect(store.models.filter((m) => m.installed)).toHaveLength(1)
    off()
  })
})

describe('F-m2 エラーの消去と初回の読み込みの失敗', () => {
  test('start() と成功した refresh() でエラーを消す', async () => {
    const store = useLocalAiStore()
    ctl.failures.GetLocalAIStatus = {
      code: 'INTERNAL',
      message: '取得に失敗しました。',
      retryable: false,
    }
    const off = store.start()
    await flushPromises()
    expect(store.error).toBe('取得に失敗しました。')
    delete ctl.failures.GetLocalAIStatus
    await store.refresh()
    expect(store.error).toBeUndefined()
    off()

    ctl.failures.GetLocalAIStatus = {
      code: 'INTERNAL',
      message: '取得に失敗しました。',
      retryable: false,
    }
    await store.refresh()
    expect(store.error).toBe('取得に失敗しました。')
    delete ctl.failures.GetLocalAIStatus
    store.start()()
    expect(store.error).toBeUndefined()
  })

  test('最初の読み込みに失敗したら「確認しています…」のままにせず、「再読み込み」を出す。押すと取り直す', async () => {
    ctl.failures.GetLocalAIStatus = {
      code: 'INTERNAL',
      message: '取得に失敗しました。',
      retryable: false,
    }
    const w = await openPage(status())
    expect(w.text()).not.toContain('確認しています')
    expect(tid(w, 'local-ai-alert').text()).toContain('取得に失敗しました。')
    delete ctl.failures.GetLocalAIStatus
    ctl.responses.GetLocalAIStatus = status()
    await tid(w, 'local-ai-reload').trigger('click')
    await flushPromises()
    expect(tid(w, 'local-ai-not-installed').exists()).toBe(true)
    expect(tid(w, 'local-ai-alert').exists()).toBe(false)
  })
})

describe('F-m3 同じ文面の警告は 1 つだけ', () => {
  test('操作の失敗と lastError が同じ文面なら 1 回だけ出す', async () => {
    const msg = 'ダウンロードが進まなくなったため止めました。'
    const w = await openPage(installedStatus([], { lastError: msg }))
    ctl.failures.PullLocalAIModel = { code: 'AI_FAILED', message: msg, retryable: false }
    await tid(w, 'local-ai-model-gemma4:e2b')
      .get('[data-testid="local-ai-model-pull"]')
      .trigger('click')
    await flushPromises()
    expect(tid(w, 'local-ai-alert').text().split(msg).length - 1).toBe(1)
  })
})

describe('G3-UI ジョブ中は「停止」を押せない', () => {
  test('導入済みで起動中でも、取得のジョブ中は「停止」が無効', async () => {
    const w = await openPage(installedStatus([], { running: true }))
    expect(tid(w, 'local-ai-stop').attributes('disabled')).toBeUndefined()
    emitStatus(
      installedStatus([], {
        running: true,
        job: { kind: 'pull', model: 'gemma4:e2b', stage: 'pull', completedBytes: 1, totalBytes: 2 },
      }),
    )
    await flushPromises()
    expect(tid(w, 'local-ai-stop').attributes('disabled')).toBeDefined()
  })
})

describe('F-m4 接続テストの結果', () => {
  async function openExternal() {
    const w = await openPage(status(), aiConfig())
    await tid(w, 'local-ai-mode-external').trigger('change')
    await flushPromises()
    return w
  }

  test('入力を変えたら前の結果を消す', async () => {
    const w = await openExternal()
    await urlInput(w).setValue(OLLAMA)
    ctl.responses.ListAIModels = { provider: 'local', models: [{ id: 'm1', displayName: 'm1' }] }
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    expect(tid(w, 'local-ai-test-result').text()).toContain('モデル 1 件')
    await urlInput(w).setValue('http://localhost:1234/v1')
    expect(tid(w, 'local-ai-test-result').exists()).toBe(false)
  })

  test('テストの最中に入力が変わったら、その結果は捨てる', async () => {
    const w = await openExternal()
    await urlInput(w).setValue(OLLAMA)
    const slow = deferred<unknown>()
    ctl.responses.ListAIModels = () => slow.promise
    await tid(w, 'local-ai-test').trigger('click')
    await urlInput(w).setValue('http://localhost:1234/v1')
    slow.resolve({ provider: 'local', models: [{ id: 'm1', displayName: 'm1' }] })
    await flushPromises()
    expect(tid(w, 'local-ai-test-result').exists()).toBe(false)
  })

  test('0 件は、成功の緑ではなく注意の文面（role="status"）で出す', async () => {
    const w = await openExternal()
    await urlInput(w).setValue(OLLAMA)
    ctl.responses.ListAIModels = { provider: 'local', models: [] }
    await tid(w, 'local-ai-test').trigger('click')
    await flushPromises()
    const r = tid(w, 'local-ai-test-result')
    expect(r.text()).toBe('接続できましたが、モデルがありません。')
    expect(r.classes()).toContain('warn')
    expect(r.classes()).not.toContain('ok')
    expect(r.attributes('role')).toBe('status')
  })
})

describe('F-m5 external の入力値を、managed との切り替えで消さない', () => {
  test('managed に切り替えて戻っても入力中の値が残る。保存前は「保存すると有効」の注記を出す', async () => {
    const w = await openPage(status(), aiConfig())
    await tid(w, 'local-ai-mode-external').trigger('change')
    await flushPromises()
    expect(tid(w, 'local-ai-external-save-note').text()).toBe(
      'このつなぎ方は、Base URL を「保存」すると有効になります。',
    )
    await urlInput(w).setValue('http://localhost:1234/v1')
    await tid(w, 'local-ai-mode-managed').trigger('change')
    await flushPromises()
    await tid(w, 'local-ai-mode-external').trigger('change')
    await flushPromises()
    expect((urlInput(w).element as HTMLInputElement).value).toBe('http://localhost:1234/v1')
  })

  test('すでに external で保存済みなら、注記は出さない', async () => {
    const w = await openPage(status(), external())
    expect(tid(w, 'local-ai-external-save-note').exists()).toBe(false)
  })
})

describe('F-m1 保存済みの loopback は Go の値を使う', () => {
  test('保存済みの Base URL は local.loopback で同意の欄を決める（localhost の別表記は loopback ではない）', async () => {
    const remote = aiConfig({
      local: { mode: 'external', baseUrl: 'https://LOCALHOST:1/v1', ready: true, loopback: false },
      consentedProviders: [],
    })
    const w = await openPage(status(), remote)
    expect(w.text()).toContain('送信への同意')
    const w2 = await openPage(
      status(),
      aiConfig({
        local: { mode: 'external', baseUrl: 'http://localhost:1/v1', ready: true, loopback: true },
      }),
    )
    expect(w2.text()).not.toContain('送信への同意')
  })
})
