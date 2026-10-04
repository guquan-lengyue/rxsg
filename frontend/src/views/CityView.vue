<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import BuildingGrid from '@/components/BuildingGrid.vue'
import BuildingPanel from '@/components/BuildingPanel.vue'
import ArmyPanel from '@/components/ArmyPanel.vue'
import ResourceBar from '@/components/ResourceBar.vue'
import TechnicPanel from '@/components/TechnicPanel.vue'
import { errorMessage } from '@/api/http'
import { zhCN } from '@/lang/zh-CN'
import { useAuthStore } from '@/stores/auth'
import { useCityStore } from '@/stores/city'
import type { Building, BuildingDetail, DispatchPayload } from '@/types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const city = useCityStore()

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

async function selectBuilding(b: Building): Promise<void> {
  if (!activeCid.value) {
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
        <button class="tech-btn" @click="openTechnics">{{ zhCN.technic.open }}</button>
        <button class="tech-btn" @click="openArmy">{{ zhCN.army.open }}</button>
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

.tech-btn {
  padding: 6px 12px;
  color: var(--accent);
  background: transparent;
  border: 1px solid var(--accent);
  border-radius: 4px;
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