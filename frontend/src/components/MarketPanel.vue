<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { formatLeft } from '@/render/cityGrid'
import { img, resIcon } from '@/assets/img'
import type { CityResource } from '@/types'
import type {
  AutoTransPayload,
  AutoTransRow,
  BuyListParams,
  BuyListResult,
  CopperGood,
  MarketInfo,
  MerchantInfo,
  MerchantTradePayload,
  PackItem,
  SellDataResult,
  SellGoodsPayload,
  SellUserPayload,
  ShopBuyPayload,
  ShopGood,
  StoreRatePayload,
  UserGood,
} from '@/api/economy'
import type { ShopView, StoreView, WorkshopView } from '@/stores/economy'

const props = defineProps<{
  cid: number
  resources: CityResource | null
  market: MarketInfo | null
  merchant: MerchantInfo | null
  buyList: BuyListResult | null
  sellData: SellDataResult | null
  autoTrans: AutoTransRow[]
  autoTransHas: boolean
  store: StoreView | null
  workshop: WorkshopView | null
  shop: ShopView | null
  goods: UserGood[]
  gold: number
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'load-merchant'): void
  (e: 'buy-merchant', p: MerchantTradePayload): void
  (e: 'sell-merchant', p: MerchantTradePayload): void
  (e: 'sell-user', p: SellUserPayload): void
  (e: 'buy-user', id: number): void
  (e: 'cancel-sell', id: number): void
  (e: 'accelerate', id: number): void
  (e: 'load-buylist', p: BuyListParams): void
  (e: 'add-autotrans', p: AutoTransPayload): void
  (e: 'remove-autotrans', id: number): void
  (e: 'cancel-autotrans', id: number): void
  (e: 'store-rate', p: StoreRatePayload): void
  (e: 'store-pack', items: PackItem[]): void
  (e: 'workshop-refresh', isNeedWuzhu: number): void
  (e: 'workshop-buy', gid: number): void
  (e: 'shop-buy', p: ShopBuyPayload, beforeUse: boolean): void
  (e: 'shop-exchange', code: string): void
  (e: 'shop-armor-attr', aid: number): void
  (e: 'shop-hero-attr', hid: number): void
  (e: 'sell-goods', p: SellGoodsPayload): void
  (e: 'close'): void
}>()

type Tab = 'market' | 'store' | 'workshop' | 'shop'
const tab = ref<Tab>('market')
const localError = ref('')

const TABS: { key: Tab; label: string }[] = [
  { key: 'market', label: '市场' },
  { key: 'store', label: '仓库' },
  { key: 'workshop', label: '工匠作坊' },
  { key: 'shop', label: '商城' },
]

const RES_TYPES = [
  { value: 0, label: '粮食', key: 'food' },
  { value: 1, label: '木材', key: 'wood' },
  { value: 2, label: '石料', key: 'rock' },
  { value: 3, label: '铁锭', key: 'iron' },
] as const

const resCounts = computed<Record<string, number>>(() => {
  const r = props.resources
  return {
    food: r?.food ?? 0,
    wood: r?.wood ?? 0,
    rock: r?.rock ?? 0,
    iron: r?.iron ?? 0,
    gold: r?.gold ?? props.gold,
  }
})

// —— 商人交易 ——
const merchantForm = reactive({ food: 0, wood: 0, rock: 0, iron: 0, times: 1, paytype: 0 })

function submitMerchant(kind: 'buy' | 'sell'): void {
  localError.value = ''
  if (merchantForm.times < 1) {
    localError.value = '请输入正常的交易倍数。'
    return
  }
  const p: MerchantTradePayload = { ...merchantForm }
  if (kind === 'buy') {
    emit('buy-merchant', p)
  } else {
    emit('sell-merchant', p)
  }
}

// —— 玩家挂单（出售 / 购买）——
const sellForm = reactive({ resType: 0, count: 0, gold: 0, hour: 24, unionOnly: 0 })

function submitSellUser(): void {
  localError.value = ''
  if (sellForm.count <= 0) {
    localError.value = '出售数量不正确。'
    return
  }
  emit('sell-user', { ...sellForm })
}

const listForm = reactive({ page: 0, filter: 0, unionOnly: 0, sellName: '' })

function loadBuyList(): void {
  emit('load-buylist', { ...listForm })
}

function gotoPage(p: number): void {
  listForm.page = Math.max(0, p)
  loadBuyList()
}

const myTrades = computed(() => props.market?.trades ?? [])

function restypeText(t: number): string {
  return RES_TYPES.find((r) => r.value === t)?.label ?? (t === 4 ? '黄金' : `资源${t}`)
}

function tradeStateText(s: number): string {
  if (s === 0) return '挂单中'
  if (s === 1) return '运输中'
  if (s === 2) return '自动运输'
  return `状态${s}`
}

// —— 自动运输 ——
const autoForm = reactive({
  fromcid: props.cid,
  tocid: 0,
  resType: 0,
  count: 0,
  transType: 0,
  startTime: '',
})

function submitAutoTrans(): void {
  localError.value = ''
  if (!autoForm.tocid) {
    localError.value = '请填写目标城池 ID。'
    return
  }
  const ms = autoForm.startTime ? new Date(autoForm.startTime).getTime() : Date.now() + 60000
  emit('add-autotrans', {
    fromcid: autoForm.fromcid,
    tocid: autoForm.tocid,
    resType: autoForm.resType,
    count: autoForm.count,
    transType: autoForm.transType,
    startTime: ms,
  })
}

const cancelAutoId = ref(0)

// —— 仓库 ——
const rateForm = reactive({ food: 0, wood: 0, rock: 0, iron: 0 })
watch(
  () => props.store,
  (s) => {
    if (s) {
      rateForm.food = s.foodRate
      rateForm.wood = s.woodRate
      rateForm.rock = s.rockRate
      rateForm.iron = s.ironRate
    }
  },
  { immediate: true },
)

function submitStoreRate(): void {
  emit('store-rate', { ...rateForm })
}

const PACK_TYPES: { value: string; label: string }[] = [
  { value: 'food1', label: '辎重包(粮)' },
  { value: 'food2', label: '辎重箱(粮)' },
  { value: 'wood1', label: '辎重包(木)' },
  { value: 'wood2', label: '辎重箱(木)' },
  { value: 'rock1', label: '辎重包(石)' },
  { value: 'rock2', label: '辎重箱(石)' },
  { value: 'iron1', label: '辎重包(铁)' },
  { value: 'iron2', label: '辎重箱(铁)' },
  { value: 'gold1', label: '金条' },
  { value: 'gold2', label: '金砖' },
]

const packForm = reactive({ type: 'wood1', pack_count: 0, res_count: 0, copper: 0 })
const packItems = ref<PackItem[]>([])

function addPackItem(): void {
  if (packForm.pack_count <= 0 || packForm.res_count <= 0) {
    localError.value = '请填写打包数量与资源数量。'
    return
  }
  packItems.value = [...packItems.value, { ...packForm }]
  packForm.pack_count = 0
  packForm.res_count = 0
  packForm.copper = 0
}

function removePackItem(index: number): void {
  packItems.value = packItems.value.filter((_, i) => i !== index)
}

function submitPack(): void {
  if (!packItems.value.length) {
    localError.value = '请先添加打包项。'
    return
  }
  emit('store-pack', [...packItems.value])
}

// —— 作坊 ——
const useWuzhu = ref(false)

function submitWorkshopRefresh(): void {
  emit('workshop-refresh', useWuzhu.value ? 1 : 0)
}

// —— 商城 ——
const buyCnt = reactive<Record<number, number>>({})
const paytype = ref(0)
const exchangeCode = ref('')
const armorAid = ref(0)
const heroHid = ref(0)

function nameOfGid(gid: number): string {
  return props.goods.find((g) => g.gid === gid)?.name || `道具 ${gid}`
}

function emitShopBuy(g: ShopGood, beforeUse: boolean): void {
  emit(
    'shop-buy',
    { id: g.id, cnt: buyCnt[g.id] || 1, paytype: paytype.value, gid: g.gid },
    beforeUse,
  )
}

function submitExchange(): void {
  emit('shop-exchange', exchangeCode.value.trim())
}

const sellGoodsForm = reactive({ gid: 0, count: 1 })

function submitSellGoods(): void {
  if (!sellGoodsForm.gid || sellGoodsForm.count <= 0) {
    localError.value = '请选择道具并填写数量。'
    return
  }
  emit('sell-goods', { cid: props.cid, gid: sellGoodsForm.gid, count: sellGoodsForm.count })
}

// 道具图标缺失回退文本。
const failedIcons = ref<Set<number>>(new Set())
function itemIcon(gid: number): string {
  return img(`item_${gid}.png`)
}
function markIconFailed(gid: number): void {
  const next = new Set(failedIcons.value)
  next.add(gid)
  failedIcons.value = next
}
function iconOk(gid: number): boolean {
  return !failedIcons.value.has(gid)
}

function copperGoodName(g: CopperGood): string {
  return g.name || nameOfGid(g.gid)
}
</script>

<template>
  <SkinDialog title="经济" wide @close="emit('close')">
    <div class="u-tabs">
      <button
        v-for="t in TABS"
        :key="t.key"
        class="u-tab"
        :class="{ 'is-active': tab === t.key }"
        type="button"
        @click="tab = t.key"
      >
        {{ t.label }}
      </button>
    </div>

    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>
    <p v-if="localError" class="u-hint is-error">{{ localError }}</p>

    <!-- 市场 -->
    <template v-if="tab === 'market'">
      <section class="block">
        <h4 class="block-title">城库资源</h4>
        <div class="res-row">
          <span v-for="r in RES_TYPES" :key="r.key" class="res-cell">
            <img class="res-icon" :src="resIcon(r.key)" :alt="r.label" />
            {{ r.label }} {{ resCounts[r.key] }}
          </span>
          <span class="res-cell">
            <img class="res-icon" :src="resIcon('gold')" alt="黄金" /> 黄金 {{ resCounts.gold }}
          </span>
        </div>
      </section>

      <section class="block">
        <h4 class="block-title">
          商人交易
          <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="emit('load-merchant')">
            刷新商人
          </button>
        </h4>
        <div v-if="merchant" class="u-row">
          <span class="label">库存</span>
          <span>
            粮 {{ merchant.food }} · 木 {{ merchant.wood }} · 石 {{ merchant.rock }} · 铁
            {{ merchant.iron }} · 金 {{ merchant.gold }}
          </span>
        </div>
        <div v-if="merchant" class="u-row">
          <span class="label">收购价</span>
          <span class="dim">
            粮 {{ merchant.food_buy_price }} · 木 {{ merchant.wood_buy_price }} · 石
            {{ merchant.rock_buy_price }} · 铁 {{ merchant.iron_buy_price }}
          </span>
        </div>
        <div v-if="sellData" class="u-row">
          <span class="label">官价</span>
          <span class="dim">
            粮 {{ sellData[2] }} · 木 {{ sellData[3] }} · 石 {{ sellData[4] }} · 铁 {{ sellData[5] }}
            （商队占用 {{ sellData[0] }}/{{ sellData[1] }}）
          </span>
        </div>
        <div class="grid4">
          <label v-for="r in RES_TYPES" :key="r.key" class="field">
            <span>{{ r.label }}</span>
            <input v-model.number="merchantForm[r.key]" type="number" min="0" class="num" />
          </label>
        </div>
        <div class="u-row">
          <label class="field"><span>倍数</span>
            <input v-model.number="merchantForm.times" type="number" min="1" class="num" />
          </label>
          <label class="field"><span>支付</span>
            <select v-model.number="merchantForm.paytype">
              <option :value="0">元宝</option>
              <option :value="1">礼金</option>
            </select>
          </label>
        </div>
        <div class="actions">
          <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="submitMerchant('buy')">
            购买
          </button>
          <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="submitMerchant('sell')">
            出售
          </button>
        </div>
      </section>

      <section class="block">
        <h4 class="block-title">挂单出售（卖给玩家）</h4>
        <div class="u-row">
          <label class="field"><span>资源</span>
            <select v-model.number="sellForm.resType">
              <option v-for="r in RES_TYPES" :key="r.value" :value="r.value">{{ r.label }}</option>
            </select>
          </label>
          <label class="field"><span>数量</span>
            <input v-model.number="sellForm.count" type="number" min="0" class="num" />
          </label>
          <label class="field"><span>总金</span>
            <input v-model.number="sellForm.gold" type="number" min="0" class="num" />
          </label>
          <label class="field"><span>时限(时)</span>
            <input v-model.number="sellForm.hour" type="number" min="1" class="num" />
          </label>
          <label class="field check"><input v-model="sellForm.unionOnly" type="checkbox" :true-value="1" :false-value="0" /> 仅联盟</label>
        </div>
        <div class="actions">
          <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="submitSellUser">
            挂单
          </button>
        </div>
      </section>

      <section class="block">
        <h4 class="block-title">本城交易挂单</h4>
        <p v-if="!myTrades.length" class="u-hint">暂无挂单</p>
        <ul v-else class="list">
          <li v-for="t in myTrades" :key="t.id" class="item">
            <div class="u-row">
              <span class="label">#{{ t.id }}</span>
              <span>{{ restypeText(t.restype) }} ×{{ t.count }} · 单价 {{ t.price }} · 总金 {{ t.gold }}</span>
              <span class="dim">{{ tradeStateText(t.state) }}</span>
            </div>
            <div class="actions">
              <button class="u-btn u-btn--cancel" type="button" :disabled="busy || t.state !== 0" @click="emit('cancel-sell', t.id)">
                取消
              </button>
              <button class="u-btn u-btn--yellow" type="button" :disabled="busy || (t.state !== 1 && t.state !== 2)" @click="emit('accelerate', t.id)">
                加速
              </button>
            </div>
          </li>
        </ul>
      </section>

      <section class="block">
        <h4 class="block-title">购买玩家挂单</h4>
        <div class="u-row">
          <label class="field"><span>资源</span>
            <select v-model.number="listForm.filter">
              <option :value="0">全部</option>
              <option v-for="r in RES_TYPES" :key="r.value" :value="r.value + 1">{{ r.label }}</option>
            </select>
          </label>
          <label class="field"><span>卖家</span>
            <input v-model="listForm.sellName" type="text" class="num wide" placeholder="城主名" />
          </label>
          <label class="field check"><input v-model="listForm.unionOnly" type="checkbox" :true-value="1" :false-value="0" /> 仅联盟</label>
          <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="loadBuyList">查询</button>
        </div>
        <template v-if="buyList">
          <div class="u-row">
            <span class="label">第 {{ buyList[3] + 1 }} / {{ Math.max(1, buyList[2]) }} 页</span>
            <span class="dim">商队占用 {{ buyList[0] }}/{{ buyList[1] }}</span>
            <button class="u-btn u-btn--cancel" type="button" :disabled="buyList[3] <= 0" @click="gotoPage(buyList[3] - 1)">上一页</button>
            <button class="u-btn u-btn--cancel" type="button" :disabled="buyList[3] + 1 >= buyList[2]" @click="gotoPage(buyList[3] + 1)">下一页</button>
          </div>
          <ul v-if="buyList[4].length" class="list">
            <li v-for="t in buyList[4]" :key="t.id" class="item">
              <div class="u-row">
                <strong>{{ t.sellername || '—' }}</strong>
                <span>{{ restypeText(t.restype) }} ×{{ t.count }} · 总金 {{ t.gold }}</span>
                <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('buy-user', t.id)">购买</button>
              </div>
            </li>
          </ul>
          <p v-else class="u-hint">没有符合条件的挂单</p>
        </template>
      </section>

      <section class="block">
        <h4 class="block-title">自动运输 {{ autoTransHas ? '（已开通商队契约）' : '（未开通）' }}</h4>
        <div class="u-row">
          <label class="field"><span>出发城</span>
            <input v-model.number="autoForm.fromcid" type="number" min="1" class="num" />
          </label>
          <label class="field"><span>目标城</span>
            <input v-model.number="autoForm.tocid" type="number" min="1" class="num" />
          </label>
          <label class="field"><span>资源</span>
            <select v-model.number="autoForm.resType">
              <option v-for="r in RES_TYPES" :key="r.value" :value="r.value">{{ r.label }}</option>
            </select>
          </label>
          <label class="field"><span>数量</span>
            <input v-model.number="autoForm.count" type="number" min="0" class="num" />
          </label>
          <label class="field"><span>类型</span>
            <select v-model.number="autoForm.transType">
              <option :value="0">单次</option>
              <option :value="1">循环</option>
            </select>
          </label>
          <label class="field"><span>开始时间</span>
            <input v-model="autoForm.startTime" type="datetime-local" />
          </label>
          <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="submitAutoTrans">添加</button>
        </div>
        <p v-if="!autoTrans.length" class="u-hint">暂无自动运输任务</p>
        <ul v-else class="list">
          <li v-for="a in autoTrans" :key="a.id" class="item">
            <div class="u-row">
              <span class="label">#{{ a.id }}</span>
              <span>{{ a.fromcity }} → {{ a.tocity }} · {{ restypeText(a.res_type) }} ×{{ a.count }}</span>
              <button class="u-btn u-btn--cancel" type="button" :disabled="busy" @click="emit('remove-autotrans', a.id)">删除</button>
            </div>
          </li>
        </ul>
        <div class="u-row">
          <label class="field"><span>取消挂单 #</span>
            <input v-model.number="cancelAutoId" type="number" min="0" class="num" />
          </label>
          <button class="u-btn u-btn--cancel" type="button" :disabled="busy" @click="emit('cancel-autotrans', cancelAutoId)">取消运输挂单</button>
        </div>
      </section>
    </template>

    <!-- 仓库 -->
    <template v-else-if="tab === 'store'">
      <template v-if="store">
        <section class="block">
          <h4 class="block-title">仓库概况</h4>
          <div class="u-row"><span class="label">仓储科技</span><span>系数 {{ store.storageTech.toFixed(2) }}</span></div>
          <div class="u-row"><span class="label">仓库数</span><span>{{ store.storeCount }} · 容量 {{ store.storeMax }}</span></div>
          <div class="u-row"><span class="label">铜钱</span><span>{{ store.copper }}</span></div>
          <div class="res-row">
            <span class="res-cell"><img class="res-icon" :src="resIcon('food')" alt="粮" /> 粮产 {{ store.foodBase }}</span>
            <span class="res-cell"><img class="res-icon" :src="resIcon('wood')" alt="木" /> 木产 {{ store.woodBase }}</span>
            <span class="res-cell"><img class="res-icon" :src="resIcon('rock')" alt="石" /> 石产 {{ store.rockBase }}</span>
            <span class="res-cell"><img class="res-icon" :src="resIcon('iron')" alt="铁" /> 铁产 {{ store.ironBase }}</span>
          </div>
        </section>

        <section class="block">
          <h4 class="block-title">存放比例（合计 ≤ 100）</h4>
          <div class="grid4">
            <label class="field"><span>粮食</span><input v-model.number="rateForm.food" type="number" min="0" class="num" /></label>
            <label class="field"><span>木材</span><input v-model.number="rateForm.wood" type="number" min="0" class="num" /></label>
            <label class="field"><span>石料</span><input v-model.number="rateForm.rock" type="number" min="0" class="num" /></label>
            <label class="field"><span>铁锭</span><input v-model.number="rateForm.iron" type="number" min="0" class="num" /></label>
          </div>
          <div class="actions">
            <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="submitStoreRate">保存比例</button>
          </div>
        </section>

        <section class="block">
          <h4 class="block-title">打包（资源 → 辎重）</h4>
          <div class="u-row">
            <label class="field"><span>类型</span>
              <select v-model="packForm.type">
                <option v-for="p in PACK_TYPES" :key="p.value" :value="p.value">{{ p.label }}</option>
              </select>
            </label>
            <label class="field"><span>打包数</span><input v-model.number="packForm.pack_count" type="number" min="0" class="num" /></label>
            <label class="field"><span>资源数</span><input v-model.number="packForm.res_count" type="number" min="0" class="num" /></label>
            <label class="field"><span>铜钱</span><input v-model.number="packForm.copper" type="number" min="0" class="num" /></label>
            <button class="u-btn u-btn--gold" type="button" @click="addPackItem">加入</button>
          </div>
          <ul v-if="packItems.length" class="list">
            <li v-for="(it, i) in packItems" :key="i" class="item">
              <div class="u-row">
                <span>{{ it.type }} · 打包 {{ it.pack_count }} · 资源 {{ it.res_count }} · 铜钱 {{ it.copper }}</span>
                <button class="u-btn u-btn--cancel" type="button" @click="removePackItem(i)">移除</button>
              </div>
            </li>
          </ul>
          <div class="actions">
            <button class="u-btn u-btn--green" type="button" :disabled="busy || !packItems.length" @click="submitPack">打包</button>
          </div>
        </section>
      </template>
      <p v-else class="u-hint">暂无仓库信息</p>
    </template>

    <!-- 工匠作坊 -->
    <template v-else-if="tab === 'workshop'">
      <template v-if="workshop">
        <div class="u-row">
          <span class="label">免费刷新冷却</span>
          <span>{{ workshop.leaveTime > 0 ? formatLeft(workshop.leaveTime) : '可刷新' }}</span>
          <span class="dim">剩余货位 {{ workshop.goodCount }}</span>
          <label class="field check"><input v-model="useWuzhu" type="checkbox" /> 用五铢钱刷新</label>
          <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="submitWorkshopRefresh">刷新</button>
        </div>
        <p v-if="!workshop.slots.length" class="u-hint">货架空空，请刷新。</p>
        <ul v-else class="list">
          <li v-for="s in workshop.slots" :key="s.gid" class="item">
            <div class="u-row">
              <img v-if="iconOk(s.gid)" class="item-icon" :src="itemIcon(s.gid)" :alt="s.name" @error="markIconFailed(s.gid)" />
              <strong>{{ s.name || nameOfGid(s.gid) }}</strong>
              <span class="dim">GID {{ s.gid }}</span>
              <span class="price">礼金 {{ s.price }}</span>
              <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('workshop-buy', s.gid)">购买</button>
            </div>
          </li>
        </ul>
      </template>
      <p v-else class="u-hint">暂无作坊信息</p>
    </template>

    <!-- 商城 -->
    <template v-else>
      <template v-if="shop">
        <div class="u-row">
          <span class="label">五铢钱</span><span>{{ shop.wuzhu }}</span>
          <span class="label">积分</span><span>{{ shop.point }}</span>
          <label class="field"><span>支付</span>
            <select v-model.number="paytype">
              <option :value="0">元宝</option>
              <option :value="1">礼金</option>
            </select>
          </label>
        </div>

        <section class="block">
          <h4 class="block-title">常规商品</h4>
          <ul v-if="shop.normal.length" class="list">
            <li v-for="g in shop.normal" :key="g.id" class="item">
              <div class="u-row">
                <img v-if="iconOk(g.gid)" class="item-icon" :src="itemIcon(g.gid)" :alt="nameOfGid(g.gid)" @error="markIconFailed(g.gid)" />
                <strong>{{ nameOfGid(g.gid) }}</strong>
                <span class="dim">价 {{ g.price }} ×{{ g.pack }}</span>
                <input v-model.number="buyCnt[g.id]" type="number" min="1" class="num" />
                <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emitShopBuy(g, false)">购买</button>
                <button class="u-btn u-btn--yellow" type="button" :disabled="busy" @click="emitShopBuy(g, true)">购买并用</button>
              </div>
            </li>
          </ul>
          <p v-else class="u-hint">暂无常规商品</p>
        </section>

        <section class="block">
          <h4 class="block-title">推荐商品</h4>
          <ul v-if="shop.recommend.length" class="list">
            <li v-for="g in shop.recommend" :key="g.id" class="item">
              <div class="u-row">
                <img v-if="iconOk(g.gid)" class="item-icon" :src="itemIcon(g.gid)" :alt="nameOfGid(g.gid)" @error="markIconFailed(g.gid)" />
                <strong>{{ nameOfGid(g.gid) }}</strong>
                <span class="dim">价 {{ g.price }} ×{{ g.pack }}</span>
                <input v-model.number="buyCnt[g.id]" type="number" min="1" class="num" />
                <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emitShopBuy(g, false)">购买</button>
              </div>
            </li>
          </ul>
          <p v-else class="u-hint">暂无推荐商品</p>
        </section>

        <section class="block">
          <h4 class="block-title">五铢钱 / 积分商品</h4>
          <ul v-if="shop.wuzhuGoods.length || shop.pointGoods.length" class="list">
            <li v-for="g in shop.wuzhuGoods" :key="'w' + g.gid" class="item">
              <div class="u-row">
                <img v-if="iconOk(g.gid)" class="item-icon" :src="itemIcon(g.gid)" :alt="copperGoodName(g)" @error="markIconFailed(g.gid)" />
                <strong>{{ copperGoodName(g) }}</strong>
                <span class="dim">五铢钱 {{ g.price }}</span>
              </div>
            </li>
            <li v-for="g in shop.pointGoods" :key="'p' + g.gid" class="item">
              <div class="u-row">
                <img v-if="iconOk(g.gid)" class="item-icon" :src="itemIcon(g.gid)" :alt="copperGoodName(g)" @error="markIconFailed(g.gid)" />
                <strong>{{ copperGoodName(g) }}</strong>
                <span class="dim">积分 {{ g.price }}</span>
              </div>
            </li>
          </ul>
          <p v-else class="u-hint">暂无五铢钱/积分商品</p>
        </section>

        <section class="block">
          <h4 class="block-title">礼券兑换</h4>
          <div class="u-row">
            <input v-model="exchangeCode" type="text" class="num wide" placeholder="礼券码" />
            <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="submitExchange">兑换</button>
          </div>
        </section>

        <section class="block">
          <h4 class="block-title">属性提示</h4>
          <div class="u-row">
            <label class="field"><span>装备 ID</span><input v-model.number="armorAid" type="number" min="0" class="num" /></label>
            <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="emit('shop-armor-attr', armorAid)">查装备</button>
            <label class="field"><span>武将 ID</span><input v-model.number="heroHid" type="number" min="0" class="num" /></label>
            <button class="u-btn u-btn--gold" type="button" :disabled="busy" @click="emit('shop-hero-attr', heroHid)">查武将</button>
          </div>
        </section>

        <section class="block">
          <h4 class="block-title">出售宝物（市场 5 级且爵位≥公士）</h4>
          <div class="u-row">
            <label class="field"><span>道具</span>
              <select v-model.number="sellGoodsForm.gid">
                <option :value="0">请选择</option>
                <option v-for="g in goods" :key="g.gid" :value="g.gid">{{ g.name }}（{{ g.count }}）</option>
              </select>
            </label>
            <label class="field"><span>数量</span><input v-model.number="sellGoodsForm.count" type="number" min="1" class="num" /></label>
            <button class="u-btn u-btn--red" type="button" :disabled="busy" @click="submitSellGoods">出售</button>
          </div>
        </section>
      </template>
      <p v-else class="u-hint">暂无商城信息</p>
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

.block {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--panel-border);
}

.block-title {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  color: var(--accent);
  font-size: 13px;
  font-weight: normal;
}

.res-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
}

.res-cell {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
}

.res-icon {
  width: 18px;
  height: 18px;
  object-fit: contain;
}

.item-icon {
  width: 26px;
  height: 26px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--bg);
}

.grid4 {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
}

.field {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
}

.field > span {
  color: var(--text-dim);
}

.field.check {
  color: var(--text-dim);
}

.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px 8px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.num {
  width: 76px;
  padding: 3px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.num.wide {
  width: 140px;
}

select {
  padding: 3px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.dim {
  color: var(--text-dim);
}

.price {
  color: var(--accent);
}
</style>
