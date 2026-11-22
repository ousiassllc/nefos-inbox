// Ousiass デザインガイド準拠の回帰テスト（#222）。タイポグラフィ（T5）の Mono の指定を確かめる。

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
import { rule } from './styleRule'

describe('桁を揃える文字の Mono（T5・#222）', () => {
  test('スレッド行のメッセージ件数（数字だけ）は --font-mono', () => {
    expect(rule('inbox/ThreadRow.vue', '.count {')).toContain('font-family: var(--font-mono)')
  })

  test('--font-mono は日本語が混ざっても Noto Sans JP で描く（Mono の次に置く。#283）', () => {
    const css = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8')
    expect(css).toContain(
      '--font-mono: "JetBrains Mono Variable", "Noto Sans JP Variable", ui-monospace, monospace;',
    )
  })

  test('送信先の URL だけを別要素に分け、--font-mono にする', () => {
    const w = mount(AIDestinationBox, {
      props: {
        provider: 'openai',
        baseUrl: 'https://llm.example.com/v1',
        draftProvider: 'openai',
        draftBaseUrl: 'https://llm.example.com/v1',
      },
    })
    expect(w.get('[data-testid="ai-destination"] .url').text()).toBe('https://llm.example.com/v1')
    expect(rule('settings/ai/AIDestinationBox.vue', '.url {')).toContain(
      'font-family: var(--font-mono)',
    )
  })

  test('メニュー項目のショートカット表記は --font-mono', () => {
    expect(rule('ui/BaseMenu.vue', '.menu-shortcut {')).toContain('font-family: var(--font-mono)')
  })
})

describe('行間はスケールの値（T3・T4・#227）', () => {
  test('プレーンテキスト本文は 16px・1.8（sandbox iframe の逸脱とは別）', () => {
    const body = rule('thread/MessageBody.vue', '.body-text {')
    expect(body).toContain('font-size: var(--font-size-base)')
    expect(body).toContain('line-height: var(--line-height-base)')
  })

  test.each([['ui/CountBadge.vue', '.badge {']])(
    '件数バッジ %s %s は line-height に xs を持ち、min-height 20px を超えない',
    (file, selector) => {
      const badge = rule(file, selector)
      expect(badge).toContain('line-height: var(--line-height-xs)')
      expect(badge).toContain('min-height: 20px')
    },
  )

  test.each([
    ['ui/BaseTooltip.vue', ':global(.tip-pop) {', 'xs'],
    ['thread/ThreadHeader.vue', '.meta-row {', 'sm'],
    ['compose/AttachmentBar.vue', '.attach-hint {', 'xs'],
    ['compose/ComposePane.vue', '.foot-hint {', 'xs'],
    ['settings/ai/ChatGPTAccountRow.vue', '.pick {', 'sm'],
    ['compose/SenderField.vue', '.sender-email {', 'sm'],
  ])('折り返しうる %s %s は font-size と対の line-height を持つ', (file, selector, size) => {
    const r = rule(file, selector)
    expect(r).toContain(`font-size: var(--font-size-${size})`)
    expect(r).toContain(`line-height: var(--line-height-${size})`)
  })

  test.each([
    ['settings/McpSettings.vue', '.state {'],
    ['settings/accounts/AccountRow.vue', '.status {'],
    ['settings/oauth/OAuthClientsSettings.vue', '.inuse {'],
    ['settings/SignatureSettings.vue', '.dirty {'],
    ['settings/ai/ChatGPTAccountRow.vue', '.badge {'],
  ])('1 行の状態ピル %s %s は font-size xs と対の line-height を持つ（#231）', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('font-size: var(--font-size-xs)')
    expect(r).toContain('line-height: var(--line-height-xs)')
  })

  test.each([
    ['inbox/ThreadRow.vue', '.sender {', 'sm'],
    ['inbox/ThreadRow.vue', '.subject {', 'sm'],
    ['inbox/ThreadRow.vue', '.snippet {', 'sm'],
    ['inbox/ThreadRow.vue', '.count {', 'xs'],
    ['inbox/ThreadRow.vue', '.clip {', 'xs'],
    ['inbox/ThreadRow.vue', '.date {', 'xs'],
    ['inbox/ThreadRow.vue', '.reauth {', 'xs'],
    ['drafts/DraftRow.vue', '.to {', 'sm'],
    ['drafts/DraftRow.vue', '.subject {', 'sm'],
    ['drafts/DraftRow.vue', '.snippet {', 'sm'],
    ['drafts/DraftRow.vue', '.date {', 'xs'],
  ])(
    '一覧の行の 1 行省略の文字 %s %s は font-size %s と対の line-height を持つ（#432）',
    (file, selector, size) => {
      const r = rule(file, selector)
      expect(r).toContain(`font-size: var(--font-size-${size})`)
      expect(r).toContain(`line-height: var(--line-height-${size})`)
    },
  )
})

describe('タブ先頭の題と xs の文字の行間（T4・T8・#436）', () => {
  test.each([['settings/ai/features/AIUseTab.vue'], ['settings/ai/providers/AIConnectTab.vue']])(
    '%s のタブ先頭の題 .sh は McpSettings の .heading と同じ base 16（下位の題 sm 14 より大きい）',
    (file) => {
      const sh = rule(file, '.sh {')
      const heading = rule('settings/McpSettings.vue', '.heading {')
      expect(sh).toContain('font-size: var(--font-size-base)')
      expect(heading).toContain('font-size: var(--font-size-base)')
      expect(sh).toContain('font-weight: 600')
      expect(rule('settings/ai/features/InboxZeroReasons.vue', '.sub {')).toContain(
        'font-size: var(--font-size-sm)',
      )
    },
  )

  test.each([
    ['account/AccountItem.vue', '.email {'],
    ['thread/MessageCard.vue', '.clip {'],
    ['compose/AttachmentBar.vue', '.attach-tag {'],
    ['settings/accounts/ColorPicker.vue', '.current {'],
  ])('xs の文字 %s %s は font-size と対の line-height を持つ', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('font-size: var(--font-size-xs)')
    expect(r).toContain('line-height: var(--line-height-xs)')
  })
})

describe('sm の見出しの行間（T・#438）', () => {
  test.each([
    ['settings/ai/AIDestinationBox.vue', '.dest-title {'],
    ['settings/oauth/OAuthClientForm.vue', '.title {'],
    ['thread/ai/ThreadAiPanel.vue', '.heading {'],
  ])('sm の見出し %s %s は font-size と対の line-height を持つ', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('font-size: var(--font-size-sm)')
    expect(r).toContain('line-height: var(--line-height-sm)')
  })
})

describe('フォームのグループ見出し legend は h4 の段（T・#439）', () => {
  test('AIChatGPTLogin の .legend は sm 14・SemiBold・--ink で、同じファイルの h4 の .sub と段が揃う', () => {
    const legend = rule('settings/ai/AIChatGPTLogin.vue', '.legend {')
    expect(legend).toContain('font-size: var(--font-size-sm)')
    expect(legend).toContain('font-weight: 600')
    expect(legend).toContain('line-height: var(--line-height-sm)')
    expect(legend).toContain('color: var(--ink)')
    expect(legend).not.toContain('--mute')
  })

  test('InboxZeroReasons の legend（.sub）も sm・600 で、設定画面内のグループ見出しが揃う', () => {
    const sub = rule('settings/ai/features/InboxZeroReasons.vue', '.sub {')
    expect(sub).toContain('font-size: var(--font-size-sm)')
    expect(sub).toContain('font-weight: 600')
    expect(sub).toContain('line-height: var(--line-height-sm)')
  })
})
