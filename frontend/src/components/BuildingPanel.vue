<script setup lang="ts">
import { computed, ref } from 'vue'

import { formatLeft } from '@/render/cityGrid'
import { buildingIntro } from '@/assets/img'
import { zhCN } from '@/lang/zh-CN'
import type { BuildingDetail } from '@/types'

const props = defineProps<{
  detail: BuildingDetail | null
  loading: boolean
  error: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'upgrade'): void
  (e: 'stop'): void
  (e: 'destroy'): void
  (e: 'destroy-all'): void
  (e: 'cancel-destroy'): void
  (e: 'exchange'): void
  (e: 'close'): void
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

const costs = computed(() => {
  const n = next.value
  if (!n) {
    return []
  }
  return [
    { key: 'wood', label: zhCN.resource.wood, value: n.woodNeed },
    { key: 'rock', label: zhCN.resource.rock, value: n.rockNeed },
    { key: 'iron', label: zhCN.resource.iron, value: n.ironNeed },
    { key: 'food', label: zhCN.resource.food, value: n.foodNeed },
    { key: 'gold', label: zhCN.resource.gold, value: n.goldNeed },
  ].filter((c) => c.value > 0)
})

const timeText = computed(() => (next.value ? formatLeft(next.value.upgradeTime) : ''))
</script>

<template>
  <aside class="panel u-modal">
    <header class="u-title">
      <strong>{{ detail?.name || '建筑' }}</strong>
      <button class="u-close" title="关闭" @click="emit('close')"></button>
    </header>

    <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
    <p v-else-if="error" class="hint error">{{ error }}</p>

    <template v-else-if="detail">
      <img class="building" :src="buildingIntro(detail.bid)" :alt="detail.name" />
      <div class="row">
        <span class="label">等级</span>
        <span>{{ detail.level }}</span>
      </div>

      <div v-if="upgrading" class="row">
        <span class="label">{{ zhCN.city.upgradeCountdown }}</span>
        <span class="countdown">{{ formatLeft(detail.state_timeleft) }}</span>
      </div>

      <div v-else-if="destroying" class="row">
        <span class="label">{{ zhCN.city.destroying }}</span>
        <span class="countdown">{{ formatLeft(detail.state_timeleft) }}</span>
      </div>

      <template v-else-if="next">
        <div class="row">
          <span class="label">升级至</span>
          <span>Lv{{ next.level }}</span>
        </div>
        <div class="row">
          <span class="label">耗时</span>
          <span>{{ timeText }}</span>
        </div>
        <ul class="costs">
          <li v-for="c in costs" :key="c.key">
            <span>{{ c.label }}</span>
            <span>{{ c.value }}</span>
          </li>
        </ul>
        <p v-if="next.no_upgrade_msg" class="hint error">{{ next.no_upgrade_msg }}</p>
      </template>

      <div class="actions">
        <button v-if="upgrading" class="stop" :disabled="busy" @click="emit('stop')">
          取消升级
        </button>
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
    </template>

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
  </aside>
</template>

<style scoped>
.panel {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 220px;
  padding: 12px 14px;
}

.row {
  display: flex;
  justify-content: space-between;
  font-size: 14px;
}

.label {
  color: var(--text-dim);
}

.countdown {
  color: var(--accent);
}

.building {
  width: 100%;
  height: auto;
  max-height: 120px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  background: var(--bg);
}

.costs {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 8px 0;
  list-style: none;
  border-top: 1px solid var(--panel-border);
  border-bottom: 1px solid var(--panel-border);
}

.costs li {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--text-dim);
}

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

/* 拆除二次确认（原版 DestroyPanel 皮肤） */
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

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}
</style>