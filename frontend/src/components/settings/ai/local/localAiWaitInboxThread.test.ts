import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { features } from '../../../../../test/aiConfigKit'
import { useInboxZeroStore } from '../../../../stores/inboxZero'
import { useSettingsStore } from '../../../../stores/settings'
import { useThreadAiStore } from '../../../../stores/threadAi'
import InboxZeroDialog from '../../../inbox/InboxZeroDialog.vue'
import ThreadAiPanel from '../../../thread/ai/ThreadAiPanel.vue'
import { base } from '../aiTestSupport'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

// Inbox Zero の判断中・スレッドの要約と質問の生成中も、使う AI が local のときだけ待ち時間の注記を出す（#458）。
const NOTE = 'ローカルAIは数分かかることがあります。'
const dialogText = () => document.body.textContent ?? ''

function configure(provider: 'local' | 'anthropic') {
  useSettingsStore().aiConfig = {
    ...base,
    features: features(
      { provider, model: 'm' },
      { provider, model: 'm' },
      { provider, model: 'm' },
    ),
    apiKeyProviders: ['anthropic'],
    consentedProviders: ['anthropic', 'local'],
    local: { mode: 'managed', baseUrl: '', ready: true, loopback: true },
  }
}

describe('Inbox Zero の判断中', () => {
  async function check(provider: 'local' | 'anthropic', needsAI = true) {
    configure(provider)
    const zero = useInboxZeroStore()
    zero.reasons = {
      ad: needsAI,
      notification: false,
      newsletter: false,
      replied: true,
      calendar: false,
    }
    zero.phase = 'checking'
    const w = mount(InboxZeroDialog, { attachTo: document.body })
    await flushPromises()
    return w
  }

  test('local のときだけ注記を出す', async () => {
    await check('local')
    expect(dialogText()).toContain(NOTE)
  })

  test('ほかの AI・AI を使わない判断（返信済みだけ）では出さない', async () => {
    await check('anthropic')
    expect(dialogText()).not.toContain(NOTE)
    document.body.innerHTML = ''
    setActivePinia(createPinia())
    await check('local', false)
    expect(dialogText()).not.toContain(NOTE)
  })
})

describe('スレッドの AI パネル', () => {
  function panel(provider: 'local' | 'anthropic', status: 'summarizing' | 'asking') {
    configure(provider)
    const store = useThreadAiStore()
    store.status = status
    store.summary =
      status === 'summarizing' ? { text: '', includedCount: 0, totalCount: 0 } : undefined
    const flow = {
      needText: { value: '' },
      needsKey: { value: false },
      step: { value: 'idle' },
      error: { value: undefined },
    }
    return mount(ThreadAiPanel, { props: { flow: flow as never }, attachTo: document.body })
  }

  test('要約中・質問中は、local のときだけ注記を出す', () => {
    expect(panel('local', 'summarizing').text()).toContain(NOTE)
    expect(panel('local', 'asking').text()).toContain(NOTE)
  })

  test('ほかの AI では出さない', () => {
    expect(panel('anthropic', 'summarizing').text()).not.toContain(NOTE)
    expect(panel('anthropic', 'asking').text()).not.toContain(NOTE)
  })
})
