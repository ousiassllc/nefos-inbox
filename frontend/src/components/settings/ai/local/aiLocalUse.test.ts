import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { features } from '../../../../../test/aiConfigKit'
import { aiModelsKey, effortChoices, hasProviderCredential } from '../../../../lib/aiConfig'
import { useUiStore } from '../../../../stores/ui'
import BaseSelect from '../../../ui/BaseSelect.vue'
import { base, setup } from '../aiTestSupport'
import AIUseTab from '../features/AIUseTab.vue'
import { featureChoices, modelsKeyFor } from '../features/aiFeatures'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

// 「機能ごとに使う AI」のローカルAI（#458）。local.ready のときだけ選べ、モデルは ListAIModels('local', …)、エフォートは出さない。

const MANAGED = { mode: 'managed', baseUrl: '', ready: true, loopback: true }
const EXTERNAL = {
  mode: 'external',
  baseUrl: 'http://localhost:1234/v1',
  ready: true,
  loopback: true,
}
const LOCAL_MODELS = {
  provider: 'local',
  models: [{ id: 'gemma4:e4b', displayName: 'Gemma 4 E4B' }],
}

function open(cfg = {}, load?: (p: string, baseUrl: string) => unknown) {
  const s = setup(
    { apiKeyProviders: [], consentedProviders: ['local'], features: features(), ...cfg },
    undefined,
    (p, baseUrl) => (load?.(p, baseUrl) ?? { provider: p, models: [] }) as never,
  )
  const saved: unknown[][] = []
  s.store.setAIFeature = async (...a: unknown[]) => {
    saved.push(a)
  }
  const w = mount(AIUseTab, { global: { plugins: [s.pinia] }, attachTo: document.body })
  return { ...s, w, saved }
}
const feat = (w: VueWrapper, id: string) => w.get(`[data-testid="ai-feature-${id}"]`)
const pick = (w: VueWrapper, id: string) => feat(w, id).getComponent(BaseSelect)
const values = (sel: ReturnType<typeof pick>) =>
  (sel.props('options') as { value: string }[]).map((o) => o.value)

describe('ローカルAIの選択肢（#458）', () => {
  test('ready のときだけ local を選べる。ログイン・API キーは要らない。表示名は「ローカルAI（Ollama） ・ モデル」', () => {
    expect(hasProviderCredential('local', { ...base, local: MANAGED })).toBe(true)
    expect(hasProviderCredential('local', { ...base, local: { ...MANAGED, ready: false } })).toBe(
      false,
    )
    const none = { provider: '', model: '' }
    const ready = featureChoices(
      'draft',
      { ...base, apiKeyProviders: [], local: MANAGED },
      { local: LOCAL_MODELS },
      none,
    )
    expect(ready.options).toEqual([
      { value: 'local|gemma4:e4b', label: 'ローカルAI（Ollama） ・ Gemma 4 E4B（gemma4:e4b）' },
    ])
    const notReady = featureChoices(
      'draft',
      { ...base, apiKeyProviders: [], local: { ...MANAGED, ready: false } },
      {},
      none,
    )
    expect(notReady.options).toEqual([])
    expect(notReady.noneAvailable).toBe(true)
  })

  test('モデル一覧の保存キーは managed と external の Base URL で別になる。エフォートの段階は無い', () => {
    expect(modelsKeyFor('local', { ...base, local: MANAGED })).toBe('local')
    expect(modelsKeyFor('local', { ...base, local: EXTERNAL })).toBe(
      'local|http://localhost:1234/v1',
    )
    expect(aiModelsKey('local', '')).toBe('local')
    expect(aiModelsKey('local', 'http://localhost:1234/v1')).toBe('local|http://localhost:1234/v1')
    expect(effortChoices('local', 'gemma4:e4b', undefined)).toEqual([])
  })

  test('managed: ListAIModels("local", "")。3 つの機能すべてで選べる。選ぶと effort は空で保存する', async () => {
    const { w, loads, saved } = open({ local: MANAGED }, () => LOCAL_MODELS)
    await flushPromises()
    expect(loads).toEqual([{ provider: 'local', baseUrl: '' }])
    for (const f of ['draft', 'threadAi', 'inboxZero']) {
      expect(values(pick(w, f))).toEqual(['local|gemma4:e4b'])
    }
    pick(w, 'draft').vm.$emit('update:modelValue', 'local|gemma4:e4b')
    await flushPromises()
    expect(saved).toEqual([['draft', 'local', 'gemma4:e4b', '']])
  })

  test('external: ListAIModels("local", 保存済みの Base URL)', async () => {
    const { loads } = open({ local: EXTERNAL }, () => LOCAL_MODELS)
    await flushPromises()
    expect(loads).toEqual([{ provider: 'local', baseUrl: 'http://localhost:1234/v1' }])
  })

  test('選んでいる AI が local のときはエフォートの選択を出さない（draft・threadAi とも）', async () => {
    const { w } = open(
      {
        local: MANAGED,
        features: features({ provider: 'local', model: 'gemma4:e4b' }, '', {
          provider: 'local',
          model: 'gemma4:e4b',
        }),
      },
      () => LOCAL_MODELS,
    )
    await flushPromises()
    expect(w.find('[data-testid="ai-effort"]').exists()).toBe(false)
    expect(feat(w, 'draft').text()).toContain('ローカルAI（Ollama）')
  })

  test('選んでいるのに ready でない: 選択を残したまま「つなぎ直しが必要」と「つなぎ先を直す」。直すとローカルAIのページへ', async () => {
    const { w } = open({
      local: { ...MANAGED, ready: false },
      features: features({ provider: 'local', model: 'gemma4:e4b' }),
    })
    await flushPromises()
    expect(feat(w, 'draft').find('[data-testid="ai-feature-reconnect"]').exists()).toBe(true)
    await feat(w, 'draft').get('[data-testid="ai-feature-fix-draft"]').trigger('click')
    expect(useUiStore().aiProviderPage).toBe('local')
  })

  test('一覧が not_installed のときは、手入力ではなく「つなぎ先」での導入を案内する', async () => {
    const { w } = open({ local: MANAGED }, () => ({
      provider: 'local',
      models: [],
      reason: 'not_installed',
      message:
        'ローカルAI（Ollama）をまだ導入していません。設定画面の「AI のつなぎ先」で導入してください。',
    }))
    await flushPromises()
    const note = feat(w, 'draft').get('[data-testid="ai-feature-not-installed-draft"]')
    expect(note.text()).toContain('導入してください')
    expect(feat(w, 'draft').find('[data-testid^="ai-feature-manual-"]').exists()).toBe(false)
  })
})
