<script setup lang="ts">
import { computed } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { resIcon } from '@/assets/img'
import type { CityResource } from '@/types'

const props = defineProps<{ resource: CityResource | null }>()

const items = computed(() => {
  const r = props.resource
  if (!r) {
    return []
  }
  return [
    { key: 'food', label: zhCN.resource.food, value: r.food, max: r.foodMax },
    { key: 'wood', label: zhCN.resource.wood, value: r.wood, max: r.woodMax },
    { key: 'rock', label: zhCN.resource.rock, value: r.rock, max: r.rockMax },
    { key: 'iron', label: zhCN.resource.iron, value: r.iron, max: r.ironMax },
    { key: 'gold', label: zhCN.resource.gold, value: r.gold, max: r.goldMax },
    { key: 'people', label: zhCN.resource.people, value: r.people, max: r.peopleMax },
    { key: 'morale', label: zhCN.resource.morale, value: r.morale, max: 100 },
  ]
})
</script>

<template>
  <div class="resource-bar">
    <div v-for="item in items" :key="item.key" class="resource-item">
      <img class="ricon" :src="resIcon(item.key)" :alt="item.label" />
      <span class="label">{{ item.label }}</span>
      <span class="value">{{ item.value }}<small v-if="item.max"> / {{ item.max }}</small></span>
    </div>
  </div>
</template>

<style scoped>
.resource-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  padding: 12px 16px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ricon {
  width: 22px;
  height: 22px;
  object-fit: contain;
}

.resource-item .label {
  color: var(--text-dim);
  font-size: 13px;
}

.resource-item .value {
  color: var(--accent);
  font-weight: 600;
}

.resource-item small {
  color: var(--text-dim);
  font-weight: 400;
}
</style>