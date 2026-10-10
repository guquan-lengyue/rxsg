// M11 世界地图 API 封装。
// 权威来源：backend/internal/world/handler.go（路由/请求体）与 service.go（响应结构）。
// 注意：本模块多处“成功提示”经 HTTP 400 的 message 返回（errLegacy），调用方一律用
// errorMessage(e) 照实展示，不得按状态码判成败。
import http from './http'

// —— DTO（响应字段对齐后端 JSON tag）——

/** 地格（getBlockData 的逗号串解析结果，按 wid 索引）。 */
export interface WorldCell {
  wid: number
  /** 0=城池占位；1=平原(可筑城)；2..5=野地/地形（cfg_world_type）。 */
  type: number
  /** 城池/附属平地的所属城池 id（0 为无主）。 */
  ownercid: number
  /** 1=战乱中。 */
  state: number
  level: number
  province: string
  jun: string
}

/** 联盟城池标记（union_marks）。 */
export interface WorldMark {
  id: number
  unionid: number
  cid: number
  endtime: number
  type: number
  wid: number
}

/** 单块地格数据（blockstart + 解析后的地格）。 */
export interface BlockChunk {
  blockstart: number
  cells: WorldCell[]
}

/** getBlockData 响应。 */
export interface BlockData {
  chunks: BlockChunk[]
  marks: WorldMark[]
}

/** getWorldCityInfo 单条（城池详情）。 */
export interface WorldCityInfo {
  cid: number
  citytype: number
  is_special: number
  provinceId: number
  cityname: string
  uid: number
  username: string | null
  passport: string | null
  union_id: number
  unionname: string | null
  prestige: number
  userstate: number
  flagchar: string
  userface: number
  usersex: number
  /** 小旗判定 0..8（getWorldCityInfo cityFlag）。 */
  flag: number
}

/** getWorldFieldInfo 单条（野地详情；无主时若干列为 null）。 */
export interface WorldFieldInfo {
  wid: number
  type: number
  ownercid: number
  province: string
  level: number
  cid: number | null
  cityname: string | null
  username: string | null
  prestige: number | null
  union_id: number | null
  unionname: string | null
}

/** 收藏列表：我的城池（用于选择目标）+ 收藏项。 */
export interface FavouriteCity {
  cid: number
  name: string
}

export interface Favourite {
  id: number
  cid: number
  name: string
  comments: string
}

export interface FavouritesResult {
  cities: FavouriteCity[]
  favourites: Favourite[]
}

/** getGovernInfo：officename / maxCount / todayCount。 */
export interface GovernInfo {
  officename: string
  maxCount: number
  todayCount: number
}

/** governOthers 请求体（对齐 handler.go governReq）。 */
export interface GovernPayload {
  type: number
  tcid: number
  tuid: number
  cid: number
  cityname: string
}

/** checkCanInvade：[true] 或 [false, type, total, invaded]。 */
export interface InvadeCheck {
  can: boolean
  type: number
  total: number
  invaded: number
}

/** getActionField：[type, wid[]]。 */
export interface ActionField {
  type: number
  wids: number[]
}

/** getMapCity 单条（名城）。 */
export interface MapCity {
  name: string
  cid: number
  type: number
  ownername: string | null
  union_name: string | null
}

type Raw = Record<string, unknown>

// —— 运行时取值（避免 any；后端列可能为 null）——

function num(v: unknown): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

function numOrNull(v: unknown): number | null {
  if (v === null || v === undefined) {
    return null
  }
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

function str(v: unknown): string {
  return v === null || v === undefined ? '' : String(v)
}

function strOrNull(v: unknown): string | null {
  return v === null || v === undefined ? null : String(v)
}

function asArray(v: unknown): unknown[] {
  return Array.isArray(v) ? v : []
}

function asRecord(v: unknown): Raw {
  return v !== null && typeof v === 'object' ? (v as Raw) : {}
}

function parseCityRow(v: unknown): WorldCityInfo {
  const r = asRecord(v)
  return {
    cid: num(r.cid),
    citytype: num(r.citytype),
    is_special: num(r.is_special),
    provinceId: num(r.provinceId),
    cityname: str(r.cityname),
    uid: num(r.uid),
    username: strOrNull(r.username),
    passport: strOrNull(r.passport),
    union_id: num(r.union_id),
    unionname: strOrNull(r.unionname),
    prestige: num(r.prestige),
    userstate: num(r.userstate),
    flagchar: str(r.flagchar),
    userface: num(r.userface),
    usersex: num(r.usersex),
    flag: num(r.flag),
  }
}

function parseCityList(data: unknown): WorldCityInfo[] {
  return asArray(data).map(parseCityRow)
}

function parseFavourites(data: unknown): FavouritesResult {
  const top = asArray(data)
  const cities = asArray(top[0]).map((v): FavouriteCity => {
    const r = asRecord(v)
    return { cid: num(r.cid), name: str(r.name) }
  })
  const favourites = asArray(top[1]).map((v): Favourite => {
    const r = asRecord(v)
    return { id: num(r.id), cid: num(r.cid), name: str(r.name), comments: str(r.comments) }
  })
  return { cities, favourites }
}

// —— 地格串解析 ——
// group_concat((wid - blockstart), ':', type, ':', ownercid, ':', state, ':', level, ':', province, ':', jun)
export function parseBlockConcat(blockstart: number, concat: string): WorldCell[] {
  if (!concat) {
    return []
  }
  const out: WorldCell[] = []
  for (const part of concat.split(',')) {
    const f = part.split(':')
    if (f.length < 7) {
      continue
    }
    const off = Number(f[0])
    if (!Number.isFinite(off)) {
      continue
    }
    out.push({
      wid: blockstart + off,
      type: num(f[1]),
      ownercid: num(f[2]),
      state: num(f[3]),
      level: num(f[4]),
      province: f[5] ?? '',
      jun: f[6] ?? '',
    })
  }
  return out
}

// —— 端点封装 ——

/** GET /cities/:cid/world/heroes（legacy doGetWorldInfo：该城全部武将）。 */
export async function getWorldInfo(cid: number): Promise<Raw[]> {
  const { data } = await http.get<unknown>(`/cities/${cid}/world/heroes`)
  return asArray(data).map(asRecord)
}

/** POST /world/blocks（getBlockData）。 */
export async function getBlockData(blocks: number[]): Promise<BlockData> {
  const { data } = await http.post<unknown>('/world/blocks', { blocks })
  const top = asArray(data)
  const chunks = asArray(top[0]).map((it): BlockChunk => {
    const pair = asArray(it)
    const blockstart = num(pair[0])
    return { blockstart, cells: parseBlockConcat(blockstart, str(pair[1])) }
  })
  const marks = asArray(top[1]).map((m): WorldMark => {
    const r = asRecord(m)
    return {
      id: num(r.id),
      unionid: num(r.unionid),
      cid: num(r.cid),
      endtime: num(r.endtime),
      type: num(r.type),
      wid: num(r.wid),
    }
  })
  return { chunks, marks }
}

/** POST /world/city-info（getWorldCityInfo）。 */
export async function getWorldCityInfo(cities: number[]): Promise<WorldCityInfo[]> {
  const { data } = await http.post<unknown>('/world/city-info', { cities })
  return parseCityList(data)
}

/** POST /world/field-info（getWorldFieldInfo；返回单元素数组）。 */
export async function getWorldFieldInfo(wid: number): Promise<WorldFieldInfo[]> {
  const { data } = await http.post<unknown>('/world/field-info', { wid })
  return asArray(data).map((v): WorldFieldInfo => {
    const r = asRecord(v)
    return {
      wid: num(r.wid),
      type: num(r.type),
      ownercid: num(r.ownercid),
      province: str(r.province),
      level: num(r.level),
      cid: numOrNull(r.cid),
      cityname: strOrNull(r.cityname),
      username: strOrNull(r.username),
      prestige: numOrNull(r.prestige),
      union_id: numOrNull(r.union_id),
      unionname: strOrNull(r.unionname),
    }
  })
}

/** POST /world/start-war（startWar：宣战，写 user_inwars，不产生 troops）。 */
export async function startWar(targetuid: number, targetcid: number): Promise<WorldCityInfo[]> {
  const { data } = await http.post<unknown>('/world/start-war', { targetuid, targetcid })
  return parseCityList(data)
}

/** POST /world/build-city（createCityFromLand：在己方平地筑城，返回空数组）。 */
export async function createCityFromLand(targetwid: number): Promise<void> {
  await http.post('/world/build-city', { targetwid })
}

/** POST /world/favourites（addFavourites；成功文案经 HTTP 400 message 返回）。 */
export async function addFavourites(targetcid: number): Promise<void> {
  await http.post('/world/favourites', { targetcid })
}

/** GET /world/favourites（getFavouritesList）。 */
export async function getFavouritesList(): Promise<FavouritesResult> {
  const { data } = await http.get<unknown>('/world/favourites')
  return parseFavourites(data)
}

/** POST /world/favourites/delete（deleteFavourites；成功返回最新收藏列表）。 */
export async function deleteFavourites(id: number): Promise<FavouritesResult> {
  const { data } = await http.post<unknown>('/world/favourites/delete', { id })
  return parseFavourites(data)
}

/** POST /world/favourites/comments（setFavouritesComments；成功文案经 HTTP 400 message 返回）。 */
export async function setFavouritesComments(id: number, comments: string): Promise<void> {
  await http.post('/world/favourites/comments', { id, comments })
}

/** POST /world/govern-info（getGovernInfo：[officename, maxCount, todayCount]）。 */
export async function getGovernInfo(cid: number): Promise<GovernInfo> {
  const { data } = await http.post<unknown>('/world/govern-info', { cid })
  const a = asArray(data)
  return { officename: str(a[0]), maxCount: num(a[1]), todayCount: num(a[2]) }
}

/** POST /world/govern（governOthers；成功/失败文案均经 HTTP 400 message 返回）。 */
export async function governOthers(p: GovernPayload): Promise<void> {
  await http.post('/world/govern', p)
}

/** GET /world/map-city（getMapCity：type∈(1,5) 的名城列表）。 */
export async function getMapCity(): Promise<MapCity[]> {
  const { data } = await http.get<unknown>('/world/map-city')
  return asArray(data).map((v): MapCity => {
    const r = asRecord(v)
    return {
      name: str(r.name),
      cid: num(r.cid),
      type: num(r.type),
      ownername: strOrNull(r.ownername),
      union_name: strOrNull(r.union_name),
    }
  })
}

/** POST /world/check-invade（checkCanInvade）。 */
export async function checkCanInvade(cid: number): Promise<InvadeCheck> {
  const { data } = await http.post<unknown>('/world/check-invade', { cid })
  const a = asArray(data)
  if (a.length <= 1) {
    return { can: a.length === 1 ? Boolean(a[0]) : false, type: 0, total: 0, invaded: 0 }
  }
  return { can: Boolean(a[0]), type: num(a[1]), total: num(a[2]), invaded: num(a[3]) }
}

/** POST /world/mark（markCity：联盟标记，成功文案经 HTTP 400 message 返回）。 */
export async function markCity(cid: number): Promise<void> {
  await http.post('/world/mark', { cid })
}

/** GET /world/action-field?type=（getActionField：[type, wid[]]）。 */
export async function getActionField(type: number): Promise<ActionField> {
  const { data } = await http.get<unknown>('/world/action-field', { params: { type } })
  const a = asArray(data)
  return { type: num(a[0]), wids: asArray(a[1]).map(num) }
}

/** GET /world/user-fields（getUserFields：返回 [{wid}]）。 */
export async function getUserFields(): Promise<number[]> {
  const { data } = await http.get<unknown>('/world/user-fields')
  const a = asArray(data)
  return asArray(a[0]).map((r) => num(asRecord(r).wid))
}
