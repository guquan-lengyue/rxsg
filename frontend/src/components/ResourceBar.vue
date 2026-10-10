<script setup lang="ts">
import { computed, ref } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { resIcon } from '@/assets/img'
import TipBox from '@/components/ui/TipBox.vue'
import type { CityResource } from '@/types'

interface ResItem {
  key: string
  label: string
  value: number
  max: number
}

const props = defineProps<{ resource: CityResource | null }>()

const items = computed<ResItem[]>(() => {
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

// 悬浮提示：文案取资源 label 与当前数量，坐标跟随鼠标移动更新。
const hover = ref<{ item: ResItem; x: number; y: number } | null>(null)

function showTip(item: ResItem, e: MouseEvent): void {
  hover.value = { item, x: e.clientX, y: e.clientY }
}

function moveTip(e: MouseEvent): void {
  if (hover.value) {
    hover.value.x = e.clientX
    hover.value.y = e.clientY
  }
}

function hideTip(): void {
  hover.value = null
}
</script>

<template>
  <div class="resource-bar">
    <div
      v-for="item in items"
      :key="item.key"
      class="resource-item"
      @mouseenter="showTip(item, $event)"
      @mousemove="moveTip"
      @mouseleave="hideTip"
    >
      <img class="ricon" :src="resIcon(item.key)" :alt="item.label" />
      <span class="label">{{ item.label }}</span>
      <span class="value">{{ item.value }}<small v-if="item.max"> / {{ item.max }}</small></span>
    </div>
    <TipBox
      v-if="hover"
      :x="hover.x"
      :y="hover.y"
      :title="hover.item.label"
      :lines="[`当前数量：${hover.item.value}${hover.item.max ? ` / ${hover.item.max}` : ''}`]"
    />
  </div>
</template>

<style scoped>
.resource-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  padding: 12px 16px;
  border: 8px solid transparent;
  border-image: url('/images/board_tip.png') 6 6 6 6 fill / 8px round;
  background: transparent;
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