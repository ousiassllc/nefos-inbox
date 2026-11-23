import { describe, expect, test } from 'bun:test'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import type { AIProviderId } from './aiProviders'
import ProviderIcon from './ProviderIcon.vue'

const logosDir = new URL('../../../../assets/provider-logos/', import.meta.url)
const icon = (id: AIProviderId) => mount(ProviderIcon, { props: { id } })

describe('ProviderIcon（公式ロゴ。2026-11-23）', () => {
  test('ChatGPT は公式ロゴ（OpenAI の Blossom）。Light は黒・Dark は白のファイル。頭文字は出さない', () => {
    const w = icon('chatgpt')
    const img = w.get('picture img')
    expect(img.attributes('src')).toContain('OAI_OpenAI-Blossom_Black.svg')
    const dark = w.get('picture source[media="(prefers-color-scheme: dark)"]')
    expect(dark.attributes('srcset')).toContain('OAI_OpenAI-Blossom_White.svg')
    expect(img.attributes('alt')).toBe('')
    expect(w.text()).toBe('')
  })

  test('公式ロゴを使わないプロバイダは、頭文字のタイル（Anthropic・TypeSafe AI）と server アイコン（OpenAI 互換）のまま', () => {
    expect(icon('anthropic').text()).toBe('A')
    expect(icon('typesafe').text()).toBe('T')
    const openai = icon('openai')
    expect(openai.text()).toBe('')
    expect(openai.find('img').exists()).toBe(false)
    expect(openai.find('svg.icon').exists()).toBe(true)
    for (const id of ['anthropic', 'typesafe'] as const)
      expect(icon(id).find('img').exists()).toBe(false)
  })

  test('ロゴのファイルは公式の配布物のまま（自作・加工をしない）。NOTICE に出どころと SHA-256 がある', () => {
    const notice = readFileSync(new URL('NOTICE', logosDir), 'utf8')
    const files = ['OAI_OpenAI-Blossom_Black.svg', 'OAI_OpenAI-Blossom_White.svg']
    for (const f of files) {
      const sha = createHash('sha256')
        .update(readFileSync(new URL(f, logosDir)))
        .digest('hex')
      expect(notice).toContain(`${sha}  ${f}`)
    }
    expect(notice).toContain('https://cdn.openai.com/brand/openai-logos.zip')
  })
})
