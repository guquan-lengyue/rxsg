import http from './http'

import type {
  Alarm,
  ArmyInfo,
  Building,
  BuildingDetail,
  City,
  CityDefence,
  CityDetail,
  CityResource,
  CitySoldier,
  DispatchPayload,
  Field,
  Hero,
  HeroInfo,
  HotelInfo,
  March,
  Technic,
  TechnicInfo,
} from '@/types'

export async function listCities(): Promise<City[]> {
  const { data } = await http.get<City[]>('/cities')
  return data
}

export async function getCity(cid: number): Promise<CityDetail> {
  const { data } = await http.get<CityDetail>(`/cities/${cid}`)
  return data
}

export async function getResources(cid: number): Promise<CityResource> {
  const { data } = await http.get<CityResource>(`/cities/${cid}/resources`)
  return data
}

export async function getBuildings(cid: number): Promise<Building[]> {
  const { data } = await http.get<Building[]>(`/cities/${cid}/buildings`)
  return data
}

export async function getBuildingDetail(
  cid: number,
  bid: number,
  x: number,
  y: number,
): Promise<BuildingDetail> {
  const { data } = await http.get<BuildingDetail>(`/cities/${cid}/buildings/info`, {
    params: { bid, x, y },
  })
  return data
}

export async function upgradeBuilding(
  cid: number,
  bid: number,
  x: number,
  y: number,
): Promise<Building[]> {
  const { data } = await http.post<Building[]>(`/cities/${cid}/buildings/upgrade`, { bid, x, y })
  return data
}

export async function stopBuilding(
  cid: number,
  bid: number,
  x: number,
  y: number,
): Promise<Building[]> {
  const { data } = await http.post<Building[]>(`/cities/${cid}/buildings/stop`, { bid, x, y })
  return data
}

export async function getTechnics(cid: number): Promise<Technic[]> {
  const { data } = await http.get<Technic[]>(`/cities/${cid}/technics`)
  return data
}

export async function getTechnicInfo(cid: number): Promise<TechnicInfo> {
  const { data } = await http.get<TechnicInfo>(`/cities/${cid}/technics/info`)
  return data
}

export async function upgradeTechnic(cid: number, tid: number): Promise<TechnicInfo> {
  const { data } = await http.post<TechnicInfo>(`/cities/${cid}/technics/upgrade`, { tid })
  return data
}

export async function stopTechnic(cid: number, tid: number): Promise<TechnicInfo> {
  const { data } = await http.post<TechnicInfo>(`/cities/${cid}/technics/stop`, { tid })
  return data
}

export async function getTroops(cid: number): Promise<CitySoldier[]> {
  const { data } = await http.get<CitySoldier[]>(`/cities/${cid}/troops`)
  return data
}

export async function getDefences(cid: number): Promise<CityDefence[]> {
  const { data } = await http.get<CityDefence[]>(`/cities/${cid}/defences`)
  return data
}

export async function getHeroes(cid: number): Promise<Hero[]> {
  const { data } = await http.get<Hero[]>(`/cities/${cid}/heroes`)
  return data
}

export async function getAlarms(cid: number): Promise<Alarm> {
  const { data } = await http.get<Alarm>(`/cities/${cid}/alarms`)
  return data
}

// —— 军事：征兵 / 出征 ——

export async function getArmyInfo(cid: number): Promise<ArmyInfo> {
  const { data } = await http.get<ArmyInfo>(`/cities/${cid}/army/info`)
  return data
}

export async function draftSoldier(cid: number, sid: number, count: number): Promise<ArmyInfo> {
  const { data } = await http.post<ArmyInfo>(`/cities/${cid}/army/draft`, { sid, count })
  return data
}

export async function stopDraft(cid: number, qid: number): Promise<ArmyInfo> {
  const { data } = await http.post<ArmyInfo>(`/cities/${cid}/army/draft/stop`, { qid })
  return data
}

export async function dissolveSoldier(cid: number, sid: number, count: number): Promise<ArmyInfo> {
  const { data } = await http.post<ArmyInfo>(`/cities/${cid}/army/dissolve`, { sid, count })
  return data
}

export async function getFields(cid: number): Promise<Field[]> {
  const { data } = await http.get<Field[]>(`/cities/${cid}/army/fields`)
  return data
}

export async function getMarches(cid: number): Promise<March[]> {
  const { data } = await http.get<March[]>(`/cities/${cid}/army/marches`)
  return data
}

export async function dispatchArmy(cid: number, payload: DispatchPayload): Promise<March[]> {
  const { data } = await http.post<March[]>(`/cities/${cid}/army/dispatch`, payload)
  return data
}

export async function recallArmy(cid: number, troopId: number): Promise<March[]> {
  const { data } = await http.post<March[]>(`/cities/${cid}/army/recall`, { troop_id: troopId })
  return data
}

// —— 武将：详情 / 升级 / 历练 ——

export async function getHeroInfo(cid: number): Promise<HeroInfo> {
  const { data } = await http.get<HeroInfo>(`/cities/${cid}/heroes/info`)
  return data
}

export async function upgradeHero(cid: number, hid: number): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/upgrade`, { hid })
  return data
}

export async function startHeroExpr(
  cid: number,
  hid: number,
  exprType: number,
  hours: number,
  carrymoney: number,
): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/expr/start`, {
    hid,
    exprType,
    hours,
    carrymoney,
  })
  return data
}

export async function cancelHeroExpr(cid: number, hid: number): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/expr/cancel`, { hid })
  return data
}

export async function fasterHeroExpr(cid: number, hid: number): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/expr/faster`, { hid })
  return data
}

export async function addHeroPoint(
  cid: number,
  hid: number,
  affairs: number,
  bravery: number,
  wisdom: number,
): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/point/add`, {
    hid,
    affairs,
    bravery,
    wisdom,
  })
  return data
}

export async function clearHeroPoint(cid: number, hid: number): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/point/clear`, { hid })
  return data
}

export async function setHeroOffice(cid: number, sets: [number, number][]): Promise<HeroInfo> {
  const { data } = await http.post<HeroInfo>(`/cities/${cid}/heroes/office`, { sets })
  return data
}

// —— 客栈：招募池 / 招募 / 招贤榜重置 ——

export async function getHotelInfo(cid: number): Promise<HotelInfo> {
  const { data } = await http.get<HotelInfo>(`/cities/${cid}/hotel/info`)
  return data
}

export async function recruitHero(cid: number, id: number): Promise<HotelInfo> {
  const { data } = await http.post<HotelInfo>(`/cities/${cid}/hotel/recruit`, { id })
  return data
}

export async function resetHotel(cid: number): Promise<HotelInfo> {
  const { data } = await http.post<HotelInfo>(`/cities/${cid}/hotel/reset`)
  return data
}

// —— 内政：收税 / 征收 / 安抚 / 产出比例 ——
// 对齐 backend city/internal_affairs.go（CityFunc.php changeTax/levyResource/pacifyPeople/getCityProduct）
// 与 economy SetCityProductRate。征收/安抚成功经 HTTP 200 返回 {message,resource}；
// 各类失败（冷却/资源不足/越界）经 HTTP 400 + message 返回，调用方统一走 errorMessage 展示。

/** 征收结果（POST /cities/:cid/levy）：成功文案 + 最新资源。 */
export interface LevyResult {
  message: string
  resource: CityResource
}

/** 安抚结果（POST /cities/:cid/pacify）：成功文案 + 最新资源。 */
export interface PacifyResult {
  message: string
  resource: CityResource
}

/** 城市产出明细（GET /cities/:cid/product，对齐后端 city.ProductInfo）。 */
export interface CityProduct {
  foodRate: number
  woodRate: number
  rockRate: number
  ironRate: number
  foodPeople: number
  woodPeople: number
  rockPeople: number
  ironPeople: number
  foodBase: number
  woodBase: number
  rockBase: number
  ironBase: number
  foodTechnic: number
  woodTechnic: number
  rockTechnic: number
  ironTechnic: number
  foodArmyUse: number
  chiefAdd: number
  skillGold: number
  skillFood: number
  skillWood: number
  skillRock: number
  skillIron: number
}

/** 产出比例提交（POST /cities/:cid/product-rate，四项百分比）。 */
export interface ProductRatePayload {
  food: number
  wood: number
  rock: number
  iron: number
}

/** 设置税率（POST /cities/:cid/tax）：返回最新资源（含 tax/morale/complaint）。 */
export async function setTax(cid: number, tax: number): Promise<CityResource> {
  const { data } = await http.post<CityResource>(`/cities/${cid}/tax`, { tax })
  return data
}

/** 征收资源（POST /cities/:cid/levy）。resid: 0金/1粮/2木/3石/4铁。 */
export async function levyResource(cid: number, resid: number): Promise<LevyResult> {
  const { data } = await http.post<LevyResult>(`/cities/${cid}/levy`, { resid })
  return data
}

/** 安抚百姓（POST /cities/:cid/pacify）。action: 0赈灾/1祈福/2祭天/3增丁。 */
export async function pacifyPeople(cid: number, action: number): Promise<PacifyResult> {
  const { data } = await http.post<PacifyResult>(`/cities/${cid}/pacify`, { action })
  return data
}

/** 查询城市产出（GET /cities/:cid/product）。 */
export async function getCityProduct(cid: number): Promise<CityProduct> {
  const { data } = await http.get<CityProduct>(`/cities/${cid}/product`)
  return data
}

/** 提交产出比例（POST /cities/:cid/product-rate）。 */
export async function setCityProductRate(cid: number, payload: ProductRatePayload): Promise<void> {
  await http.post(`/cities/${cid}/product-rate`, payload)
}