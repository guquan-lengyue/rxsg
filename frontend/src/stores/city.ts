import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as cityApi from '@/api/city'
import type { Building, City, CityDetail, CityResource } from '@/types'

// 心跳间隔对齐 legacy getCityBaseInfo 的 10s 节奏。
const HEARTBEAT_MS = 10000

export const useCityStore = defineStore('city', () => {
  const cities = ref<City[]>([])
  const currentCity = ref<City | null>(null)
  const detail = ref<CityDetail | null>(null)
  const resources = ref<CityResource | null>(null)
  const buildings = ref<Building[]>([])

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
    return d
  }

  async function refreshResources(cid: number): Promise<void> {
    resources.value = await cityApi.getResources(cid)
  }

  function startHeartbeat(cid: number): void {
    stopHeartbeat()
    timer = setInterval(() => {
      void refreshResources(cid)
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
    loadCities,
    loadCity,
    refreshResources,
    startHeartbeat,
    stopHeartbeat,
  }
})