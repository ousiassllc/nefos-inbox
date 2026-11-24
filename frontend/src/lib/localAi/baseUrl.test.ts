import { describe, expect, test } from 'bun:test'
import { isLoopbackInput } from './baseUrl'

// 入力中の Base URL の loopback 判定は、Go の ai.IsLoopbackBaseURL（厳密一致）と同じ結果にする。
describe('isLoopbackInput', () => {
  test.each([
    ['http://localhost:11434/v1', true],
    ['http://127.0.0.1:1234/v1', true],
    ['http://[::1]:11434/v1', true],
    ['https://localhost/v1', true],
    ['http://localhost', true],
    ['  http://localhost:1/v1  ', true],
    ['http://LOCALHOST:11434/v1', false],
    ['http://Localhost/v1', false],
    ['http://127.1:11434/v1', false],
    ['http://2130706433/v1', false],
    ['http://0x7f.0.0.1/v1', false],
    ['http://[0:0:0:0:0:0:0:1]:11434/v1', false],
    ['http://localhost.:11434/v1', false],
    ['http://user@localhost/v1', false],
    ['http://localhost@evil.example/v1', false],
    ['http://localhost:11434/v1?x=1', false],
    ['ftp://localhost/v1', false],
    ['localhost:11434', false],
    ['', false],
  ])('%s → %s', (raw, want) => {
    expect(isLoopbackInput(raw)).toBe(want)
  })
})
