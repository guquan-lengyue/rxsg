// M6 经济（市场 / 仓库 / 工匠作坊 / 商城 / 出售宝物）REST 封装。
// 后端多为 legacy 数组返回值（[a, b, ...]），此处按 Go service 的真实结构体逐个对齐；
// 成功类提示（如"购买成功！"）由后端经 400 + message 返回，调用方统一走 errorMessage 展示。
import http from './http'

import type { BuildingDetail } from '@/types'

// —— 通用行类型（后端 `select *` 原样返回，字段沿用 legacy 下划线命名）——

/** 城市交易挂单行（city_trades）。 */
export interface TradeRow {
  id: number
  cid: number
  buycid: number
  state: number
  restype: number
  count: number
  price: number
  gold: number
  distance: number
  unionid: number
  limittime: number
  endtime: number
  sellername?: string
  unionname?: string
}

/** 市场信息（GET /cities/:cid/market/info）：市场建筑详情 + 本城挂单。 */
export interface MarketInfo {
  building: BuildingDetail
  trades: TradeRow[]
}

/** 商人当日库存与收购价（GET /cities/:cid/market/merchant）。 */
export interface MerchantInfo {
  city_id: number
  food: number
  wood: number
  rock: number
  iron: number
  gold: number
  trade_day: number
  food_buy_price: string
  wood_buy_price: string
  rock_buy_price: string
  iron_buy_price: string
}

/** 玩家挂单列表（GET /market/buylist）：[占用商队, 市场等级, 总页数, 当前页, 行]。 */
export type BuyListResult = [number, number, number, number, TradeRow[]]

/** 出售参考数据（GET /market/selldata）：[占用, 等级, 粮价, 木价, 石价, 铁价]。 */
export type SellDataResult = [number, number, number, number, number, number]

/** 自动运输行（GET /market/autotrans）。 */
export interface AutoTransRow {
  id: number
  fromcid: number
  fromcity: string
  tocid: number
  tocity: string
  res_type: number
  count: number
  start_time: number
}

/** 仓库存放比例行。 */
export interface StoreRateRow {
  food_store: number
  wood_store: number
  rock_store: number
  iron_store: number
}

/** 仓库信息（GET /cities/:cid/store/info）：
 *  [粮基, 木基, 石基, 铁基, 仓储科技系数, 仓库数, 仓库容量, 存放比例行, 铜钱数]。 */
export type StoreInfoResult = [
  number,
  number,
  number,
  number,
  number,
  number,
  number,
  StoreRateRow,
  number,
]

/** 打包请求项（对齐后端 store.PackItem）。 */
export interface PackItem {
  type: string
  pack_count: number
  res_count: number
  copper: number
}

/** 作坊货架原始行（cfg_goods）。 */
export interface WorkshopGood {
  gid: number
  name?: string
}

/** 作坊信息原始数组：[冷却剩余] 或 [冷却剩余, 数量, 货, 价格, 货, 价格, ...]。 */
export type WorkshopInfoResult = unknown[]

/** 商城商品（cfg_shops）。 */
export interface ShopGood {
  id: number
  gid: number
  price: number
  pack: number
  group: number
  position: number
  commend: number
  rebate: number
  totalCount: number
  userbuycnt: number
  daybuycnt: number
}

/** 五铢钱/积分商品（cfg_goods_copper ⨝ cfg_goods）。 */
export interface CopperGood {
  gid: number
  name?: string
  price: number
  type: number
}

/** 商城信息（GET /shop/info）：[五铢钱, 积分, 常规商品, 推荐商品, 五铢钱商品, 积分商品]。 */
export type ShopInfoResult = [
  number,
  number,
  ShopGood[],
  ShopGood[],
  CopperGood[],
  CopperGood[],
]

/** 用户背包道具（GET /goods，M2 已有，经济面板用于名称解析与出售宝物）。 */
export interface UserGood {
  gid: number
  count: number
  name: string
  group_id: number
  position: number
  value: number
}

/** 出售宝物结果（POST /goods/sell）：[道具行, cid, 城金]。 */
export type SellGoodsResult = [UserGood, number, number]

// —— 请求体 ——

export interface MerchantTradePayload {
  food: number
  wood: number
  rock: number
  iron: number
  times: number
  paytype: number
}

export interface SellUserPayload {
  resType: number
  count: number
  gold: number
  hour: number
  unionOnly: number
}

export interface BuyListParams {
  page: number
  filter: number
  unionOnly: number
  sellName: string
}

export interface AutoTransPayload {
  fromcid: number
  tocid: number
  resType: number
  count: number
  transType: number
  startTime: number
}

export interface ShopBuyPayload {
  id: number
  cnt: number
  paytype: number
  gid: number
}

export interface SellGoodsPayload {
  cid: number
  gid: number
  count: number
}

export interface StoreRatePayload {
  food: number
  wood: number
  rock: number
  iron: number
}

export interface ProductRatePayload {
  food: number
  wood: number
  rock: number
  iron: number
}

// —— 市场 ——

export async function getMarketInfo(cid: number): Promise<MarketInfo> {
  const { data } = await http.get<MarketInfo>(`/cities/${cid}/market/info`)
  return data
}

export async function getMerchantInfo(cid: number): Promise<MerchantInfo> {
  const { data } = await http.get<MerchantInfo>(`/cities/${cid}/market/merchant`)
  return data
}

// 成功提示经 400 message 返回，无成功响应体。
export async function buyFromMerchant(cid: number, p: MerchantTradePayload): Promise<void> {
  await http.post(`/cities/${cid}/market/buy-merchant`, p)
}

export async function sellToMerchant(cid: number, p: MerchantTradePayload): Promise<void> {
  await http.post(`/cities/${cid}/market/sell-merchant`, p)
}

export async function sellToUser(cid: number, p: SellUserPayload): Promise<MarketInfo> {
  const { data } = await http.post<MarketInfo>(`/cities/${cid}/market/sell-user`, p)
  return data
}

export async function buyFromUser(cid: number, id: number): Promise<MarketInfo> {
  const { data } = await http.post<MarketInfo>(`/cities/${cid}/market/buy-user`, { id })
  return data
}

export async function getBuyList(cid: number, p: BuyListParams): Promise<BuyListResult> {
  const { data } = await http.get<BuyListResult>(`/cities/${cid}/market/buylist`, {
    params: { page: p.page, filter: p.filter, unionOnly: p.unionOnly, sellName: p.sellName },
  })
  return data
}

export async function getSellData(cid: number): Promise<SellDataResult> {
  const { data } = await http.get<SellDataResult>(`/cities/${cid}/market/selldata`)
  return data
}

export async function cancelSell(cid: number, id: number): Promise<MarketInfo> {
  const { data } = await http.post<MarketInfo>(`/cities/${cid}/market/cancel-sell`, { id })
  return data
}

export async function cancelAutoTrans(cid: number, id: number): Promise<MarketInfo> {
  const { data } = await http.post<MarketInfo>(`/cities/${cid}/market/cancel-autotrans`, { id })
  return data
}

export async function accelerateSell(cid: number, id: number): Promise<MarketInfo> {
  const { data } = await http.post<MarketInfo>(`/cities/${cid}/market/accelerate`, { id })
  return data
}

export async function listAutoTrans(cid: number): Promise<AutoTransRow[]> {
  const { data } = await http.get<AutoTransRow[]>(`/cities/${cid}/market/autotrans`)
  return data
}

export async function addAutoTrans(cid: number, p: AutoTransPayload): Promise<AutoTransRow[]> {
  const { data } = await http.post<AutoTransRow[]>(`/cities/${cid}/market/autotrans`, p)
  return data
}

export async function removeAutoTrans(cid: number, id: number): Promise<number[]> {
  const { data } = await http.post<number[]>(`/cities/${cid}/market/autotrans/remove`, { id })
  return data
}

export async function hasAutoTrans(cid: number): Promise<[boolean]> {
  const { data } = await http.get<[boolean]>(`/cities/${cid}/market/autotrans/has`)
  return data
}

export async function setProductRate(cid: number, p: ProductRatePayload): Promise<unknown[]> {
  const { data } = await http.post<unknown[]>(`/cities/${cid}/product-rate`, p)
  return data
}

// —— 仓库 / 粮仓 ——

export async function getStoreInfo(cid: number): Promise<StoreInfoResult> {
  const { data } = await http.get<StoreInfoResult>(`/cities/${cid}/store/info`)
  return data
}

// 成功文案经 400 message 返回（"修改仓库存放比例成功！"）。
export async function setStoreRate(cid: number, p: StoreRatePayload): Promise<void> {
  await http.post(`/cities/${cid}/store/rate`, p)
}

export async function packStore(cid: number, items: PackItem[]): Promise<number[]> {
  const { data } = await http.post<number[]>(`/cities/${cid}/store/pack`, { items })
  return data
}

// —— 工匠作坊（cid 走 query / body，非路径参数）——

export async function getWorkshopInfo(cid: number): Promise<WorkshopInfoResult> {
  const { data } = await http.get<WorkshopInfoResult>('/workshop/info', { params: { cid } })
  return data
}

export async function refreshWorkshop(cid: number, isNeedWuzhu: number): Promise<WorkshopInfoResult> {
  const { data } = await http.post<WorkshopInfoResult>('/workshop/refresh', {
    cid,
    isNeedWuzhu,
  })
  return data
}

export async function buyWorkshopGood(cid: number, gid: number): Promise<unknown[]> {
  const { data } = await http.post<unknown[]>('/workshop/buy', { cid, gid })
  return data
}

// —— 商城 ——

export async function getShopInfo(): Promise<ShopInfoResult> {
  const { data } = await http.get<ShopInfoResult>('/shop/info')
  return data
}

export async function buyGoods(p: ShopBuyPayload): Promise<unknown[]> {
  const { data } = await http.post<unknown[]>('/shop/buy', p)
  return data
}

export async function buyGoodsBeforeUse(p: ShopBuyPayload): Promise<unknown[]> {
  const { data } = await http.post<unknown[]>('/shop/buy-before-use', p)
  return data
}

export async function exchangeLiquan(code: string): Promise<unknown[]> {
  const { data } = await http.post<unknown[]>('/shop/exchange', { code })
  return data
}

export async function getHeroAttr(hid: number): Promise<{ ok: boolean }> {
  const { data } = await http.get<{ ok: boolean }>('/shop/hero-attr', { params: { hid } })
  return data
}

export async function getArmorAttr(aid: number): Promise<unknown[]> {
  const { data } = await http.get<unknown[]>('/shop/armor-attr', { params: { aid } })
  return data
}

export async function sellGoods(p: SellGoodsPayload): Promise<SellGoodsResult> {
  const { data } = await http.post<SellGoodsResult>('/goods/sell', p)
  return data
}

// —— 背包道具（M2 已有 GET /goods；经济面板用于名称解析与出售选择）——

export async function listUserGoods(): Promise<UserGood[]> {
  const { data } = await http.get<UserGood[]>('/goods')
  return data
}
