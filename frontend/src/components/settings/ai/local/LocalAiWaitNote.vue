<script setup lang="ts">
import { settings as msg } from '../../../../lib/messages'

// ローカルAI（#458）の生成の待ち表示。AI の生成（下書き・要約・質問・Inbox Zero の判断）は、生成中のスピナーの近くにこの 1 行を置く。
// 使う AI が local のときだけ出す（タイムアウトは 300 秒。ほかの AI は出さない）。自動で生成を始めることはしない。
defineProps<{ provider?: string }>()
</script>

<template>
  <span v-if="provider === 'local'" class="wait" role="status" data-testid="local-ai-wait">{{ msg.ai.local.waitNote }}</span>
</template>

<style scoped>
.wait {
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
