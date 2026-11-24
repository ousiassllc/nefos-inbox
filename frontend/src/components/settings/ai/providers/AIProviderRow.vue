<script setup lang="ts">
import { computed } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import BaseButton from '../../../ui/BaseButton.vue'
import AIStatusChip from './AIStatusChip.vue'
import { type AIProviderId, connectionState, needsConsent } from './aiProviders'
import ProviderIcon from './ProviderIcon.vue'

// 「AI のつなぎ先」の 1 行（#376）。プロバイダごとに、状態チップとつなぐ操作（ログイン／API キーを追加／接続を外す）だけを置く。
// モデルはここでは選ばない（「機能ごとに使う AI」）。390px 幅では、行の中身が折り返して操作が下に回る。
const props = defineProps<{ id: AIProviderId; busy?: boolean }>()
const emit = defineEmits<{ connect: []; open: []; disconnect: [] }>()
const store = useSettingsStore()
const t = msg.ai.connect

const state = computed(() => connectionState(props.id, store.aiConfig))
const consentPending = computed(() => needsConsent(props.id, store.aiConfig))
const chip = computed(
  () => ({ connected: t.connected, disconnected: t.disconnected, reconnect: t.reconnect })[state.value],
)
const chipKind = computed(
  () => ({ connected: 'ok', disconnected: 'off', reconnect: 'warn' })[state.value] as 'ok' | 'off' | 'warn',
)
const name = computed(() => msg.ai.providerList.names[props.id] ?? props.id)
const isLocal = computed(() => props.id === 'local')
const connectLabel = computed(() => {
  if (consentPending.value) return t.consent
  return props.id === 'chatgpt' ? t.login : t.addKey
})
const note = computed(() => {
  if (consentPending.value) return t.consentNote
  if (isLocal.value) return msg.ai.local.rowNote
  if (props.id === 'typesafe') return t.typesafeNote
  if (props.id === 'chatgpt' && state.value === 'connected') return store.aiConfig?.chatgptEmail
  return undefined
})
</script>

<template>
  <li class="prow" :data-testid="`ai-provider-${id}`" :data-state="state">
    <ProviderIcon :id="id" />
    <div class="meta">
      <div class="pname">
        <span class="name">{{ name }}</span>
        <AIStatusChip :kind="chipKind" :data-testid="`ai-provider-status-${id}`">{{ chip }}</AIStatusChip>
      </div>
      <p v-if="note" class="note">{{ note }}</p>
    </div>
    <div class="actions">
      <!-- ローカルAIは API キーもログインも無い（導入・つなぎ方の設定だけ）。常に「設定」でページを開き、「接続を外す」は無い。 -->
      <BaseButton v-if="isLocal" variant="secondary" size="sm" :disabled="busy" :data-testid="`ai-provider-open-${id}`" :aria-label="msg.ai.providerList.open(name)" @click="emit('open')">
        {{ t.manage }}
      </BaseButton>
      <template v-else-if="state === 'connected'">
        <BaseButton variant="ghost" size="sm" :disabled="busy" :data-testid="`ai-provider-open-${id}`" :aria-label="msg.ai.providerList.open(name)" @click="emit('open')">
          {{ t.manage }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" :disabled="busy" :data-testid="`ai-provider-disconnect-${id}`" :aria-label="t.disconnectFor(name)" @click="emit('disconnect')">
          {{ t.disconnect }}
        </BaseButton>
      </template>
      <BaseButton v-else variant="secondary" size="sm" :disabled="busy" :data-testid="`ai-provider-connect-${id}`" :aria-label="t.connectFor(name, connectLabel)" @click="emit('connect')">
        {{ connectLabel }}
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.prow {
  display: flex;
  align-items: center;
  gap: var(--s-3);
  min-height: var(--list-row-h);
  padding: var(--s-2) var(--s-4);
}

.meta {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--list-title-caption-gap);
  min-width: 0;
}

.pname {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.note {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--s-2);
}

/* 390px 幅: 行はカードになり、名前・チップ・補足の下の行に操作を右寄せで置く。日本語は単語の途中で折らない。 */
@media (max-width: 640px) {
  .prow {
    flex-wrap: wrap;
    padding: var(--s-3);
    line-break: strict;
    word-break: keep-all;
  }

  /* 名前・チップ・補足は、アイコンの右いっぱいに使う。操作は次の行に右寄せで置く。 */
  .meta {
    flex-basis: calc(100% - 32px - var(--s-3));
  }

  .actions {
    flex-basis: 100%;
    justify-content: flex-end;
  }
}
</style>
