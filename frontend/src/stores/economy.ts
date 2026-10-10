import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as economyApi from '@/api/economy'
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
  StoreInfoResult,
  StoreRatePayload,
  UserGood,
  WorkshopInfoResult,
} from '@/api/economy'

/** 仓库视图（解析 store/info 数组）。 */
export interface StoreView {
  foodBase: number
  woodBase: number
  rockBase: number
  ironBase: number
  storageTech: number
  storeCount: number
  storeMax: number
  foodRate: number
  woodRate: number
  rockRate: number
  ironRate: number
  copper: number
}

/** 作坊货架项。 */
export interface WorkshopSlot {
  gid: number
  name: string
  price: number
}

/** 作坊视图（解析 workshop/info 数组）。 */
export interface WorkshopView {
  leaveTime: number
  goodCount: number
  slots: WorkshopSlot[]
}

/** 商城视图（解析 shop/info 数组）。 */
export interface ShopView {
  wuzhu: number
  point: number
  normal: ShopGood[]
  recommend: ShopGood[]
  wuzhuGoods: CopperGood[]
  pointGoods: CopperGood[]
}

function toNum(v: unknown): number {
  if (typeof v === 'number') {
    return Number.isFinite(v) ? v : 0
  }
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : 0
  }
  return 0
}

function toStr(v: unknown): string {
  if (typeof v === 'string') {
    return v
  }
  if (v === null || v === undefined) {
    return ''
  }
  return String(v)
}

function asRecord(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === 'object' ? (v as Record<string, unknown>) : {}
}

function parseStore(raw: StoreInfoResult): StoreView {
  const rate = raw[7] ?? { food_store: 0, wood_store: 0, rock_store: 0, iron_store: 0 }
  return {
    foodBase: toNum(raw[0]),
    woodBase: toNum(raw[1]),
    rockBase: toNum(raw[2]),
    ironBase: toNum(raw[3]),
    storageTech: toNum(raw[4]),
    storeCount: toNum(raw[5]),
    storeMax: toNum(raw[6]),
    foodRate: toNum(rate.food_store),
    woodRate: toNum(rate.wood_store),
    rockRate: toNum(rate.rock_store),
    ironRate: toNum(rate.iron_store),
    copper: toNum(raw[8]),
  }
}

function parseWorkshop(raw: WorkshopInfoResult): WorkshopView {
  const leaveTime = toNum(raw[0])
  let goodCount = 0
  const slots: WorkshopSlot[] = []
  if (raw.length >= 2) {
    goodCount = toNum(raw[1])
    for (let i = 2; i < raw.length; i += 2) {
      const good = asRecord(raw[i])
      slots.push({
        gid: toNum(good.gid),
        name: toStr(good.name),
        price: toNum(raw[i + 1]),
      })
    }
  }
  return { leaveTime, goodCount, slots }
}

function parseShop(raw: economyApi.ShopInfoResult): ShopView {
  return {
    wuzhu: toNum(raw[0]),
    point: toNum(raw[1]),
    normal: Array.isArray(raw[2]) ? raw[2] : [],
    recommend: Array.isArray(raw[3]) ? raw[3] : [],
    wuzhuGoods: Array.isArray(raw[4]) ? raw[4] : [],
    pointGoods: Array.isArray(raw[5]) ? raw[5] : [],
  }
}

// M6 经济 store：市场 / 仓库 / 作坊 / 商城 四块数据。
export const useEconomyStore = defineStore('economy', () => {
  const market = ref<MarketInfo | null>(null)
  const merchant = ref<MerchantInfo | null>(null)
  const buyList = ref<BuyListResult | null>(null)
  const sellData = ref<SellDataResult | null>(null)
  const autoTrans = ref<AutoTransRow[]>([])
  const autoTransHas = ref(false)
  const store = ref<StoreView | null>(null)
  const workshop = ref<WorkshopView | null>(null)
  const shop = ref<ShopView | null>(null)
  const goods = ref<UserGood[]>([])

  // —— 读取 ——

  async function loadMarket(cid: number): Promise<MarketInfo> {
    market.value = await economyApi.getMarketInfo(cid)
    return market.value
  }

  async function loadMerchant(cid: number): Promise<MerchantInfo> {
    merchant.value = await economyApi.getMerchantInfo(cid)
    return merchant.value
  }

  async function loadBuyList(cid: number, params: BuyListParams): Promise<BuyListResult> {
    buyList.value = await economyApi.getBuyList(cid, params)
    return buyList.value
  }

  async function loadSellData(cid: number): Promise<SellDataResult> {
    sellData.value = await economyApi.getSellData(cid)
    return sellData.value
  }

  async function loadAutoTrans(cid: number): Promise<AutoTransRow[]> {
    autoTrans.value = await economyApi.listAutoTrans(cid)
    return autoTrans.value
  }

  async function loadAutoTransHas(cid: number): Promise<boolean> {
    const res = await economyApi.hasAutoTrans(cid)
    autoTransHas.value = Array.isArray(res) ? Boolean(res[0]) : false
    return autoTransHas.value
  }

  async function loadStore(cid: number): Promise<StoreView> {
    store.value = parseStore(await economyApi.getStoreInfo(cid))
    return store.value
  }

  async function loadWorkshop(cid: number): Promise<WorkshopView> {
    workshop.value = parseWorkshop(await economyApi.getWorkshopInfo(cid))
    return workshop.value
  }

  async function loadShop(): Promise<ShopView> {
    shop.value = parseShop(await economyApi.getShopInfo())
    return shop.value
  }

  async function loadGoods(): Promise<UserGood[]> {
    goods.value = await economyApi.listUserGoods()
    return goods.value
  }

  // —— 市场写入 ——

  async function buyMerchant(cid: number, p: MerchantTradePayload): Promise<void> {
    await economyApi.buyFromMerchant(cid, p)
  }

  async function sellMerchant(cid: number, p: MerchantTradePayload): Promise<void> {
    await economyApi.sellToMerchant(cid, p)
  }

  async function sellUser(cid: number, p: SellUserPayload): Promise<MarketInfo> {
    market.value = await economyApi.sellToUser(cid, p)
    return market.value
  }

  async function buyUser(cid: number, id: number): Promise<MarketInfo> {
    market.value = await economyApi.buyFromUser(cid, id)
    return market.value
  }

  async function cancelSell(cid: number, id: number): Promise<MarketInfo> {
    market.value = await economyApi.cancelSell(cid, id)
    return market.value
  }

  async function accelerateSell(cid: number, id: number): Promise<MarketInfo> {
    market.value = await economyApi.accelerateSell(cid, id)
    return market.value
  }

  async function addAutoTrans(cid: number, p: AutoTransPayload): Promise<AutoTransRow[]> {
    autoTrans.value = await economyApi.addAutoTrans(cid, p)
    return autoTrans.value
  }

  async function removeAutoTrans(cid: number, id: number): Promise<void> {
    await economyApi.removeAutoTrans(cid, id)
    await loadAutoTrans(cid)
  }

  async function cancelAutoTrans(cid: number, id: number): Promise<MarketInfo> {
    market.value = await economyApi.cancelAutoTrans(cid, id)
    return market.value
  }

  // —— 仓库写入 ——

  async function setStoreRate(cid: number, p: StoreRatePayload): Promise<void> {
    await economyApi.setStoreRate(cid, p)
    await loadStore(cid)
  }

  async function packStore(cid: number, items: PackItem[]): Promise<void> {
    await economyApi.packStore(cid, items)
    await Promise.all([loadStore(cid), loadGoods()])
  }

  // —— 作坊写入 ——

  async function refreshWorkshop(cid: number, isNeedWuzhu: number): Promise<WorkshopView> {
    workshop.value = parseWorkshop(await economyApi.refreshWorkshop(cid, isNeedWuzhu))
    return workshop.value
  }

  async function buyWorkshopGood(cid: number, gid: number): Promise<WorkshopView> {
    const res = await economyApi.buyWorkshopGood(cid, gid)
    // 返回 ["购买成功！", loadInit...]；取第二项为最新货架。
    workshop.value = parseWorkshop(Array.isArray(res[1]) ? (res[1] as WorkshopInfoResult) : res)
    return workshop.value
  }

  // —— 商城写入 ——

  async function buyGoods(p: ShopBuyPayload): Promise<void> {
    await economyApi.buyGoods(p)
    await Promise.all([loadShop(), loadGoods()])
  }

  async function buyGoodsBeforeUse(p: ShopBuyPayload): Promise<void> {
    await economyApi.buyGoodsBeforeUse(p)
    await Promise.all([loadShop(), loadGoods()])
  }

  async function exchangeLiquan(code: string): Promise<void> {
    await economyApi.exchangeLiquan(code)
    await loadGoods()
  }

  async function loadHeroAttr(hid: number): Promise<void> {
    await economyApi.getHeroAttr(hid)
  }

  async function loadArmorAttr(aid: number): Promise<unknown[]> {
    return economyApi.getArmorAttr(aid)
  }

  async function sellGoods(p: SellGoodsPayload): Promise<void> {
    await economyApi.sellGoods(p)
    await loadGoods()
  }

  // 背包道具 gid→名称（商城 cfg_shops 无名称时的回退解析）。
  function goodName(gid: number): string {
    return goods.value.find((g) => g.gid === gid)?.name ?? ''
  }

  return {
    market,
    merchant,
    buyList,
    sellData,
    autoTrans,
    autoTransHas,
    store,
    workshop,
    shop,
    goods,
    loadMarket,
    loadMerchant,
    loadBuyList,
    loadSellData,
    loadAutoTrans,
    loadAutoTransHas,
    loadStore,
    loadWorkshop,
    loadShop,
    loadGoods,
    buyMerchant,
    sellMerchant,
    sellUser,
    buyUser,
    cancelSell,
    accelerateSell,
    addAutoTrans,
    removeAutoTrans,
    cancelAutoTrans,
    setStoreRate,
    packStore,
    refreshWorkshop,
    buyWorkshopGood,
    buyGoods,
    buyGoodsBeforeUse,
    exchangeLiquan,
    loadHeroAttr,
    loadArmorAttr,
    sellGoods,
    goodName,
  }
})
