<script setup lang="ts">
import { ref } from 'vue'
import type { LocalAIStatus } from '../../../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { settings as msg } from '../../../../lib/messages'
import { useLocalAiStore } from '../../../../stores/localAi'
import BaseButton from '../../../ui/BaseButton.vue'
import ConfirmDialog from '../../../ui/ConfirmDialog.vue'
import AIStatusChip from '../providers/AIStatusChip.vue'
import LocalAIModelRow from './LocalAIModelRow.vue'

// 導入済みのローカルAI（#458）。Ollama の状態（起動中／停止中・「停止」）、モデルの一覧（取得・中止・削除）、「ローカルAIを削除」。
// 削除は、どちらも確認のうえ行う（モデルの削除・ローカルAIの削除）。ジョブ中は「停止」「ローカルAIを削除」を押せない（Go も拒む）。
const props = defineProps<{ status: LocalAIStatus }>()
const store = useLocalAiStore()
const t = msg.ai.local

/** 削除の確認を出しているモデル（undefined は出していない）。 */
const modelTarget = ref<{ name: string; displayName: string }>()
const removeOpen = ref(false)

async function confirmDeleteModel(): Promise<void> {
  const target = modelTarget.value
  modelTarget.value = undefined
  if (target) await store.deleteModel(target.name)
}

async function confirmRemove(): Promise<void> {
  removeOpen.value = false
  await store.remove()
}
</script>

<template>
  <div class="installed" data-testid="local-ai-installed">
    <div class="state">
      <h4 class="sub">{{ t.installedHeading }}</h4>
      <span class="version">{{ t.version(props.status.version) }}</span>
      <AIStatusChip :kind="props.status.running ? 'ok' : 'off'" data-testid="local-ai-running">
        {{ props.status.running ? t.running : t.stopped }}
      </AIStatusChip>
      <BaseButton
        v-if="props.status.running"
        variant="secondary"
        size="sm"
        :disabled="store.busy || props.status.job != null"
        data-testid="local-ai-stop"
        @click="store.stop()"
      >
        {{ t.stop }}
      </BaseButton>
    </div>
    <p class="hint">{{ t.stopHint }}</p>

    <div class="group">
      <h4 class="sub">{{ t.modelsHeading }}</h4>
      <p class="hint">{{ t.modelsHint }}</p>
    </div>
    <ul class="box" :aria-label="t.modelListLabel" data-testid="local-ai-models">
      <LocalAIModelRow
        v-for="m in props.status.models ?? []"
        :key="m.name"
        :model="m"
        :job="props.status.job ?? undefined"
        :busy="store.busy"
        @pull="store.pull(m.name)"
        @cancel="store.cancel()"
        @delete="modelTarget = { name: m.name, displayName: m.displayName }"
      />
    </ul>

    <div class="actions">
      <BaseButton
        variant="danger"
        :disabled="store.busy || props.status.job != null"
        data-testid="local-ai-remove"
        @click="removeOpen = true"
      >
        {{ t.remove }}
      </BaseButton>
    </div>

    <ConfirmDialog
      :open="modelTarget !== undefined"
      :title="t.deleteModelTitle(modelTarget?.displayName ?? '')"
      :detail="t.deleteModelDetail"
      :confirm-label="t.deleteModelConfirm"
      :busy="store.busy"
      @update:open="(v) => !v && (modelTarget = undefined)"
      @confirm="confirmDeleteModel"
    />
    <ConfirmDialog
      v-model:open="removeOpen"
      :title="t.removeTitle"
      :detail="t.removeDetail"
      :confirm-label="t.removeConfirm"
      :busy="store.busy"
      @confirm="confirmRemove"
    />
  </div>
</template>

<style scoped>
.installed {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.state {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
}

.group {
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

.version {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.box {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.box > :deep(.mrow + .mrow) {
  border-top: 1px solid var(--border);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
