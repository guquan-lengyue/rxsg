import http from './http'

import type {
  Alarm,
  Building,
  City,
  CityDefence,
  CityDetail,
  CityResource,
  CitySoldier,
  Hero,
  Technic,
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

export async function getTechnics(cid: number): Promise<Technic[]> {
  const { data } = await http.get<Technic[]>(`/cities/${cid}/technics`)
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