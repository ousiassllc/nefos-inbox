<script setup lang="ts">
import { computed } from 'vue'
import type {
  LocalAIJob,
  LocalAIModel,
} from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { gbNumber } from '../../../../lib/localAi/format'
import { settings as msg } from '../../../../lib/messages'
import BaseButton from '../../../ui/BaseButton.vue'
import AIStatusChip from '../providers/AIStatusChip.vue'
import LocalAIProgress from './LocalAIProgress.vue'

// ローカルAIのモデル 1 つ（#458）。名前・説明・サイズ・ライセンス・取得済みバッジと、「取得」／取得中の進捗＋「中止」／「削除」。
// ジョブ（導入・取得）は同時に 1 つなので、別のジョブが動いている間は「取得」「削除」を押せない。
const props = defineProps<{
  model: LocalAIModel
  /** 実行中のジョブ（無ければ undefined）。 */
  job?: LocalAIJob
  /** 操作を呼び出している間（二重に押せないようにする）。 */
  busy?: boolean
}>()
const emit = defineEmits<{ pull: []; cancel: []; delete: [] }>()
const t = msg.ai.local

const pulling = computed(() => props.job?.kind === 'pull' && props.job.model === props.model.name)
const locked = computed(() => props.busy === true || props.job !== undefined)
</script>

<template>
  <li class="mrow" :data-testid="`local-ai-model-${model.name}`" :data-installed="model.installed">
    <div class="meta">
      <div class="mname">
        <span class="name">{{ model.displayName }}</span>
        <AIStatusChip v-if="model.installed" kind="ok" data-testid="local-ai-model-installed">{{ t.modelInstalled }}</AIStatusChip>
      </div>
      <p class="desc">{{ model.description }}</p>
      <p class="spec">{{ t.modelSize(`${gbNumber(model.sizeBytes)} GB`) }} ・ {{ t.modelLicense(model.license) }}</p>
      <LocalAIProgress v-if="pulling && job" :job="job" />
    </div>
    <div class="actions">
      <BaseButton
        v-if="pulling"
        variant="secondary"
        size="sm"
        :disabled="busy"
        :aria-label="t.cancelFor(model.displayName)"
        data-testid="local-ai-model-cancel"
        @click="emit('cancel')"
      >
        {{ t.cancel }}
      </BaseButton>
      <BaseButton
        v-else-if="!model.installed"
        variant="secondary"
        size="sm"
        :disabled="locked"
        :aria-label="t.pullFor(model.displayName)"
        data-testid="local-ai-model-pull"
        @click="emit('pull')"
      >
        {{ t.pull }}
      </BaseButton>
      <BaseButton
        v-else
        variant="ghost"
        size="sm"
        :disabled="locked"
        :aria-label="t.deleteFor(model.displayName)"
        data-testid="local-ai-model-delete"
        @click="emit('delete')"
      >
        {{ t.delete }}
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.mrow {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
}

.meta {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
}

.mname {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.name {
  font-weight: 600;
}

.desc,
.spec {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.desc {
  color: var(--ink);
}

.spec {
  color: var(--mute);
}

.actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--s-2);
}

/* 390px 幅: 操作は名前・説明の下の行に右寄せで置く。日本語は単語の途中で折らない。 */
@media (max-width: 640px) {
  .mrow {
    flex-wrap: wrap;
    padding: var(--s-3);
    line-break: strict;
    word-break: keep-all;
  }

  .meta {
    flex-basis: 100%;
  }

  .actions {
    flex-basis: 100%;
    justify-content: flex-end;
  }
}
</style>
