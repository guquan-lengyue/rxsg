import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as cityApi from '@/api/city'
import type {
  Building,
  BuildingDetail,
  City,
  CityDetail,
  CityResource,
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
    startHeartbeat,
    stopHeartbeat,
  }
})