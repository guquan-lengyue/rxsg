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

// 元宝小图标：原版 images/lottery/small_money.png
const moneyIcon = img('lottery_small_money.png')

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
    <!-- 面板底图 lottery_bg.png（原版 LotteryBG，九宫格） -->
    <div class="lottery-bg">
      <!-- 标题贴图 lottery_title.png（原版 LotteryTitle；弹窗标题文案不变，此处为原版装饰条） -->
      <div class="lottery-banner" aria-hidden="true"></div>

      <p v-if="loading" class="u-hint">加载中…</p>
      <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

      <template v-else>
        <div class="lottery-main">
          <!-- 转盘：lottery_turntable.png 作底，8 格按原版环形均布 -->
          <div v-if="hasBoard" class="turn">
            <div v-for="(tier, idx) in tiers" :key="idx" class="turn-cell">
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

          <!-- 说明条 lottery_des.png -->
          <aside class="lottery-des">
            <p class="des-text">{{ winText }}</p>
            <p v-if="lastWin" class="des-text">
              最近领奖：{{ lastWin.name || (lastWin.isArmor ? `装备${lastWin.id}` : `道具${lastWin.gid}`) }}
              ×{{ lastWin.count }}
            </p>
          </aside>
        </div>

        <!-- 次数底 lottery_playcount.png + 元宝图标 lottery_small_money.png -->
        <div class="lottery-status">
          <span class="playcount">今日已抽：{{ todayCount }}</span>
          <span>重开次数：{{ restartCount }}</span>
          <span class="money-state">
            <img class="money-icon" :src="moneyIcon" alt="" />元宝：{{ moneyOk ? '充足' : '不足' }}
          </span>
        </div>

        <div class="money">
          <label>使用元宝数量</label>
          <input v-model.number="moneyCount" class="u-input" type="number" min="1" />
          <!-- 付费按钮：lottery_pay 三态（贴图自带文字；DOM 文案保留不改） -->
          <button
            class="lot-btn lot-btn--pay"
            type="button"
            title="使用元宝"
            :disabled="busy"
            @click="emit('use-money', moneyCount)"
          ></button>
        </div>
      </template>
    </div>

    <template #footer>
      <!-- 原版三态按钮贴图；禁用条件与点击行为保持现状 -->
      <button
        class="lot-btn lot-btn--start"
        type="button"
        title="开始抽奖"
        :disabled="busy"
        @click="emit('start')"
      ></button>
      <button
        class="lot-btn lot-btn--get"
        type="button"
        title="领取奖励"
        :disabled="busy"
        @click="emit('claim-win')"
      ></button>
      <button
        class="lot-btn lot-btn--get"
        type="button"
        title="领奖并重开"
        :disabled="busy"
        @click="emit('reward')"
      ></button>
      <button
        class="lot-btn lot-btn--get"
        type="button"
        title="一键领奖重开"
        :disabled="busy"
        @click="emit('auto-reward')"
      ></button>
      <button
        class="lot-btn lot-btn--restart"
        type="button"
        title="重开"
        :disabled="busy"
        @click="emit('restart', winInfo.id, winInfo.type)"
      ></button>
    </template>
  </SkinDialog>
</template>

<style scoped>
/* 面板底：lottery_bg.png（九宫格，与 .u-modal/.u-title 同风格） */
.lottery-bg {
  padding: 8px 12px 12px;
  border: 10px solid transparent;
  border-image: url('/images/lottery_bg.png') 10 10 10 10 fill / 10px round;
}

/* 标题装饰条：lottery_title.png（179×36，原版 LotteryTitle） */
.lottery-banner {
  width: 179px;
  height: 36px;
  margin: 0 auto 6px;
  background: url('/images/lottery_title.png') center / 100% 100% no-repeat;
}

.lottery-main {
  display: flex;
  justify-content: center;
  align-items: flex-start;
  gap: 12px;
}

/* 转盘底：lottery_turntable.png（原版 TurnTable） */
.turn {
  position: relative;
  flex: 0 0 auto;
  width: 380px;
  height: 380px;
  background: url('/images/lottery_turntable.png') center / 100% 100% no-repeat;
}

/* 8 个奖品格按原版环形均布（对应原版 goods0..goods7 的坐标） */
.turn-cell {
  position: absolute;
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 84px;
  min-height: 88px;
  padding: 3px 4px;
  overflow: hidden;
  font-size: 11px;
  color: var(--text-dim);
  transform: translate(-50%, -50%);
}

.turn-cell:nth-child(1) { left: 36%; top: 13%; }
.turn-cell:nth-child(2) { left: 64%; top: 13%; }
.turn-cell:nth-child(3) { left: 88%; top: 37%; }
.turn-cell:nth-child(4) { left: 88%; top: 63%; }
.turn-cell:nth-child(5) { left: 64%; top: 87%; }
.turn-cell:nth-child(6) { left: 36%; top: 87%; }
.turn-cell:nth-child(7) { left: 12%; top: 63%; }
.turn-cell:nth-child(8) { left: 12%; top: 37%; }

.tier {
  color: var(--accent);
  font-size: 11px;
}

.items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--text-dim);
}

.item.is-win {
  color: var(--accent);
}

.icon {
  width: 20px;
  height: 20px;
  object-fit: contain;
}

.fallback {
  min-width: 20px;
  color: var(--text-dim);
  font-size: 10px;
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

/* 发光近似原版 GlowTween.as（悬停）与中奖高亮（黄光呼吸，纯 CSS，无 JS 定时器） */
@keyframes lottery-glow {
  0%,
  100% {
    filter: drop-shadow(0 0 1px #ffd75e);
  }
  50% {
    filter: drop-shadow(0 0 6px #ffd75e);
  }
}
.item:hover .icon,
.item.is-win .icon {
  animation: lottery-glow 1s ease-in-out infinite;
}

/* 说明条：lottery_des.png（原版 LotteryDes） */
.lottery-des {
  flex: 0 0 auto;
  width: 140px;
  min-height: 168px;
  padding: 18px 10px;
  background: url('/images/lottery_des.png') center / 100% 100% no-repeat;
}

.des-text {
  margin: 0 0 6px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--text);
}

/* 次数底：lottery_playcount.png（原版 LotteryPlayCount） */
.lottery-status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin: 8px 0;
  font-size: 13px;
  color: var(--text-dim);
}

.playcount {
  padding: 2px 12px;
  color: var(--text);
  background: url('/images/lottery_playcount.png') center / 100% 100% no-repeat;
}

.money-state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.money-icon {
  width: 16px;
  height: 16px;
  object-fit: contain;
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

/* 原版三态按钮贴图：默认=常态图，:hover=_over，:active=_select，:disabled=_disable。
   贴图自带文字，按钮不再显示 DOM 文案（见模板 title 属性保留原文案）。 */
.lot-btn {
  flex: 0 0 auto;
  padding: 0;
  border: none;
  background: center / 100% 100% no-repeat;
}

.lot-btn--start {
  width: 109px;
  height: 34px;
  background-image: url('/images/lottery_start.png');
}
.lot-btn--start:hover:not(:disabled) {
  background-image: url('/images/lottery_start_over.png');
}
.lot-btn--start:active:not(:disabled) {
  background-image: url('/images/lottery_start_select.png');
}
.lot-btn--start:disabled {
  background-image: url('/images/lottery_start_disable.png');
}

.lot-btn--restart {
  width: 129px;
  height: 34px;
  background-image: url('/images/lottery_restart.png');
}
.lot-btn--restart:hover:not(:disabled) {
  background-image: url('/images/lottery_restart_over.png');
}
.lot-btn--restart:active:not(:disabled) {
  background-image: url('/images/lottery_restart_select.png');
}
.lot-btn--restart:disabled {
  background-image: url('/images/lottery_restart_disable.png');
}

/* lottery_get / lottery_pay 无 _disable 变体，禁用态沿用全局 button:disabled 的降透明度处理 */
.lot-btn--get {
  width: 109px;
  height: 34px;
  background-image: url('/images/lottery_get.png');
}
.lot-btn--get:hover:not(:disabled) {
  background-image: url('/images/lottery_get_over.png');
}
.lot-btn--get:active:not(:disabled) {
  background-image: url('/images/lottery_get_select.png');
}

.lot-btn--pay {
  width: 129px;
  height: 34px;
  background-image: url('/images/lottery_pay.png');
}
.lot-btn--pay:hover:not(:disabled) {
  background-image: url('/images/lottery_pay_over.png');
}
.lot-btn--pay:active:not(:disabled) {
  background-image: url('/images/lottery_pay_select.png');
}
</style>
