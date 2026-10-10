import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as cityApi from '@/api/city'
import type { CityProduct, LevyResult, PacifyResult, ProductRatePayload } from '@/api/city'
import type {
  ArmyInfo,
  Building,
  BuildingCandidate,
  BuildingDetail,
  BuildingQueueItem,
  City,
  CityDefence,
  CityDetail,
  CityResource,
  CitySoldier,
  DispatchPayload,
  Field,
  HeroInfo,
  HotelInfo,
  March,
  OfficeInfo,
  TechnicInfo,
} from '@/types'

// 心跳间隔对齐 legacy getCityBaseInfo 的 10s 节奏。
const HEARTBEAT_MS = 10000

export const useCityStore = defineStore('city', () => {
  const cities = ref<City[]>([])
  const currentCity = ref<City | null>(null)
  const detail = ref<CityDetail | null>(null)
  const resources = ref<CityResource | null>(null)
  const buildings = ref<Building[]>([])
  const buildingQueue = ref<BuildingQueueItem[]>([])
  const technicInfo = ref<TechnicInfo | null>(null)
  const armyInfo = ref<ArmyInfo | null>(null)
  const fields = ref<Field[]>([])
  const marches = ref<March[]>([])
  const heroInfo = ref<HeroInfo | null>(null)
  const hotelInfo = ref<HotelInfo | null>(null)
  const officeInfo = ref<OfficeInfo | null>(null)
  const product = ref<CityProduct | null>(null)
  const troops = ref<CitySoldier[]>([])
  const defences = ref<CityDefence[]>([])

  let timer: ReturnType<typeof setInterval> | null = null

  async function loadCities(): Promise<City[]> {
    cities.value = await cityApi.listCities()
    return cities.value
  }

  async function loadCity(cid: number): Promise<CityDetail> {
    const d = await cityApi.getCity(cid)
    detail.value = d
    currentCity.value = d.city
    resources.value = d.base.resource
    buildings.value = d.buildings
    buildingQueue.value = []
    technicInfo.value = null
    armyInfo.value = null
    fields.value = []
    marches.value = []
    heroInfo.value = null
    hotelInfo.value = null
    officeInfo.value = null
    product.value = null
    troops.value = []
    defences.value = []
    return d
  }

  async function refreshResources(cid: number): Promise<void> {
    resources.value = await cityApi.getResources(cid)
  }

  async function refreshBuildings(cid: number): Promise<void> {
    buildings.value = await cityApi.getBuildings(cid)
  }

  async function loadBuildingDetail(
    cid: number,
    bid: number,
    x: number,
    y: number,
  ): Promise<BuildingDetail> {
    return cityApi.getBuildingDetail(cid, bid, x, y)
  }

  // 升级/停止后后端返回最新建筑列表，同时刷新资源与详情。
  async function upgradeBuilding(
    cid: number,
    bid: number,
    x: number,
    y: number,
  ): Promise<BuildingDetail> {
    buildings.value = await cityApi.upgradeBuilding(cid, bid, x, y)
    resources.value = await cityApi.getResources(cid)
    return cityApi.getBuildingDetail(cid, bid, x, y)
  }

  async function stopBuilding(
    cid: number,
    bid: number,
    x: number,
    y: number,
  ): Promise<BuildingDetail> {
    buildings.value = await cityApi.stopBuilding(cid, bid, x, y)
    resources.value = await cityApi.getResources(cid)
    return cityApi.getBuildingDetail(cid, bid, x, y)
  }

  // —— 建筑：建造 / 拆除 / 彻底拆除 / 取消拆除 / 资源地转换 / 队列 ——

  // 建造候选列表为只读查询，不落库；由 CityView 暂存后交给 BuildDialog 展示。
  async function loadValidBuildings(cid: number, inner: number): Promise<BuildingCandidate[]> {
    return cityApi.validBuildings(cid, inner)
  }

  // 建造/拆除系列接口成功后端返回整份建筑列表，同步刷新资源与建筑队列。
  async function createBuilding(
    cid: number,
    bid: number,
    inner: number,
    x: number,
    y: number,
  ): Promise<Building[]> {
    buildings.value = await cityApi.createBuilding(cid, bid, inner, x, y)
    resources.value = await cityApi.getResources(cid)
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildings.value
  }

  async function destroyBuilding(
    cid: number,
    bid: number,
    inner: number,
    x: number,
    y: number,
  ): Promise<Building[]> {
    buildings.value = await cityApi.destroyBuilding(cid, bid, inner, x, y)
    resources.value = await cityApi.getResources(cid)
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildings.value
  }

  async function destroyAllBuilding(
    cid: number,
    bid: number,
    inner: number,
    x: number,
    y: number,
  ): Promise<Building[]> {
    buildings.value = await cityApi.destroyAllBuilding(cid, bid, inner, x, y)
    resources.value = await cityApi.getResources(cid)
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildings.value
  }

  async function cancelDestroyBuilding(
    cid: number,
    bid: number,
    inner: number,
    x: number,
    y: number,
  ): Promise<Building[]> {
    buildings.value = await cityApi.cancelDestroyBuilding(cid, bid, inner, x, y)
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildings.value
  }

  async function exchangeBuilding(
    cid: number,
    bid: number,
    targetbid: number,
    inner: number,
    x: number,
    y: number,
  ): Promise<Building[]> {
    buildings.value = await cityApi.exchangeBuilding(cid, bid, targetbid, inner, x, y)
    resources.value = await cityApi.getResources(cid)
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildings.value
  }

  async function loadBuildingQueue(cid: number): Promise<BuildingQueueItem[]> {
    buildingQueue.value = await cityApi.buildingQueue(cid)
    return buildingQueue.value
  }

  async function loadTechnicInfo(cid: number): Promise<TechnicInfo> {
    technicInfo.value = await cityApi.getTechnicInfo(cid)
    return technicInfo.value
  }

  // 研究/取消后后端返回最新科技列表，同时刷新资源。
  async function upgradeTechnic(cid: number, tid: number): Promise<TechnicInfo> {
    technicInfo.value = await cityApi.upgradeTechnic(cid, tid)
    resources.value = await cityApi.getResources(cid)
    return technicInfo.value
  }

  async function stopTechnic(cid: number, tid: number): Promise<TechnicInfo> {
    technicInfo.value = await cityApi.stopTechnic(cid, tid)
    resources.value = await cityApi.getResources(cid)
    return technicInfo.value
  }

  // —— 军事：征兵 / 出征 ——

  async function loadArmyInfo(cid: number): Promise<ArmyInfo> {
    armyInfo.value = await cityApi.getArmyInfo(cid)
    return armyInfo.value
  }

  async function loadFields(cid: number): Promise<Field[]> {
    fields.value = await cityApi.getFields(cid)
    return fields.value
  }

  async function loadMarches(cid: number): Promise<March[]> {
    marches.value = await cityApi.getMarches(cid)
    return marches.value
  }

  async function draft(cid: number, sid: number, count: number): Promise<ArmyInfo> {
    armyInfo.value = await cityApi.draftSoldier(cid, sid, count)
    resources.value = await cityApi.getResources(cid)
    return armyInfo.value
  }

  async function cancelDraft(cid: number, qid: number): Promise<ArmyInfo> {
    armyInfo.value = await cityApi.stopDraft(cid, qid)
    resources.value = await cityApi.getResources(cid)
    return armyInfo.value
  }

  async function dissolve(cid: number, sid: number, count: number): Promise<ArmyInfo> {
    armyInfo.value = await cityApi.dissolveSoldier(cid, sid, count)
    resources.value = await cityApi.getResources(cid)
    return armyInfo.value
  }

  // 出征后兵力减少，需同步刷新兵营信息与资源。
  async function dispatch(cid: number, payload: DispatchPayload): Promise<March[]> {
    marches.value = await cityApi.dispatchArmy(cid, payload)
    armyInfo.value = await cityApi.getArmyInfo(cid)
    resources.value = await cityApi.getResources(cid)
    return marches.value
  }

  // 召回后部队进入返程，刷新行军列表。
  async function recall(cid: number, troopId: number): Promise<March[]> {
    marches.value = await cityApi.recallArmy(cid, troopId)
    return marches.value
  }

  // —— 武将：详情 / 升级 / 历练 ——

  async function loadHeroInfo(cid: number): Promise<HeroInfo> {
    heroInfo.value = await cityApi.getHeroInfo(cid)
    return heroInfo.value
  }

  // 升级消耗经验，历练结束惰性加经验：两者都以整份列表回写。
  async function upgradeHero(cid: number, hid: number): Promise<HeroInfo> {
    heroInfo.value = await cityApi.upgradeHero(cid, hid)
    return heroInfo.value
  }

  async function startHeroExpr(
    cid: number,
    hid: number,
    exprType: number,
    hours: number,
    carrymoney: number,
  ): Promise<HeroInfo> {
    heroInfo.value = await cityApi.startHeroExpr(cid, hid, exprType, hours, carrymoney)
    return heroInfo.value
  }

  async function cancelHeroExpr(cid: number, hid: number): Promise<HeroInfo> {
    heroInfo.value = await cityApi.cancelHeroExpr(cid, hid)
    return heroInfo.value
  }

  async function fasterHeroExpr(cid: number, hid: number): Promise<HeroInfo> {
    heroInfo.value = await cityApi.fasterHeroExpr(cid, hid)
    return heroInfo.value
  }

  async function addHeroPoint(
    cid: number,
    hid: number,
    affairs: number,
    bravery: number,
    wisdom: number,
  ): Promise<HeroInfo> {
    heroInfo.value = await cityApi.addHeroPoint(cid, hid, affairs, bravery, wisdom)
    return heroInfo.value
  }

  async function clearHeroPoint(cid: number, hid: number): Promise<HeroInfo> {
    heroInfo.value = await cityApi.clearHeroPoint(cid, hid)
    return heroInfo.value
  }

  async function setHeroOffice(cid: number, sets: [number, number][]): Promise<HeroInfo> {
    heroInfo.value = await cityApi.setHeroOffice(cid, sets)
    return heroInfo.value
  }

  // —— 官署：面板聚合读取（对齐后端 hero.OfficeInfo）——

  async function loadOfficeInfo(cid: number): Promise<OfficeInfo> {
    officeInfo.value = await cityApi.getOfficeInfo(cid)
    return officeInfo.value
  }

  // —— 客栈：招募池 / 招募 / 招贤榜重置 ——

  async function loadHotelInfo(cid: number): Promise<HotelInfo> {
    hotelInfo.value = await cityApi.getHotelInfo(cid)
    return hotelInfo.value
  }

  // 招募扣城金并入城武将：同步刷新资源、武将列表与酒店信息。
  async function recruitHero(cid: number, id: number): Promise<HotelInfo> {
    hotelInfo.value = await cityApi.recruitHero(cid, id)
    resources.value = await cityApi.getResources(cid)
    if (heroInfo.value) {
      await loadHeroInfo(cid)
    }
    return hotelInfo.value
  }

  // 招贤榜重置消耗道具（元宝侧），刷新酒店信息即可（池全量重建）。
  async function resetHotel(cid: number): Promise<HotelInfo> {
    hotelInfo.value = await cityApi.resetHotel(cid)
    return hotelInfo.value
  }

  // —— 内政：收税 / 征收 / 安抚 / 产出比例 ——

  // 调税直接回写最新资源（后端返回整份 city_resources）。
  async function setTax(cid: number, tax: number): Promise<CityResource> {
    resources.value = await cityApi.setTax(cid, tax)
    return resources.value
  }

  // 征收后民心-20：后端随响应回带最新资源，直接整份回写。
  async function levy(cid: number, resid: number): Promise<LevyResult> {
    const r = await cityApi.levyResource(cid, resid)
    resources.value = r.resource
    return r
  }

  // 安抚百姓：同上，随响应回写资源。
  async function pacify(cid: number, action: number): Promise<PacifyResult> {
    const r = await cityApi.pacifyPeople(cid, action)
    resources.value = r.resource
    return r
  }

  async function loadProduct(cid: number): Promise<CityProduct> {
    product.value = await cityApi.getCityProduct(cid)
    return product.value
  }

  // 提交产出比例后重新查询，回写最新四项比例。
  async function setProductRate(cid: number, payload: ProductRatePayload): Promise<CityProduct> {
    await cityApi.setCityProductRate(cid, payload)
    product.value = await cityApi.getCityProduct(cid)
    return product.value
  }

  // —— 外城：驻军 / 城防器械 ——

  async function loadTroops(cid: number): Promise<CitySoldier[]> {
    troops.value = await cityApi.getTroops(cid)
    return troops.value
  }

  async function loadDefences(cid: number): Promise<CityDefence[]> {
    defences.value = await cityApi.getDefences(cid)
    return defences.value
  }

  function startHeartbeat(cid: number): void {
    stopHeartbeat()
    timer = setInterval(() => {
      void refreshResources(cid)
      // 建筑/科技等到期结算由后端惰性触发，心跳带上建筑列表以刷新等级与状态。
      void refreshBuildings(cid)
      // 建筑队列条常驻顶栏，随心跳刷新（到期结算由后端惰性触发）。
      void loadBuildingQueue(cid)
      // 科技面板打开过才刷新，避免无谓请求。
      if (technicInfo.value) {
        void loadTechnicInfo(cid)
      }
      // 军队面板打开过才刷新（征兵/行军皆为惰性结算，靠轮询推进）。
      if (armyInfo.value) {
        void loadArmyInfo(cid)
        void loadMarches(cid)
      }
      // 武将面板打开过才刷新（历练到期靠惰性结算推进）。
      if (heroInfo.value) {
        void loadHeroInfo(cid)
      }
      // 客栈面板打开过才刷新（招募池按刷新块惰性重建）。
      if (hotelInfo.value) {
        void loadHotelInfo(cid)
      }
    }, HEARTBEAT_MS)
  }

  function stopHeartbeat(): void {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  return {
    cities,
    currentCity,
    detail,
    resources,
    buildings,
    buildingQueue,
    technicInfo,
    armyInfo,
    fields,
    marches,
    heroInfo,
    hotelInfo,
    officeInfo,
    product,
    troops,
    defences,
    loadCities,
    loadCity,
    refreshResources,
    refreshBuildings,
    loadBuildingDetail,
    upgradeBuilding,
    stopBuilding,
    loadValidBuildings,
    createBuilding,
    destroyBuilding,
    destroyAllBuilding,
    cancelDestroyBuilding,
    exchangeBuilding,
    loadBuildingQueue,
    loadTechnicInfo,
    upgradeTechnic,
    stopTechnic,
    loadArmyInfo,
    loadFields,
    loadMarches,
    draft,
    cancelDraft,
    dissolve,
    dispatch,
    recall,
    loadHeroInfo,
    upgradeHero,
    startHeroExpr,
    cancelHeroExpr,
    fasterHeroExpr,
    addHeroPoint,
    clearHeroPoint,
    setHeroOffice,
    loadOfficeInfo,
    loadHotelInfo,
    recruitHero,
    resetHotel,
    setTax,
    levy,
    pacify,
    loadProduct,
    setProductRate,
    loadTroops,
    loadDefences,
    startHeartbeat,
    stopHeartbeat,
  }
})