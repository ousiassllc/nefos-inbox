<script setup lang="ts">
import { Server } from '@iconoir/vue'
import { computed } from 'vue'
import blossomBlack from '../../../../assets/provider-logos/OAI_OpenAI-Blossom_Black.svg'
import blossomWhite from '../../../../assets/provider-logos/OAI_OpenAI-Blossom_White.svg'
import type { AIProviderId } from './aiProviders'

// プロバイダのアイコン枠（#315）。一覧のカードは、アイコンを必ずこのコンポーネント 1 つを通して出す。
// 公式ロゴがあるプロバイダ（ChatGPT＝OpenAI の Blossom）は LOGOS の公式ファイルをそのまま出す（2026-11-23。色替え・切り抜きはしない。
// 地が明るい Light は黒、暗い Dark は白の公式ファイル）。素材の出どころと利用条件は assets/provider-logos/NOTICE。
// 公式ロゴが使えない・無いものは、頭文字のタイル（A・T。色分けなし）と、頭文字の無い OpenAI 互換だけ Iconoir の server。
// Anthropic は商標のガイドラインが事前の許可を求めるため、許可が出るまで頭文字のまま（NOTICE）。
// 装飾なので読み上げない（カードにはサービス名の文字がある）。
const props = defineProps<{ id: AIProviderId }>()

const LOGOS: Partial<Record<AIProviderId, { light: string; dark: string }>> = {
  chatgpt: { light: blossomBlack, dark: blossomWhite },
}
const TILES: Partial<Record<AIProviderId, string>> = {
  anthropic: 'A',
  typesafe: 'T',
}
const logo = computed(() => LOGOS[props.id])
const initial = computed(() => TILES[props.id])
</script>

<template>
  <span class="provider-icon" :data-provider-icon="id" aria-hidden="true">
    <picture v-if="logo" class="logo">
      <source media="(prefers-color-scheme: dark)" :srcset="logo.dark" />
      <img :src="logo.light" alt="" width="32" height="32" draggable="false" />
    </picture>
    <template v-else-if="initial">{{ initial }}</template>
    <Server v-else class="icon" />
  </span>
</template>

<style scoped>
.provider-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--ink);
  font-family: var(--font-sans);
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: 1;
}
.logo,
.logo img {
  display: block;
  width: 100%;
  height: 100%;
}
</style>
