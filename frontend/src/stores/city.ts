import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as cityApi from '@/api/city'
import type {
  ArmyInfo,
  Building,
  BuildingDetail,
  City,
  CityDetail,
  CityResource,
  DispatchPayload,
  Field,
  March,
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
  const technicInfo = ref<TechnicInfo | null>(null)
  const armyInfo = ref<ArmyInfo | null>(null)
  const fields = ref<Field[]>([])
  const marches = ref<March[]>([])

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
    technicInfo.value = null
    armyInfo.value = null
    fields.value = []
    marches.value = []
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

  function startHeartbeat(cid: number): void {
    stopHeartbeat()
    timer = setInterval(() => {
      void refreshResources(cid)
      // 建筑/科技等到期结算由后端惰性触发，心跳带上建筑列表以刷新等级与状态。
      void refreshBuildings(cid)
      // 科技面板打开过才刷新，避免无谓请求。
      if (technicInfo.value) {
        void loadTechnicInfo(cid)
      }
      // 军队面板打开过才刷新（征兵/行军皆为惰性结算，靠轮询推进）。
      if (armyInfo.value) {
        void loadArmyInfo(cid)
        void loadMarches(cid)
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
    technicInfo,
    armyInfo,
    fields,
    marches,
    loadCities,
    loadCity,
    refreshResources,
    refreshBuildings,
    loadBuildingDetail,
    upgradeBuilding,
    stopBuilding,
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
    startHeartbeat,
    stopHeartbeat,
  }
})