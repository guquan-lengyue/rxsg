<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import BuildingGrid from '@/components/BuildingGrid.vue'
import BuildingPanel from '@/components/BuildingPanel.vue'
import ArmyPanel from '@/components/ArmyPanel.vue'
import ArmorPanel from '@/components/ArmorPanel.vue'
import HeroPanel from '@/components/HeroPanel.vue'
import HotelPanel from '@/components/HotelPanel.vue'
import ResourceBar from '@/components/ResourceBar.vue'
import TechnicPanel from '@/components/TechnicPanel.vue'
import { errorMessage } from '@/api/http'
import type { StrongPayload } from '@/api/armor'
import { zhCN } from '@/lang/zh-CN'
import { useArmorStore } from '@/stores/armor'
import { useAuthStore } from '@/stores/auth'
import { useCityStore } from '@/stores/city'
import type { Building, BuildingDetail, DispatchPayload } from '@/types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const city = useCityStore()
const armor = useArmorStore()

const loading = ref(false)
const error = ref('')
const activeCid = ref(0)

// 建筑面板状态
const selected = ref<Building | null>(null)
const buildingDetail = ref<BuildingDetail | null>(null)
const panelLoading = ref(false)
const panelError = ref('')
const busy = ref(false)

// 科技面板状态
const showTechnics = ref(false)
const technicLoading = ref(false)
const technicError = ref('')
const busyTid = ref(0)

async function openTechnics(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showTechnics.value = true
  technicError.value = ''
  technicLoading.value = true
  try {
    await city.loadTechnicInfo(activeCid.value)
  } catch (e) {
    technicError.value = errorMessage(e)
  } finally {
    technicLoading.value = false
  }
}

function closeTechnics(): void {
  showTechnics.value = false
  technicError.value = ''
  busyTid.value = 0
}

async function onTechnicUpgrade(tid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  busyTid.value = tid
  technicError.value = ''
  try {
    await city.upgradeTechnic(activeCid.value, tid)
  } catch (e) {
    technicError.value = errorMessage(e)
  } finally {
    busyTid.value = 0
  }
}

async function onTechnicStop(tid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  busyTid.value = tid
  technicError.value = ''
  try {
    await city.stopTechnic(activeCid.value, tid)
  } catch (e) {
    technicError.value = errorMessage(e)
  } finally {
    busyTid.value = 0
  }
}

// 军队面板状态
const showArmy = ref(false)
const armyLoading = ref(false)
const armyError = ref('')
const armyBusy = ref(false)

// 可出征的武将：仅空闲（state=0）。
const idleHeroes = computed(() => (city.detail?.base.heroes ?? []).filter((h) => h.state === 0))

async function openArmy(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showArmy.value = true
  armyError.value = ''
  armyLoading.value = true
  try {
    await Promise.all([
      city.loadArmyInfo(activeCid.value),
      city.loadFields(activeCid.value),
      city.loadMarches(activeCid.value),
    ])
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyLoading.value = false
  }
}

function closeArmy(): void {
  showArmy.value = false
  armyError.value = ''
  armyBusy.value = false
}

async function onDraft(sid: number, count: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armyBusy.value = true
  armyError.value = ''
  try {
    await city.draft(activeCid.value, sid, count)
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyBusy.value = false
  }
}

async function onStopDraft(qid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armyBusy.value = true
  armyError.value = ''
  try {
    await city.cancelDraft(activeCid.value, qid)
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyBusy.value = false
  }
}

async function onDissolve(sid: number, count: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armyBusy.value = true
  armyError.value = ''
  try {
    await city.dissolve(activeCid.value, sid, count)
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyBusy.value = false
  }
}

async function onDispatch(payload: DispatchPayload): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armyBusy.value = true
  armyError.value = ''
  try {
    await city.dispatch(activeCid.value, payload)
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyBusy.value = false
  }
}

async function onRecall(id: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armyBusy.value = true
  armyError.value = ''
  try {
    await city.recall(activeCid.value, id)
  } catch (e) {
    armyError.value = errorMessage(e)
  } finally {
    armyBusy.value = false
  }
}

// 武将面板状态
const showHeroes = ref(false)
const heroLoading = ref(false)
const heroError = ref('')
const heroBusy = ref(false)

async function openHeroes(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showHeroes.value = true
  heroError.value = ''
  heroLoading.value = true
  try {
    await city.loadHeroInfo(activeCid.value)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroLoading.value = false
  }
}

function closeHeroes(): void {
  showHeroes.value = false
  heroError.value = ''
  heroBusy.value = false
}

async function onHeroUpgrade(hid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.upgradeHero(activeCid.value, hid)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

async function onHeroStartExpr(
  hid: number,
  exprType: number,
  hours: number,
  carrymoney: number,
): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.startHeroExpr(activeCid.value, hid, exprType, hours, carrymoney)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

async function onHeroCancelExpr(hid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.cancelHeroExpr(activeCid.value, hid)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

async function onHeroFasterExpr(hid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.fasterHeroExpr(activeCid.value, hid)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

async function onHeroAddPoint(hid: number, affairs: number, bravery: number, wisdom: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.addHeroPoint(activeCid.value, hid, affairs, bravery, wisdom)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

async function onHeroClearPoint(hid: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  heroBusy.value = true
  heroError.value = ''
  try {
    await city.clearHeroPoint(activeCid.value, hid)
  } catch (e) {
    heroError.value = errorMessage(e)
  } finally {
    heroBusy.value = false
  }
}

// 客栈面板状态
const showHotel = ref(false)
const hotelLoading = ref(false)
const hotelError = ref('')
const hotelBusy = ref(false)

async function openHotel(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showHotel.value = true
  hotelError.value = ''
  hotelLoading.value = true
  try {
    await city.loadHotelInfo(activeCid.value)
  } catch (e) {
    hotelError.value = errorMessage(e)
  } finally {
    hotelLoading.value = false
  }
}

function closeHotel(): void {
  showHotel.value = false
  hotelError.value = ''
  hotelBusy.value = false
}

async function onHotelRecruit(id: number): Promise<void> {
  if (!activeCid.value) {
    return
  }
  hotelBusy.value = true
  hotelError.value = ''
  try {
    await city.recruitHero(activeCid.value, id)
  } catch (e) {
    hotelError.value = errorMessage(e)
  } finally {
    hotelBusy.value = false
  }
}

async function onHotelReset(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  hotelBusy.value = true
  hotelError.value = ''
  try {
    await city.resetHotel(activeCid.value)
  } catch (e) {
    hotelError.value = errorMessage(e)
  } finally {
    hotelBusy.value = false
  }
}

// 装备面板状态
const showArmor = ref(false)
const armorLoading = ref(false)
const armorError = ref('')
const armorBusy = ref(false)

async function openArmor(): Promise<void> {
  showArmor.value = true
  armorError.value = ''
  armorLoading.value = true
  try {
    await Promise.all([
      armor.loadBag(),
      city.heroInfo ? Promise.resolve() : activeCid.value ? city.loadHeroInfo(activeCid.value) : Promise.resolve(),
    ])
  } catch (e) {
    armorError.value = errorMessage(e)
  } finally {
    armorLoading.value = false
  }
}

function closeArmor(): void {
  showArmor.value = false
  armorError.value = ''
  armorBusy.value = false
}

async function runArmor(fn: () => Promise<unknown>): Promise<void> {
  if (!activeCid.value) {
    return
  }
  armorBusy.value = true
  armorError.value = ''
  try {
    await fn()
    // 修复/出售等扣城金，同步刷新资源
    await city.refreshResources(activeCid.value)
  } catch (e) {
    armorError.value = errorMessage(e)
  } finally {
    armorBusy.value = false
  }
}

function onArmorEquip(hid: number, sid: number, spart: number): void {
  void runArmor(() => armor.equip(hid, sid, spart))
}

function onArmorOffload(hid: number, spart: number): void {
  void runArmor(() => armor.offload(hid, spart))
}

function onArmorRepair(sid: number): void {
  void runArmor(() => armor.repair(activeCid.value, sid))
}

function onArmorRenovate(sid: number): void {
  void runArmor(() => armor.renovate(sid))
}

function onArmorSell(sid: number): void {
  void runArmor(() => armor.sell(activeCid.value, sid))
}

function onArmorChaijie(sid: number): void {
  void runArmor(() => armor.chaijie(sid))
}

function onArmorStrong(p: StrongPayload): void {
  void runArmor(() => armor.strong(p))
}

function onArmorCombine(mainSid: number, sub1: number, sub2: number, goodsFlag: number): void {
  void runArmor(() => armor.combine(mainSid, 1, sub1, sub2, goodsFlag))
}

function onArmorInitHoles(sid: number): void {
  void runArmor(() => armor.initHoles(activeCid.value, sid))
}

function onArmorOpenHole(sid: number, gid: number, pos: number, useType: number, count: number): void {
  void runArmor(() => armor.openHole(sid, gid, pos, useType, count))
}

function onArmorEmbed(sid: number, pos: number, gid: number): void {
  void runArmor(() => armor.embed(sid, pos, gid, 0))
}

function onArmorLoadHero(hid: number): void {
  void runArmor(() => armor.loadHeroArmors(hid))
}

async function selectBuilding(b: Building): Promise<void> {
  if (!activeCid.value) {
    return
  }
  // 原版点击客栈建筑直接进入招贤馆（bid=12 为扩展槽位，legacy HOTEL=10 与校场冲突）。
  if (b.bid === 12) {
    await openHotel()
    return
  }
  selected.value = b
  buildingDetail.value = null
  panelError.value = ''
  panelLoading.value = true
  try {
    buildingDetail.value = await city.loadBuildingDetail(activeCid.value, b.bid, b.x, b.y)
  } catch (e) {
    panelError.value = errorMessage(e)
  } finally {
    panelLoading.value = false
  }
}

async function refreshSelectedDetail(): Promise<void> {
  const b = selected.value
  if (!b || !activeCid.value || busy.value) {
    return
  }
  try {
    buildingDetail.value = await city.loadBuildingDetail(activeCid.value, b.bid, b.x, b.y)
  } catch (e) {
    panelError.value = errorMessage(e)
  }
}

async function onUpgrade(): Promise<void> {
  const b = selected.value
  if (!b || !activeCid.value) {
    return
  }
  busy.value = true
  panelError.value = ''
  try {
    buildingDetail.value = await city.upgradeBuilding(activeCid.value, b.bid, b.x, b.y)
  } catch (e) {
    panelError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function onStop(): Promise<void> {
  const b = selected.value
  if (!b || !activeCid.value) {
    return
  }
  busy.value = true
  panelError.value = ''
  try {
    buildingDetail.value = await city.stopBuilding(activeCid.value, b.bid, b.x, b.y)
  } catch (e) {
    panelError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

function closePanel(): void {
  selected.value = null
  buildingDetail.value = null
  panelError.value = ''
}

// 心跳刷新建筑列表后，同步刷新已选建筑详情（用于展示升级完成后的等级）。
watch(() => city.buildings, refreshSelectedDetail, { deep: false })

const title = computed(() => city.currentCity?.name || '—')

function parseCid(): number {
  const raw = route.params.cid
  const value = Array.isArray(raw) ? raw[0] : raw
  const n = Number(value)
  return Number.isFinite(n) && n > 0 ? n : 0
}

async function enterCity(cid: number): Promise<void> {
  loading.value = true
  error.value = ''
  closePanel()
  closeTechnics()
  closeArmy()
  closeHeroes()
  closeHotel()
  try {
    if (!city.cities.length) {
      await city.loadCities()
    }
    const target = cid || city.cities[0]?.cid || 0
    if (!target) {
      error.value = '当前账号还没有城池'
      return
    }
    activeCid.value = target
    await city.loadCity(target)
    city.startHeartbeat(target)
    if (parseCid() !== target) {
      await router.replace({ name: 'city', params: { cid: String(target) } })
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

async function switchCity(cid: number): Promise<void> {
  if (cid === activeCid.value) {
    return
  }
  city.stopHeartbeat()
  await enterCity(cid)
}

async function onLogout(): Promise<void> {
  city.stopHeartbeat()
  await auth.logout()
  await router.replace({ name: 'login' })
}

onMounted(() => {
  void enterCity(parseCid())
})

onBeforeUnmount(() => {
  city.stopHeartbeat()
})
</script>

<template>
  <div class="city-page">
    <header class="topbar">
      <div class="left">
        <strong class="city-name">{{ title }}</strong>
        <span v-if="city.detail?.province" class="province">
          {{ city.detail.province.province }} · {{ city.detail.province.jun }}
        </span>
      </div>
      <div class="right">
        <select
          v-if="city.cities.length > 1"
          :value="activeCid"
          @change="switchCity(Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="c in city.cities" :key="c.cid" :value="c.cid">{{ c.name }}（{{ c.cid }}）</option>
        </select>
        <button class="nav-tech" type="button" @click="openTechnics">{{ zhCN.technic.open }}</button>
        <button class="nav-btn army" type="button" title="军事" @click="openArmy"></button>
        <button class="nav-btn hero" type="button" title="武将" @click="openHeroes"></button>
        <button class="nav-armor" type="button" @click="openArmor">{{ zhCN.armor.open }}</button>
        <span class="user">{{ auth.user?.name || auth.user?.passport }}</span>
        <button class="logout" @click="onLogout">{{ zhCN.city.logout }}</button>
      </div>
    </header>

    <main class="content">
      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>

      <template v-if="!loading && !error">
        <ResourceBar :resource="city.resources" />
        <div class="city-body">
          <BuildingGrid
            :buildings="city.buildings"
            :selected="selected"
            @select="selectBuilding"
          />
          <BuildingPanel
            v-if="selected"
            :detail="buildingDetail"
            :loading="panelLoading"
            :error="panelError"
            :busy="busy"
            @upgrade="onUpgrade"
            @stop="onStop"
            @close="closePanel"
          />
        </div>
      </template>
    </main>

    <TechnicPanel
      v-if="showTechnics"
      :info="city.technicInfo"
      :loading="technicLoading"
      :error="technicError"
      :busy-tid="busyTid"
      @upgrade="onTechnicUpgrade"
      @stop="onTechnicStop"
      @close="closeTechnics"
    />

    <ArmyPanel
      v-if="showArmy"
      :info="city.armyInfo"
      :fields="city.fields"
      :marches="city.marches"
      :heroes="idleHeroes"
      :loading="armyLoading"
      :error="armyError"
      :busy="armyBusy"
      @draft="onDraft"
      @stop-draft="onStopDraft"
      @dissolve="onDissolve"
      @dispatch="onDispatch"
      @recall="onRecall"
      @close="closeArmy"
    />

    <HeroPanel
      v-if="showHeroes"
      :info="city.heroInfo"
      :loading="heroLoading"
      :error="heroError"
      :busy="heroBusy"
      @upgrade="onHeroUpgrade"
      @add-point="onHeroAddPoint"
      @clear-point="onHeroClearPoint"
      @start-expr="onHeroStartExpr"
      @cancel-expr="onHeroCancelExpr"
      @faster-expr="onHeroFasterExpr"
      @close="closeHeroes"
    />

    <HotelPanel
      v-if="showHotel"
      :info="city.hotelInfo"
      :loading="hotelLoading"
      :error="hotelError"
      :busy="hotelBusy"
      :gold="city.resources?.gold ?? 0"
      @recruit="onHotelRecruit"
      @reset="onHotelReset"
      @close="closeHotel"
    />

    <ArmorPanel
      v-if="showArmor"
      :bag="armor.bag"
      :heroes="city.heroInfo?.heroes ?? []"
      :hero-armors="armor.heroArmors"
      :cid="activeCid"
      :gold="city.resources?.gold ?? 0"
      :loading="armorLoading"
      :error="armorError"
      :busy="armorBusy"
      @equip="onArmorEquip"
      @offload="onArmorOffload"
      @repair="onArmorRepair"
      @renovate="onArmorRenovate"
      @sell="onArmorSell"
      @chaijie="onArmorChaijie"
      @strong="onArmorStrong"
      @combine="onArmorCombine"
      @init-holes="onArmorInitHoles"
      @open-hole="onArmorOpenHole"
      @embed="onArmorEmbed"
      @load-hero="onArmorLoadHero"
      @close="closeArmor"
    />
  </div>
</template>

<style scoped>
.city-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 100vh;
  padding: 16px;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.left,
.right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.city-name {
  color: var(--accent);
  font-size: 18px;
}

.province {
  color: var(--text-dim);
  font-size: 13px;
}

.user {
  color: var(--text-dim);
  font-size: 13px;
}

select {
  padding: 6px 8px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.logout {
  padding: 6px 12px;
  color: var(--text);
  background: transparent;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

/* 顶栏模块按钮：原版 topbutton 贴图（军事/武将 自带文字）。
   源图 topbutton_*.png 均为 61×26，显式定宽高以保持贴图不被压扁，
   且避免空内容导致宽度塌缩为 12px 竖条。 */
.nav-btn {
  width: 61px;
  height: 26px;
  flex: 0 0 auto;
  padding: 0;
  border: none;
  background: center / 100% 100% no-repeat;
}

.nav-btn:active {
  transform: translateY(1px);
}

.nav-btn.army {
  background-image: url('/images/topbutton_army.png');
}
.nav-btn.army:hover:not(:active) {
  background-image: url('/images/topbutton_army_on.png');
}
.nav-btn.army:active {
  background-image: url('/images/topbutton_army_down.png');
}

.nav-btn.hero {
  background-image: url('/images/topbutton_hero.png');
}
.nav-btn.hero:hover:not(:active) {
  background-image: url('/images/topbutton_hero_on.png');
}
.nav-btn.hero:active {
  background-image: url('/images/topbutton_hero_down.png');
}

/* 科技：原版无对应 topbutton，用 topicon_tactic 图标 + 文字合成 */
.nav-tech {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 12px;
  color: #f0e3c2;
  background: linear-gradient(180deg, #4c3b24, #2c2116);
  border: 1px solid #6d582f;
  border-radius: 3px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

/* 装备：同科技按钮样式（原版装备入口在武将面板内，此处为独立入口） */
.nav-armor {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 12px;
  color: #f0e3c2;
  background: linear-gradient(180deg, #4c3b24, #2c2116);
  border: 1px solid #6d582f;
  border-radius: 3px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
  cursor: pointer;
}

.nav-armor:hover {
  color: #ffe9a8;
  border-color: var(--accent);
}

.nav-tech::before {
  content: '';
  width: 17px;
  height: 17px;
  background: url('/images/topicon_tactic.png') center / 100% 100% no-repeat;
}

.nav-tech:hover {
  color: #ffe9a8;
  border-color: var(--accent);
}

.content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.city-body {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}

.city-body > :first-child {
  flex: 1;
  min-width: 0;
}

.hint {
  color: var(--text-dim);
}

.hint.error {
  color: var(--danger);
}
</style>