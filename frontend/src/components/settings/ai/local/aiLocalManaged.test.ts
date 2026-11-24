import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
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
} from './localAiKit'

installLocalAiMocks()
afterAll(restoreLocalAiMocks)

const { useSettingsStore } = await import('../../../../stores/settings')
const { useUiStore } = await import('../../../../stores/ui')
const { default: AILocalPage } = await import('./AILocalPage.vue')

// 開いたページは、テストごとに必ず閉じる（`localai:status` の購読は本物の Wails ランタイムにあり、閉じないと次のテストへ残る）。
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
const dialogButton = (text: string) =>
  [...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
    (b) => b.textContent?.trim() === text,
  )

async function open(s = status(), cfg = aiConfig()) {
  ctl.responses.GetLocalAIStatus = s
  useSettingsStore().aiConfig = cfg
  const w = mount(AILocalPage, { attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('ローカルAI（managed）の状態ごとの表示（#458）', () => {
  test('使えない PC: 理由とクラウドの案内を出し、導入のボタンは出さない。「つなぎ先の一覧へ戻る」で一覧へ', async () => {
    const w = await open(
      status({ supported: false, unsupportedReason: 'ARM64 の Windows には対応していません。' }),
    )
    expect(tid(w, 'local-ai-unsupported-reason').text()).toBe(
      'ARM64 の Windows には対応していません。',
    )
    expect(tid(w, 'local-ai-unsupported').text()).toContain(
      'ChatGPT プラン・Anthropic・OpenAI 互換',
    )
    expect(tid(w, 'local-ai-install').exists()).toBe(false)
    const ui = useUiStore()
    ui.aiProviderPage = 'local'
    await tid(w, 'local-ai-back-to-list').trigger('click')
    expect(ui.aiProviderPage).toBeUndefined()
  })

  test('未導入: 版・ダウンロード量・「Ollama を導入」。押すと InstallLocalAI を呼ぶ', async () => {
    const w = await open()
    const box = tid(w, 'local-ai-not-installed')
    expect(box.text()).toContain('v0.40.17')
    expect(box.text()).toContain('約 1.6 GB')
    expect(w.get('[data-testid="local-ai-note"]').text()).toBe(
      'API キーは要りません。メールの内容はこの PC の外へ送られません。',
    )
    ctl.responses.InstallLocalAI = status({
      job: { kind: 'install', stage: 'download', completedBytes: 0, totalBytes: 1618522644 },
    })
    await tid(w, 'local-ai-install').trigger('click')
    await flushPromises()
    expect(callsOf('InstallLocalAI')).toHaveLength(1)
    expect(tid(w, 'local-ai-installing').exists()).toBe(true)
  })

  test('導入中: 段階・進捗バー（%・GB）・「中止」。中止は CancelLocalAIJob', async () => {
    const w = await open(
      status({
        job: {
          kind: 'install',
          stage: 'download',
          completedBytes: 809261322,
          totalBytes: 1618522644,
        },
      }),
    )
    const bar = w.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuenow')).toBe('50')
    const text = tid(w, 'local-ai-progress').text()
    expect(text).toContain('ダウンロード中')
    expect(text).toContain('50%')
    expect(text).toContain('0.8 / 1.6 GB')
    ctl.responses.CancelLocalAIJob = status()
    await tid(w, 'local-ai-cancel').trigger('click')
    await flushPromises()
    expect(callsOf('CancelLocalAIJob')).toHaveLength(1)
    expect(tid(w, 'local-ai-not-installed').exists()).toBe(true)
  })

  test('導入済み: 起動中／停止中・「停止」（起動中だけ）・モデルの一覧（取得済みバッジ・取得・削除）', async () => {
    const w = await open(installedStatus(['gemma4:e4b'], { running: true }))
    expect(tid(w, 'local-ai-running').text()).toBe('起動中')
    const rows = w.findAll('[data-testid^="local-ai-model-gemma"]')
    expect(rows).toHaveLength(3)
    const e4b = tid(w, 'local-ai-model-gemma4:e4b')
    expect(e4b.text()).toContain('Gemma 4 E4B')
    expect(e4b.text()).toContain('標準（おすすめ）')
    expect(e4b.text()).toContain('約 6.6 GB')
    expect(e4b.text()).toContain('Apache-2.0')
    expect(e4b.text()).toContain('取得済み')
    expect(e4b.find('[data-testid="local-ai-model-delete"]').exists()).toBe(true)
    const e2b = tid(w, 'local-ai-model-gemma4:e2b')
    expect(e2b.text()).not.toContain('取得済み')
    expect(e2b.find('[data-testid="local-ai-model-pull"]').exists()).toBe(true)

    ctl.responses.StopLocalAI = installedStatus(['gemma4:e4b'])
    await tid(w, 'local-ai-stop').trigger('click')
    await flushPromises()
    expect(callsOf('StopLocalAI')).toHaveLength(1)
    expect(tid(w, 'local-ai-running').text()).toBe('停止中')
    expect(tid(w, 'local-ai-stop').exists()).toBe(false)
  })

  test('「取得」は PullLocalAIModel(name)。取得中は進捗＋「中止」になり、ほかのモデルの「取得」は押せない', async () => {
    const w = await open(installedStatus())
    ctl.responses.PullLocalAIModel = installedStatus([], {
      job: {
        kind: 'pull',
        model: 'gemma4:e2b',
        stage: 'pull',
        completedBytes: 1000000000,
        totalBytes: 4587156505,
      },
    })
    await tid(w, 'local-ai-model-gemma4:e2b')
      .get('[data-testid="local-ai-model-pull"]')
      .trigger('click')
    await flushPromises()
    expect(callsOf('PullLocalAIModel').map((c) => c.args)).toEqual([['gemma4:e2b']])
    const pulling = tid(w, 'local-ai-model-gemma4:e2b')
    expect(pulling.find('[role="progressbar"]').attributes('aria-valuenow')).toBe('21')
    expect(pulling.text()).toContain('モデルを取得中')
    expect(pulling.text()).toContain('1.0 / 4.6 GB')
    expect(
      tid(w, 'local-ai-model-gemma4:e4b')
        .get('[data-testid="local-ai-model-pull"]')
        .attributes('disabled'),
    ).toBeDefined()
    ctl.responses.CancelLocalAIJob = installedStatus()
    await pulling.get('[data-testid="local-ai-model-cancel"]').trigger('click')
    await flushPromises()
    expect(callsOf('CancelLocalAIJob')).toHaveLength(1)
    expect(
      tid(w, 'local-ai-model-gemma4:e2b').find('[data-testid="local-ai-model-pull"]').exists(),
    ).toBe(true)
  })

  test('モデルの「削除」は確認のあと DeleteLocalAIModel。「ローカルAIを削除」も確認のあと RemoveLocalAI', async () => {
    const w = await open(installedStatus(['gemma4:e4b']))
    await tid(w, 'local-ai-model-gemma4:e4b')
      .get('[data-testid="local-ai-model-delete"]')
      .trigger('click')
    await flushPromises()
    expect(callsOf('DeleteLocalAIModel')).toHaveLength(0)
    ctl.responses.DeleteLocalAIModel = installedStatus()
    dialogButton('削除する')?.click()
    await flushPromises()
    expect(callsOf('DeleteLocalAIModel').map((c) => c.args)).toEqual([['gemma4:e4b']])

    await tid(w, 'local-ai-remove').trigger('click')
    await flushPromises()
    expect(callsOf('RemoveLocalAI')).toHaveLength(0)
    ctl.responses.RemoveLocalAI = status()
    dialogButton('削除する')?.click()
    await flushPromises()
    expect(callsOf('RemoveLocalAI')).toHaveLength(1)
    expect(tid(w, 'local-ai-not-installed').exists()).toBe(true)
  })

  test('空き容量が足りない（no_space）: Go のメッセージを role="alert" で出す。lastError も出す', async () => {
    const w = await open(installedStatus([], { lastError: 'ダウンロードが途中で止まりました。' }))
    expect(tid(w, 'local-ai-alert').text()).toContain('ダウンロードが途中で止まりました。')
    const msg = '空き容量が足りません（必要: 約 6.6 GB・空き: 約 2.0 GB）。'
    ctl.failures.PullLocalAIModel = {
      code: 'VALIDATION',
      message: msg,
      retryable: false,
      details: { reason: 'no_space', requiredBytes: 6583656505, freeBytes: 2000000000 },
    }
    await tid(w, 'local-ai-model-gemma4:e4b')
      .get('[data-testid="local-ai-model-pull"]')
      .trigger('click')
    await flushPromises()
    const alert = w.get('[role="alert"]')
    expect(alert.text()).toContain(msg)
  })

  test('状態は開いたとき取得し、localai:status のイベントで更新する。閉じたら購読を解除する', async () => {
    const w = await open()
    expect(callsOf('GetLocalAIStatus')).toHaveLength(1)
    expect(subscribed('localai:status')).toBe(true)
    expect(tid(w, 'local-ai-not-installed').exists()).toBe(true)

    emitStatus(
      status({ job: { kind: 'install', stage: 'extract', completedBytes: 10, totalBytes: 20 } }),
    )
    await flushPromises()
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')
    expect(tid(w, 'local-ai-progress').text()).toContain('展開中')

    emitStatus(installedStatus(['gemma4:e2b'], { models: null }))
    await flushPromises()
    expect(tid(w, 'local-ai-installed').exists()).toBe(true)

    mounted.pop()
    w.unmount()
    expect(unsubscribed).toEqual(['localai:status'])
    expect(subscribed('localai:status')).toBe(false)
  })

  test('導入・取得で接続済み（ready）が変わったら、設定（aiConfig）を取り直す', async () => {
    await open(installedStatus())
    expect(callsOf('GetAIConfig')).toHaveLength(0)
    ctl.responses.GetAIConfig = aiConfig({ local: { mode: 'managed', baseUrl: '', ready: true } })
    emitStatus(installedStatus(['gemma4:e4b']))
    await flushPromises()
    expect(callsOf('GetAIConfig')).toHaveLength(1)
    expect(useSettingsStore().aiConfig?.local.ready).toBe(true)
  })
})
