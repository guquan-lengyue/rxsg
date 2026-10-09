import http from './http'

import type {
  BagArmor,
  CombineResult,
  EmbedResult,
  EquipResult,
  HeroArmorItem,
  StrongResult,
} from '@/types'

// —— 装备（对齐后端 armor 包 REST 端点）——

export async function listBag(): Promise<BagArmor[]> {
  const { data } = await http.get<BagArmor[]>('/armors')
  return data
}

export async function getHeroArmors(hid: number): Promise<HeroArmorItem[]> {
  const { data } = await http.get<HeroArmorItem[]>(`/heroes/${hid}/armors`)
  return data
}

export async function equipArmor(hid: number, sid: number, spart: number): Promise<EquipResult> {
  const { data } = await http.post<EquipResult>('/armors/equip', { hid, sid, spart })
  return data
}

export async function offloadArmor(hid: number, spart: number): Promise<EquipResult> {
  const { data } = await http.post<EquipResult>('/armors/offload', { hid, spart })
  return data
}

export async function repairArmor(
  cid: number,
  sid: number,
): Promise<{ sid: number; hp_max: number; gold: number }> {
  const { data } = await http.post<{ sid: number; hp_max: number; gold: number }>(
    '/armors/repair',
    { cid, sid },
  )
  return data
}

export async function repairAllArmor(
  cid: number,
  sids: number[],
): Promise<{ gold: number }> {
  const { data } = await http.post<{ gold: number }>('/armors/repair-all', { cid, sids })
  return data
}

export async function renovateArmor(sid: number): Promise<{ sid: number }> {
  const { data } = await http.post<{ sid: number }>('/armors/renovate', { sid })
  return data
}

export async function renovateAllArmor(sids: number[]): Promise<{ ok: boolean }> {
  const { data } = await http.post<{ ok: boolean }>('/armors/renovate-all', { sids })
  return data
}

export async function sellArmor(
  cid: number,
  sid: number,
): Promise<{ sid: number; cid: number; gold: number }> {
  const { data } = await http.post<{ sid: number; cid: number; gold: number }>('/armors/sell', {
    cid,
    sid,
  })
  return data
}

export async function chaijieArmor(sid: number): Promise<{ msg: string }> {
  const { data } = await http.post<{ msg: string }>('/armors/chaijie', { sid })
  return data
}

export interface StrongPayload {
  cid: number
  gid1: number
  good1_count: number
  gid2: number
  good2_count: number
  sid: number
  is_zuoji: number
}

export async function strongArmor(p: StrongPayload): Promise<StrongResult> {
  const { data } = await http.post<StrongResult>('/armors/strong', p)
  return data
}

export async function combineArmor(
  mainSid: number,
  mainFlag: number,
  subSid1: number,
  subSid2: number,
  goodsFlag: number,
): Promise<CombineResult> {
  const { data } = await http.post<CombineResult>('/armors/combine', {
    main_sid: mainSid,
    main_flag: mainFlag,
    sub_sid1: subSid1,
    sub_sid2: subSid2,
    goods_flag: goodsFlag,
  })
  return data
}

export async function initHoles(cid: number, sid: number): Promise<{ reduce_gold: number }> {
  const { data } = await http.post<{ reduce_gold: number }>('/armors/holes', { cid, sid })
  return data
}

// open-hole 返回结算后的装备行（map），仅取所需列。
export async function openHole(
  sid: number,
  gid: number,
  pos: number,
  useType: number,
  count: number,
): Promise<Partial<BagArmor>> {
  const { data } = await http.post<Partial<BagArmor>>('/armors/open-hole', {
    sid,
    gid,
    pos,
    use_type: useType,
    count,
  })
  return data
}

export async function embedPearl(
  sid: number,
  pos: number,
  gid: number,
  isZuoji: number,
): Promise<EmbedResult> {
  const { data } = await http.post<EmbedResult>('/armors/embed', {
    sid,
    pos,
    gid,
    is_zuoji: isZuoji,
  })
  return data
}
