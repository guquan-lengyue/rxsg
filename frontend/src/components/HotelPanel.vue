<script setup lang="ts">
import { computed } from 'vue'

import { heroFace } from '@/assets/img'
import { zhCN } from '@/lang/zh-CN'
import type { HotelInfo, Recruit } from '@/types'

const props = defineProps<{
  info: HotelInfo | null
  loading: boolean
  error: string
  busy: boolean
  gold: number
}>()
const emit = defineEmits<{
  (e: 'recruit', id: number): void
  (e: 'reset'): void
  (e: 'close'): void
}>()

const recruits = computed(() => props.info?.recruits ?? [])

// 原版 sex：0=女 1=男（HotelFunc.php:180）；heroFace 的 girl 判定为 sex===2，此处换算。
function faceOf(r: Recruit): string {
  return heroFace(r.sex === 0 ? 2 : 1, r.face)
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal u-modal">
      <header class="u-title">
        <strong>{{ zhCN.hotel.title }}</strong>
        <span v-if="info" class="sub dim small">
          {{ info.hotel_name }} Lv{{ info.hotel_level }} · {{ zhCN.hotel.officePos }}
          {{ info.office_pos }} · {{ zhCN.hotel.gold }} {{ gold }}
        </span>
        <button class="u-close" title="关闭" @click="emit('close')"></button>
      </header>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>
      <p v-else-if="info?.tip" class="hint warn">{{ info.tip }}</p>
      <p v-else-if="!recruits.length" class="hint">{{ zhCN.hotel.empty }}</p>

      <ul v-else class="list">
        <li v-for="r in recruits" :key="r.id" class="item">
          <div class="row">
            <img class="face" :src="faceOf(r)" :alt="r.name" />
            <div class="head-main">
              <div class="row">
                <strong>{{ r.name }}</strong>
                <span class="state">
                  Lv{{ r.level }}
                  <span v-if="r.is_act_hero" class="act">·{{ zhCN.hotel.actHero }}</span>
                </span>
              </div>
              <div class="row dim small">
                <span>
                  {{ zhCN.hotel.command }}{{ r.command_base + r.command_add }}
                  {{ zhCN.hotel.bravery }}{{ r.bravery_base + r.bravery_add }}
                  {{ zhCN.hotel.wisdom }}{{ r.wisdom_base + r.wisdom_add }}
                  {{ zhCN.hotel.affairs }}{{ r.affairs_base + r.affairs_add }}
                </span>
                <span>{{ zhCN.hero.loyalty }} {{ r.loyalty }}</span>
              </div>
            </div>
            <div class="btns">
              <span class="price">{{ r.gold_need }}</span>
              <button
                class="primary"
                :disabled="busy || (info && info.office_pos <= 0) || r.gold_need > gold"
                @click="emit('recruit', r.id)"
              >
                {{ zhCN.hotel.recruit }}
              </button>
            </div>
          </div>
        </li>
      </ul>

      <footer class="foot">
        <button class="ghost" :disabled="busy || loading" @click="emit('reset')">
          {{ zhCN.hotel.useZhaoXinLin }}
        </button>
        <span class="dim small">{{ zhCN.hotel.resetHint }}</span>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.55);
}

.modal {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  max-width: 620px;
  max-height: 86vh;
  padding: 16px;
  overflow-y: auto;
}

.sub {
  flex: 1;
  text-align: right;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.face {
  flex: 0 0 auto;
  width: 48px;
  height: 60px;
  object-fit: cover;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.head-main {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.dim {
  color: var(--text-dim);
}

.small {
  font-size: 12px;
}

.state {
  color: var(--accent);
  font-size: 13px;
}

.act {
  color: var(--danger);
}

.price {
  color: var(--accent);
  font-size: 13px;
}

.btns {
  display: flex;
  align-items: center;
  gap: 8px;
}

.btns button {
  padding: 5px 14px;
  border-radius: 4px;
  cursor: pointer;
}

.primary {
  color: #10202e;
  background: var(--accent);
  border: 1px solid var(--accent);
}

.ghost {
  color: var(--text);
  background: transparent;
  border: 1px solid var(--panel-border);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.foot {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 4px;
  border-top: 1px solid var(--panel-border);
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}

.hint.warn {
  color: var(--accent);
}
</style>
