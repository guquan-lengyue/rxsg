<script setup lang="ts">
// 内政面板：税率设置 / 征收 / 安抚百姓 / 产出比例。
// 对齐后端 city.ChangeTax、city.LevyResource、city.PacifyPeople、city.GetCityProduct 与 economy.SetCityProductRate。
// 面板只负责展示与发事件（props in / events out），写操作由父级 runCivil 执行并回灌 notice/error。
import { computed, reactive, ref, watch } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { resIcon } from '@/assets/img'
import type { CityProduct, ProductRatePayload } from '@/api/city'
import type { CityResource } from '@/types'

const props = defineProps<{
  product: CityProduct | null
  resource: CityResource | null
  loading: boolean
  error: string
  notice: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'set-tax', tax: number): void
  (e: 'levy', resid: number): void
  (e: 'pacify', action: number): void
  (e: 'set-product-rate', payload: ProductRatePayload): void
  (e: 'close'): void
}>()

type Tab = 'tax' | 'levy' | 'pacify' | 'product'
const tab = ref<Tab>('tax')

// —— 税率（对齐 changeTax：0~100，落库后按 100-tax-complaint 重算民心稳定）——
const taxInput = ref(0)
watch(
  () => props.resource?.tax,
  (v) => {
    taxInput.value = v ?? 0
  },
  { immediate: true },
)

function submitTax(): void {
  emit('set-tax', taxInput.value)
}

// —— 征收（对齐 levyResource resid：0金/1粮/2木/3石/4铁；无人口项）——
const levyOptions: { resid: number; key: string; name: string }[] = [
  { resid: 1, key: 'food', name: '粮食' },
  { resid: 2, key: 'wood', name: '木材' },
  { resid: 3, key: 'rock', name: '石料' },
  { resid: 4, key: 'iron', name: '铁锭' },
  { resid: 0, key: 'gold', name: '黄金' },
]

// —— 安抚百姓（对齐 pacifyPeople action：0赈灾/1祈福/2祭天/3增丁）——
const pacifyOptions: { action: number; name: string; effect: string }[] = [
  { action: 0, name: '赈灾', effect: '消耗粮食＝人口上限×速率，民心+5、民怨-15' },
  { action: 1, name: '祈福', effect: '消耗黄金＝人口上限×速率，民心+25、民怨-5' },
  { action: 2, name: '祭天', effect: '消耗粮食＝人口上限×速率、黄金＝人口上限×10%×速率，推迟天灾并有几率触发天赐' },
  { action: 3, name: '增丁', effect: '消耗粮食＝人口上限×5×速率，人口增加至多为人口上限的5%' },
]

// —— 产出比例（对齐 getCityProduct / setCityProductRate 的 food_rate 等四项百分比）——
const rates = reactive({ food: 0, wood: 0, rock: 0, iron: 0 })
watch(
  () => props.product,
  (p) => {
    rates.food = p?.foodRate ?? 0
    rates.wood = p?.woodRate ?? 0
    rates.rock = p?.rockRate ?? 0
    rates.iron = p?.ironRate ?? 0
  },
  { immediate: true },
)

const rateTotal = computed(() => rates.food + rates.wood + rates.rock + rates.iron)
const rateInvalid = computed(
  () =>
    rates.food < 0 ||
    rates.wood < 0 ||
    rates.rock < 0 ||
    rates.iron < 0 ||
    rates.food > 100 ||
    rates.wood > 100 ||
    rates.rock > 100 ||
    rates.iron > 100,
)

function submitRate(): void {
  emit('set-product-rate', {
    food: rates.food,
    wood: rates.wood,
    rock: rates.rock,
    iron: rates.iron,
  })
}
</script>

<template>
  <SkinDialog title="内政" @close="emit('close')">
    <div class="u-tabs">
      <button class="u-tab" :class="{ 'is-active': tab === 'tax' }" type="button" @click="tab = 'tax'">
        税率
      </button>
      <button class="u-tab" :class="{ 'is-active': tab === 'levy' }" type="button" @click="tab = 'levy'">
        征收
      </button>
      <button class="u-tab" :class="{ 'is-active': tab === 'pacify' }" type="button" @click="tab = 'pacify'">
        安抚
      </button>
      <button
        class="u-tab"
        :class="{ 'is-active': tab === 'product' }"
        type="button"
        @click="tab = 'product'"
      >
        产出比例
      </button>
    </div>

    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>
    <p v-else-if="notice" class="u-hint is-notice">{{ notice }}</p>

    <!-- 税率 -->
    <template v-if="tab === 'tax'">
      <template v-if="resource">
        <div class="u-row">
          <span class="label">当前税率</span>
          <span>{{ resource.tax }}%</span>
          <span class="label">民心</span>
          <span>{{ resource.morale }}</span>
          <span class="label">民怨</span>
          <span>{{ resource.complaint }}</span>
        </div>
      </template>
      <div class="u-row">
        <span class="label">设置税率</span>
        <input v-model.number="taxInput" type="number" min="0" max="100" class="num" />
        <span class="u-hint">%（0~100）</span>
        <button
          class="u-btn u-btn--confirm"
          type="button"
          :disabled="busy || taxInput < 0 || taxInput > 100"
          @click="submitTax"
        >
          确定
        </button>
      </div>
      <p class="u-hint">税率越高黄金收入越多，但民心稳定＝100−税率−民怨（下限 0）。</p>
    </template>

    <!-- 征收 -->
    <template v-else-if="tab === 'levy'">
      <p class="u-hint">按当前人口征收资源，征收后民心-20，冷却 15 分钟；民心≤20 时不可征收。</p>
      <ul class="grid">
        <li v-for="o in levyOptions" :key="o.resid" class="cell">
          <img class="res" :src="resIcon(o.key)" :alt="o.name" />
          <span>{{ o.name }}</span>
          <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="emit('levy', o.resid)">
            征收
          </button>
        </li>
      </ul>
    </template>

    <!-- 安抚 -->
    <template v-else-if="tab === 'pacify'">
      <p class="u-hint">安抚百姓冷却 15 分钟；本城处于战乱状态时不可安抚。</p>
      <ul class="list">
        <li v-for="o in pacifyOptions" :key="o.action" class="item">
          <div class="u-row between">
            <strong>{{ o.name }}</strong>
            <button
              class="u-btn u-btn--green"
              type="button"
              :disabled="busy"
              @click="emit('pacify', o.action)"
            >
              安抚
            </button>
          </div>
          <p class="u-hint">{{ o.effect }}</p>
        </li>
      </ul>
    </template>

    <!-- 产出比例 -->
    <template v-else>
      <p class="u-hint">按百分比分配四种基础产出（劳力 100% 分配，合计建议为 100%）。</p>
      <div class="u-row">
        <span class="label rate-label"><img class="res" :src="resIcon('food')" alt="农田" />农田</span>
        <input v-model.number="rates.food" type="number" min="0" max="100" class="num" />
        <span class="u-hint">%　劳力 {{ product?.foodPeople ?? 0 }}</span>
      </div>
      <div class="u-row">
        <span class="label rate-label"><img class="res" :src="resIcon('wood')" alt="伐木" />伐木</span>
        <input v-model.number="rates.wood" type="number" min="0" max="100" class="num" />
        <span class="u-hint">%　劳力 {{ product?.woodPeople ?? 0 }}</span>
      </div>
      <div class="u-row">
        <span class="label rate-label"><img class="res" :src="resIcon('rock')" alt="采石" />采石</span>
        <input v-model.number="rates.rock" type="number" min="0" max="100" class="num" />
        <span class="u-hint">%　劳力 {{ product?.rockPeople ?? 0 }}</span>
      </div>
      <div class="u-row">
        <span class="label rate-label"><img class="res" :src="resIcon('iron')" alt="铁矿" />铁矿</span>
        <input v-model.number="rates.iron" type="number" min="0" max="100" class="num" />
        <span class="u-hint">%　劳力 {{ product?.ironPeople ?? 0 }}</span>
      </div>
      <div class="u-row">
        <span class="label">合计</span>
        <span :class="{ warn: rateTotal !== 100 }">{{ rateTotal }}%</span>
        <span class="u-hint">军粮消耗 {{ product?.foodArmyUse ?? 0 }}，城守加成 {{ product?.chiefAdd ?? 0 }}</span>
      </div>
      <div class="u-row">
        <button
          class="u-btn u-btn--confirm"
          type="button"
          :disabled="busy || rateInvalid"
          @click="submitRate"
        >
          保存产出比例
        </button>
      </div>
    </template>

    <template #footer>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.u-tabs {
  margin-bottom: 8px;
}

.u-hint {
  margin: 4px 0;
}

.u-hint.is-notice {
  color: var(--accent);
}

.u-row {
  margin: 6px 0;
}

.u-row.between {
  justify-content: space-between;
}

.num {
  width: 72px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.rate-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: 64px;
}

.res {
  width: 20px;
  height: 20px;
  object-fit: contain;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.cell {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  font-size: 13px;
}

.cell .u-btn {
  margin-left: auto;
  min-width: 56px;
  padding: 0 10px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.warn {
  color: var(--danger);
}
</style>
