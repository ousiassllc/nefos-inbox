<script setup lang="ts">
import { ArrowLeft } from '@iconoir/vue'
import { settings as msg } from '../../../../lib/messages'
import { useUiStore } from '../../../../stores/ui'
import BaseButton from '../../../ui/BaseButton.vue'

// このPCでは、アプリが用意するローカルAIを使えない（Windows 以外・ARM64・古い Windows・メモリ不足。#458）。
// 理由（Go が日本語 1 文で返す）と、クラウドの AI（ChatGPT プラン・Anthropic・OpenAI 互換）の案内、つなぎ先の一覧へ戻る導線を出す。導入のボタンは出さない。
defineProps<{ reason?: string }>()
const ui = useUiStore()
const t = msg.ai.local
</script>

<template>
  <div class="unsupported" data-testid="local-ai-unsupported">
    <h4 class="sub">{{ t.unsupportedHeading }}</h4>
    <p v-if="reason" class="reason" data-testid="local-ai-unsupported-reason">{{ reason }}</p>
    <p class="cloud">{{ t.unsupportedCloud }}</p>
    <div class="actions">
      <BaseButton variant="secondary" size="sm" data-testid="local-ai-back-to-list" @click="ui.aiProviderPage = undefined">
        <ArrowLeft class="icon icon-sm" aria-hidden="true" />{{ t.unsupportedBack }}
      </BaseButton>
    </div>
  </div>
</template>

<style scoped>
.unsupported {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sub {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.reason,
.cloud {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.actions {
  display: flex;
  gap: var(--s-2);
}
</style>
