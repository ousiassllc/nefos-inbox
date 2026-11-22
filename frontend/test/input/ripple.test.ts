import { describe, expect, test } from 'bun:test'
import { spawn } from '../../src/lib/ripple'

function press(attrs: Record<string, string>): HTMLButtonElement {
  const button = document.createElement('button')
  for (const [name, value] of Object.entries(attrs)) button.setAttribute(name, value)
  spawn(button, new PointerEvent('pointerdown', { button: 0, clientX: 4, clientY: 4 }))
  return button
}

describe('spawn（押下リップル）', () => {
  test('aria-busy="true" のボタンには出さない（処理中は disabled にせず aria-busy で示す）', () => {
    expect(press({ 'aria-busy': 'true' }).querySelector('.ripple')).toBeNull()
  })

  test('aria-disabled="true" のボタンには出さない', () => {
    expect(press({ 'aria-disabled': 'true' }).querySelector('.ripple')).toBeNull()
  })

  test('どちらも無ければ出る', () => {
    expect(press({}).querySelector('.ripple')).not.toBeNull()
  })

  test('aria-busy="false"・aria-disabled="false" は押せる状態なので出る', () => {
    expect(
      press({ 'aria-busy': 'false', 'aria-disabled': 'false' }).querySelector('.ripple'),
    ).not.toBeNull()
  })
})
