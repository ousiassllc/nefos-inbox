import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { features } from '../../../../../test/aiConfigKit'
import { useSettingsStore } from '../../../../stores/settings'
import AiDraftButton from '../../../compose/ai/AiDraftButton.vue'
import { base } from '../aiTestSupport'
import LocalAiWaitNote from './LocalAiWaitNote.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

// ローカルAIは生成に数分かかることがあるので、生成中のスピナーの近くに待ち時間の長い旨を出す（#458）。自動では生成しない。

const NOTE = 'ローカルAIは数分かかることがあります。'

function mountDraft(draftProvider: 'local' | 'anthropic', ready = true) {
  const settings = useSettingsStore()
  settings.aiConfig = {
    ...base,
    features: features({ provider: draftProvider, model: 'm' }),
    apiKeyProviders: draftProvider === 'anthropic' ? ['anthropic'] : [],
    consentedProviders: ['anthropic', 'local'],
    local: { mode: 'managed', baseUrl: '', ready },
  }
  let finish: () => void = () => undefined
  const generated: string[] = []
  settings.generateReplyDraft = () =>
    new Promise((resolve) => {
      generated.push('call')
      finish = () => resolve({ bodyHtml: '<p>本文</p>' })
    })
  const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
  return { w, generated, finish: () => finish() }
}

describe('ローカルAIの待ち表示（#458）', () => {
  test('LocalAiWaitNote は provider が local のときだけ（role="status"）', () => {
    expect(
      mount(LocalAiWaitNote, { props: { provider: 'local' } })
        .get('[role="status"]')
        .text(),
    ).toBe(NOTE)
    for (const p of ['anthropic', 'openai', 'chatgpt', 'typesafe', undefined]) {
      expect(mount(LocalAiWaitNote, { props: { provider: p } }).text()).toBe('')
    }
  })

  test('「AI で下書き」: 押す前は何も生成せず・注記も出さない。local で生成中だけ注記とキャンセル。同意の画面は出さない', async () => {
    const { w, generated, finish } = mountDraft('local')
    await flushPromises()
    expect(generated).toEqual([])
    expect(w.text()).not.toContain(NOTE)

    await w.get('button').trigger('click')
    await flushPromises()
    expect(generated).toEqual(['call'])
    expect(w.text()).toContain(NOTE)
    expect(w.text()).toContain('生成をキャンセル')
    expect(document.body.querySelector('[data-testid="ai-consent-body"]')).toBeNull()

    finish()
    await flushPromises()
    expect(w.text()).not.toContain(NOTE)
    expect(w.emitted('generated')?.[0]).toEqual(['<p>本文</p>'])
  })

  test('ほかの AI（anthropic）で生成中のときは注記を出さない', async () => {
    const { w, generated, finish } = mountDraft('anthropic')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(generated).toEqual(['call'])
    expect(w.text()).not.toContain(NOTE)
    finish()
    await flushPromises()
  })

  test('local が使える状態でない（未導入など）ときは、押せず、導入の案内を出す（APIキーの案内にしない）', async () => {
    const { w } = mountDraft('local', false)
    await flushPromises()
    expect(w.get('button').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('ローカルAI（Ollama）が使える状態ではありません')
    expect(w.text()).not.toContain('API キー')
  })
})
