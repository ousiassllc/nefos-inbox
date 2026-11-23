// 設定まわりの見た目の回帰テスト（#455・#456。PR #452 のデザイン確認の指摘）。
import { describe, expect, test } from 'bun:test'
import { rule } from './styleRule'

describe('ProviderIcon の頭文字タイル（#455）', () => {
  const body = rule('settings/ai/providers/ProviderIcon.vue', '.provider-icon {')

  test('文字は base 16（隣の公式ロゴ 16px と重さをそろえる）', () => {
    expect(body).toContain('font-size: var(--font-size-base)')
    expect(body).not.toContain('--font-size-sm')
  })

  test('角丸は固定値 6px ではなく --r-md', () => {
    expect(body).toContain('border-radius: var(--r-md)')
    expect(body).not.toMatch(/border-radius:\s*\d/)
  })
})

describe('設定ダイアログのタブのホバー（#456）', () => {
  test('ホバーは文字色 --ink だけ。地（--accent-soft など）は付けない', () => {
    const body = rule('settings/SettingsDialog.vue', '.tab:hover {')
    expect(body).toContain('color: var(--ink)')
    expect(body).not.toContain('background')
  })

  test('選択中の下線は --accent のまま', () => {
    const body = rule('settings/SettingsDialog.vue', '.tab[data-state="active"] {')
    expect(body).toContain('border-bottom-color: var(--accent)')
  })
})
