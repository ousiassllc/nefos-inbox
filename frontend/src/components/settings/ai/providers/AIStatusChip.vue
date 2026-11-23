<script setup lang="ts">
// 設定の AI の状態チップ（接続済み・未接続・つなぎ直しが必要・準備中）。色だけでなく文言で伝える。
// ガイド C4 のステータスのピル（brand-guide main b53610d の .pill）: 文字は状態色・地は 10%・枠なし・padding 3×12・--r-full。
// off は中立（--mute）で同じ形にする。
defineProps<{ kind: 'ok' | 'off' | 'warn' | 'wait' }>()
</script>

<template>
  <span class="status" :class="`status-${kind}`" :data-status="kind"><slot /></span>
</template>

<style scoped>
.status {
  flex: none;
  display: inline-flex;
  align-items: center;
  padding: 3px var(--s-3);
  border-radius: var(--r-full);
  background: var(--status-soft);
  color: var(--status-color);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
  white-space: nowrap;
}

.status-ok {
  --status-color: var(--success);
  --status-soft: var(--success-soft);
}

.status-warn {
  --status-color: var(--warning);
  --status-soft: var(--warning-soft);
}

.status-wait {
  --status-color: var(--info);
  --status-soft: var(--info-soft);
}

.status-off {
  --status-color: var(--mute);
  --status-soft: color-mix(in srgb, var(--mute) 10%, transparent);
}
</style>
