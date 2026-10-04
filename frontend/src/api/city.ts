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