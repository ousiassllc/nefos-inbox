// Ousiass デザインガイド準拠の回帰テスト（#210）。色（C4・C6）の指定を各部品の <style> で確かめる。
import { describe, expect, test } from 'bun:test'
import { rule } from './styleRule'

describe('状態ピルと同じ色（C4・#210）', () => {
  test('無効な宛先チップは文字 --danger・枠 --danger-border・地 --danger-soft', () => {
    const body = rule('compose/recipient/RecipientChip.vue', '.chip-invalid {')
    expect(body).toContain('border-color: var(--danger-border)')
    expect(body).toContain('background: var(--danger-soft)')
    expect(body).toContain('color: var(--danger)')
  })
})

describe('長文・複数文は --mute にしない（C6・#210）', () => {
  test('本文エディタの署名・引用は --ink', () => {
    const sig = rule('compose/RichTextEditor.vue', '[data-hitomado-signature])')
    const quote = rule('compose/RichTextEditor.vue', '[data-hitomado-quote])')
    expect(sig).toContain('color: var(--ink)')
    expect(quote).toContain('color: var(--ink)')
  })
})

describe('識別色の地の上の文字は薄めない（C2・C6・#425）', () => {
  test('AccountChip のアドレスは opacity で薄めず、継承の --ink のまま', () => {
    const body = rule('account/AccountChip.vue', '.account-chip-email {')
    expect(body).not.toContain('opacity')
    expect(body).not.toContain('color:')
  })
})

describe('AI の状態チップは状態色のピル（C4・#405）', () => {
  const chip = 'settings/ai/providers/AIStatusChip.vue'

  test('文字・地は --status-* で受け、枠なし・padding 3×12 のピル（--r-full）にする（brand-guide b53610d の .pill）', () => {
    const body = rule(chip, '.status {')
    expect(body).not.toContain('border:')
    expect(body).toContain('padding: 3px var(--s-3)')
    expect(body).toContain('border-radius: var(--r-full)')
    expect(body).toContain('background: var(--status-soft)')
    expect(body).toContain('color: var(--status-color)')
  })

  test('ok は success・warn は warning（danger にしない）・wait は info の色にする', () => {
    const ok = rule(chip, '.status-ok {')
    expect(ok).toContain('--status-color: var(--success)')
    expect(ok).toContain('--status-soft: var(--success-soft)')
    const warn = rule(chip, '.status-warn {')
    expect(warn).toContain('--status-color: var(--warning)')
    expect(warn).toContain('--status-soft: var(--warning-soft)')
    expect(warn).not.toContain('--danger')
    const wait = rule(chip, '.status-wait {')
    expect(wait).toContain('--status-color: var(--info)')
    expect(wait).toContain('--status-soft: var(--info-soft)')
  })
})
