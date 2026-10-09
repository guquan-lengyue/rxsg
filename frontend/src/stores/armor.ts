import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as armorApi from '@/api/armor'
import type { BagArmor, HeroArmorItem } from '@/types'

// 装备 store：背包列表 + 按武将的穿戴映射（用户级数据，与城池无关）。
export const useArmorStore = defineStore('armor', () => {
  const bag = ref<BagArmor[]>([])
  const heroArmors = ref<Record<number, HeroArmorItem[]>>({})

  async function loadBag(): Promise<BagArmor[]> {
    bag.value = await armorApi.listBag()
    return bag.value
  }

  async function loadHeroArmors(hid: number): Promise<HeroArmorItem[]> {
    const list = await armorApi.getHeroArmors(hid)
    heroArmors.value = { ...heroArmors.value, [hid]: list }
    return list
  }

  async function equip(hid: number, sid: number, spart: number) {
    const res = await armorApi.equipArmor(hid, sid, spart)
    heroArmors.value = { ...heroArmors.value, [hid]: res.armors }
    await loadBag()
    return res
  }

  async function offload(hid: number, spart: number) {
    const res = await armorApi.offloadArmor(hid, spart)
    heroArmors.value = { ...heroArmors.value, [hid]: res.armors }
    await loadBag()
    return res
  }

  async function repair(cid: number, sid: number) {
    const res = await armorApi.repairArmor(cid, sid)
    await loadBag()
    return res
  }

  async function repairAll(cid: number, sids: number[]) {
    const res = await armorApi.repairAllArmor(cid, sids)
    await loadBag()
    return res
  }

  async function renovate(sid: number) {
    const res = await armorApi.renovateArmor(sid)
    await loadBag()
    return res
  }

  async function sell(cid: number, sid: number) {
    const res = await armorApi.sellArmor(cid, sid)
    await loadBag()
    return res
  }

  async function chaijie(sid: number) {
    const res = await armorApi.chaijieArmor(sid)
    await loadBag()
    return res
  }

  async function strong(p: armorApi.StrongPayload) {
    const res = await armorApi.strongArmor(p)
    await loadBag()
    return res
  }

  async function combine(mainSid: number, mainFlag: number, sub1: number, sub2: number, goodsFlag: number) {
    const res = await armorApi.combineArmor(mainSid, mainFlag, sub1, sub2, goodsFlag)
    await loadBag()
    return res
  }

  async function initHoles(cid: number, sid: number) {
    const res = await armorApi.initHoles(cid, sid)
    await loadBag()
    return res
  }

  async function openHole(sid: number, gid: number, pos: number, useType: number, count: number) {
    const res = await armorApi.openHole(sid, gid, pos, useType, count)
    await loadBag()
    return res
  }

  async function embed(sid: number, pos: number, gid: number, isZuoji: number) {
    const res = await armorApi.embedPearl(sid, pos, gid, isZuoji)
    await loadBag()
    return res
  }

  return {
    bag,
    heroArmors,
    loadBag,
    loadHeroArmors,
    equip,
    offload,
    repair,
    repairAll,
    renovate,
    sell,
    chaijie,
    strong,
    combine,
    initHoles,
    openHole,
    embed,
  }
})
