<script setup lang="ts">
import { computed, ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { img } from '@/assets/img'
import type { LotteryBoard, LotteryItem, LotteryWin } from '@/api/lottery'

const props = defineProps<{
  todayCount: number
  moneyOk: boolean
  board: LotteryBoard
  winInfo: LotteryWin
  restartCount: number
  lastWin: LotteryItem | null
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'start'): void
  (e: 'claim-win'): void
  (e: 'reward'): void
  (e: 'auto-reward'): void
  (e: 'restart', winId: number, winType: number): void
  (e: 'use-money', count: number): void
  (e: 'close'): void
}>()

const TIER_LABELS = ['1等奖', '2等奖', '3等奖', '4等奖', '5等奖', '6等奖', '7等奖', '8等奖']
const moneyCount = ref(2)
const failed = ref<Record<string, boolean>>({})

const tiers = computed(() => props.board.slice(0, 8))
const hasBoard = computed(() => tiers.value.some((t) => t.length > 0))

function keyOf(item: LotteryItem): string {
  return item.isArmor ? `a${item.id}` : `g${item.gid}`
}

function itemIcon(item: LotteryItem): string {
  return item.isArmor ? img(`armor/${item.id}.png`) : img(`item_${item.gid}.png`)
}

function isWin(item: LotteryItem): boolean {
  const w = props.winInfo
  if (w.type < 0) {
    return false
  }
  return w.type === (item.isArmor ? 1 : 0) && w.id === (item.isArmor ? item.id : item.gid)
}

function onImgError(item: LotteryItem): void {
  failed.value = { ...failed.value, [keyOf(item)]: true }
}

const winText = computed(() => {
  const w = props.winInfo
  if (w.type < 0) {
    return '尚未抽奖'
  }
  const label = w.type === 1 ? `装备${w.id}` : `道具${w.id}`
  return `本次抽中：${label} ×${w.count}`
})
</script>

<template>
  <SkinDialog title="幸运宝匣" wide @close="emit('close')">
    <div class="status">
      <span>今日已抽：{{ todayCount }}</span>
      <span>重开次数：{{ restartCount }}</span>
      <span>元宝：{{ moneyOk ? '充足' : '不足' }}</span>
    </div>

    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <template v-else>
      <!-- 8 格盘面 -->
      <div v-if="hasBoard" class="board">
        <div v-for="(tier, idx) in tiers" :key="idx" class="cell">
          <span class="tier">{{ TIER_LABELS[idx] }}</span>
          <div class="items">
            <div
              v-for="item in tier"
              :key="keyOf(item)"
              class="item"
              :class="{ 'is-win': isWin(item) }"
            >
              <img
                v-if="!failed[keyOf(item)]"
                class="icon"
                :src="itemIcon(item)"
                :alt="item.name"
                @error="onImgError(item)"
              />
              <span v-else class="fallback">{{ item.name || keyOf(item) }}</span>
              <span class="name">{{ item.name || `#${keyOf(item)}` }}</span>
              <span class="count">×{{ item.count }}</span>
            </div>
            <span v-if="!tier.length" class="u-hint">—</span>
          </div>
        </div>
      </div>
      <p v-else class="u-hint">盘面未生成，点击下方「领奖并重开」获取当前盘面。</p>

      <p class="u-hint">{{ winText }}</p>
      <p v-if="lastWin" class="u-hint">
        最近领奖：{{ lastWin.name || (lastWin.isArmor ? `装备${lastWin.id}` : `道具${lastWin.gid}`) }}
        ×{{ lastWin.count }}
      </p>

      <div class="money">
        <label>使用元宝数量</label>
        <input v-model.number="moneyCount" class="u-input" type="number" min="1" />
        <button
          class="u-btn u-btn--yellow"
          type="button"
          :disabled="busy"
          @click="emit('use-money', moneyCount)"
        >
          使用元宝
        </button>
      </div>
    </template>

    <template #footer>
      <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="emit('start')">
        开始抽奖
      </button>
      <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('claim-win')">
        领取奖励
      </button>
      <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('reward')">
        领奖并重开
      </button>
      <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('auto-reward')">
        一键领奖重开
      </button>
      <button
        class="u-btn u-btn--yellow"
        type="button"
        :disabled="busy"
        @click="emit('restart', winInfo.id, winInfo.type)"
      >
        重开
      </button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.status {
  display: flex;
  gap: 16px;
  font-size: 13px;
  color: var(--text-dim);
}

.board {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6px;
  margin: 8px 0;
}

.cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 96px;
  padding: 6px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.tier {
  color: var(--accent);
  font-size: 12px;
}

.items {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--text-dim);
}

.item.is-win {
  color: var(--accent);
}

.icon {
  width: 22px;
  height: 22px;
  object-fit: contain;
}

.fallback {
  min-width: 22px;
  color: var(--text-dim);
  font-size: 11px;
  text-align: center;
}

.name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  color: var(--accent);
}

.money {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-dim);
}

.u-input {
  width: 64px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}
</style>
