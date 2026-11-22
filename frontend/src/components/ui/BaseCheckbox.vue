<script setup lang="ts">
defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <input
    type="checkbox"
    class="box"
    :checked="modelValue"
    @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
  />
</template>

<style scoped>
/* ガイドのチェックボックス（O8）。ブラウザ標準にせず、ChatGPTAccountRow の .radio と同じ作りにする。
   外枠 --mute・18px・--r-sm・選択時は --accent 地に --bone のチェック・フォーカスは --accent の outline（3px・offset 2px）・無効は opacity .4。 */
.box {
  appearance: none;
  flex: none;
  display: grid;
  place-content: center;
  width: 18px;
  height: 18px;
  margin: 0;
  border: 1px solid var(--mute);
  border-radius: var(--r-sm);
  background: var(--surface);
  cursor: pointer;
}

.box:checked {
  border-color: var(--accent);
  background: var(--accent);
}

.box:checked::after {
  content: '';
  width: 5px;
  height: 9px;
  margin-bottom: 2px;
  border: solid var(--bone);
  border-width: 0 2px 2px 0;
  transform: rotate(45deg);
}

.box:focus-visible {
  outline: 3px solid var(--accent);
  outline-offset: 2px;
}

.box:disabled {
  opacity: 0.4;
  cursor: default;
}
</style>
