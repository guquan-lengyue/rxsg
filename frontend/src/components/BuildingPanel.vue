<script setup lang="ts">
import { computed } from 'vue'

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
const emit = defineEmits<{ (e: 'upgrade'): void; (e: 'stop'): void; (e: 'close'): void }>()

const next = computed(() => props.detail?.next ?? null)
const upgrading = computed(() => props.detail?.state === 1)

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
  <aside class="panel">
    <header class="panel-head">
      <strong>{{ detail?.name || '建筑' }}</strong>
      <button class="close" @click="emit('close')">×</button>
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
        <button
          v-if="upgrading"
          class="stop"
          :disabled="busy"
          @click="emit('stop')"
        >
          取消升级
        </button>
        <button
          v-else
          class="upgrade"
          :disabled="busy || !next?.canUpgrade"
          @click="emit('upgrade')"
        >
          升级
        </button>
      </div>
    </template>
  </aside>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 220px;
  padding: 14px 16px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.panel-head strong {
  color: var(--accent);
}

.close {
  color: var(--text-dim);
  background: transparent;
  border: none;
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
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
.stop:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.stop {
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