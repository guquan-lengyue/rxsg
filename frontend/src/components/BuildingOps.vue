<script setup lang="ts">
// 建筑操作区：升级 / 取消升级 / 拆除（含二次确认）/ 取消拆除 / 资源地转换（仅资源田）。
// 从 BuildingPanel 抽出为可复用组件，供通用建筑面板与官署/马厩专面板复用，
// 保证按钮、文案与二次确认逻辑单一实现。
import { computed, ref } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import type { BuildingDetail } from '@/types'

const props = withDefaults(
  defineProps<{
    detail: BuildingDetail | null
    busy?: boolean
  }>(),
  { busy: false },
)

const emit = defineEmits<{
  (e: 'upgrade'): void
  (e: 'stop'): void
  (e: 'destroy'): void
  (e: 'destroy-all'): void
  (e: 'cancel-destroy'): void
  (e: 'exchange'): void
}>()

const next = computed(() => props.detail?.next ?? null)
const upgrading = computed(() => props.detail?.state === 1)
const destroying = computed(() => props.detail?.state === 2)
// 资源地（新库 building_id 2..5，对齐后端 isResourceField）可做资源地转换（原版 bid<5 且 state==0）。
const isResource = computed(() => {
  const bid = props.detail?.bid
  return bid !== undefined && bid >= 2 && bid <= 5
})

// 拆除二次确认（对齐原版 DestroyPanel：彻底拆除 / 拆除一级 / 取消）。
const showDestroy = ref(false)

function askDestroy(): void {
  showDestroy.value = true
}

function destroyAll(): void {
  showDestroy.value = false
  emit('destroy-all')
}

function destroyLevel(): void {
  showDestroy.value = false
  emit('destroy')
}
</script>

<template>
  <div class="actions">
    <button v-if="upgrading" class="stop" :disabled="busy" @click="emit('stop')">取消升级</button>
    <button
      v-else-if="destroying"
      class="stop"
      :disabled="busy"
      @click="emit('cancel-destroy')"
    >
      {{ zhCN.city.cancelDestroy }}
    </button>
    <template v-else>
      <button v-if="isResource" class="exchange" :disabled="busy" @click="emit('exchange')">
        {{ zhCN.city.exchangeBtn }}
      </button>
      <button class="destroy" :disabled="busy" @click="askDestroy">
        {{ zhCN.city.destroy }}
      </button>
      <button class="upgrade" :disabled="busy || !next?.canUpgrade" @click="emit('upgrade')">
        升级
      </button>
    </template>
  </div>

  <!-- 拆除二次确认（对齐原版 DestroyPanel：彻底拆除 / 拆除一级 / 取消） -->
  <div v-if="showDestroy" class="destroy-mask">
    <div class="destroy-panel u-modal">
      <p class="destroy-text">{{ zhCN.city.destroyConfirmText }}</p>
      <div class="destroy-actions">
        <button class="all" :disabled="busy" @click="destroyAll">
          {{ zhCN.city.destroyAll }}
        </button>
        <button class="level" :disabled="busy" @click="destroyLevel">
          {{ zhCN.city.destroyLevel }}
        </button>
        <button class="cancel" :disabled="busy" @click="showDestroy = false">
          {{ zhCN.city.cancel }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.actions button {
  padding: 6px 16px;
  color: var(--text);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.upgrade {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.upgrade:disabled,
.stop:disabled,
.exchange:disabled,
.destroy:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.stop,
.exchange,
.destroy {
  background: transparent;
}

/* 拆除二次确认（原版 DestroyPanel 皮肤）。定位到最近的定位祖先：
   BuildingPanel 内为 .panel（覆盖整面板）；官署/马厩内为 .skin-mask（覆盖弹窗遮罩）。 */
.destroy-mask {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 12px;
  background: rgba(0, 0, 0, 0.5);
}

.destroy-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 240px;
  padding: 14px;
}

.destroy-text {
  margin: 0;
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
}

.destroy-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
}

.destroy-actions button {
  min-width: 68px;
  padding: 5px 12px;
  color: var(--text);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.destroy-actions .all {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.destroy-actions .level,
.destroy-actions .cancel {
  background: transparent;
}
</style>
