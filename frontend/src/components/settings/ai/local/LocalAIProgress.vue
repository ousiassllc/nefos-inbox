<script setup lang="ts">
import { computed } from 'vue'
import type { LocalAIJob } from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { gbNumber, percentOf } from '../../../../lib/localAi/format'
import { settings as msg } from '../../../../lib/messages'

// ローカルAIのジョブ（導入・モデルの取得）の進み。段階の名前＋進捗バー（%・「1.2 / 1.6 GB」）。バイト数が分からない段階（照合・展開など）は
// 不定の進捗（値なし）にする。色だけに頼らず、数字でも伝える。
const props = defineProps<{ job: LocalAIJob }>()
const t = msg.ai.local

const stage = computed(() => t.stages[props.job.stage] ?? props.job.stage)
const percent = computed(() => percentOf(props.job.completedBytes, props.job.totalBytes))
const bytes = computed(() =>
  props.job.totalBytes > 0
    ? t.progressBytes(gbNumber(props.job.completedBytes), `${gbNumber(props.job.totalBytes)} GB`)
    : undefined,
)
</script>

<template>
  <div class="progress" data-testid="local-ai-progress">
    <div class="line">
      <span class="stage">{{ stage }}</span>
      <span v-if="percent !== undefined" class="figures">{{ t.progressPercent(percent) }}<template v-if="bytes">（{{ bytes }}）</template></span>
    </div>
    <div
      class="bar"
      :class="{ indeterminate: percent === undefined }"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="percent"
      :aria-label="t.progressLabel(stage)"
    >
      <div class="fill" :style="{ width: `${percent ?? 0}%` }" />
    </div>
  </div>
</template>

<style scoped>
.progress {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  min-width: 0;
}

.line {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: var(--s-2);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.figures {
  color: var(--mute);
  font-variant-numeric: tabular-nums;
}

.bar {
  height: var(--s-2);
  overflow: hidden;
  border-radius: var(--r-full);
  background: var(--accent-soft);
}

.fill {
  height: 100%;
  border-radius: var(--r-full);
  background: var(--accent);
  transition: width var(--motion-fade-in) var(--ease-out);
}

@media (prefers-reduced-motion: reduce) {
  .fill {
    transition: none;
  }
}
</style>
