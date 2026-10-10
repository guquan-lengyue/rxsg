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
import MarketPanel from '@/components/MarketPanel.vue'
import BattlePanel from '@/components/BattlePanel.vue'
import TaskPanel from '@/components/TaskPanel.vue'
import AchievementPanel from '@/components/AchievementPanel.vue'
import LotteryPanel from '@/components/LotteryPanel.vue'
import PkPanel from '@/components/PkPanel.vue'
import CivilPanel from '@/components/CivilPanel.vue'
import OutCityPanel from '@/components/OutCityPanel.vue'
import WorldMapPanel from '@/components/WorldMapPanel.vue'
import { errorMessage } from '@/api/http'
import type { StrongPayload } from '@/api/armor'
import type { ProductRatePayload } from '@/api/city'
import type { GovernPayload } from '@/api/world'
import { cidToWid } from '@/render/worldGrid'
import type {
  AutoTransPayload,
  BuyListParams,
  MerchantTradePayload,
  PackItem,
  SellGoodsPayload,
  SellUserPayload,
  ShopBuyPayload,
  StoreRatePayload,
} from '@/api/economy'
import type { AchievementFilter } from '@/api/achievement'
import { zhCN } from '@/lang/zh-CN'
import { useArmorStore } from '@/stores/armor'
import { useAuthStore } from '@/stores/auth'
import { useCityStore } from '@/stores/city'
import { useEconomyStore } from '@/stores/economy'
import { useBattleStore } from '@/stores/battle'
import { useTaskStore } from '@/stores/task'
import { useAchievementStore } from '@/stores/achievement'
import { useLotteryStore } from '@/stores/lottery'
import { usePkStore } from '@/stores/pk'
import { useWorldStore } from '@/stores/world'
import type { Building, BuildingDetail, DispatchPayload } from '@/types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const city = useCityStore()
const armor = useArmorStore()
const economy = useEconomyStore()
const battle = useBattleStore()
const task = useTaskStore()
const achievement = useAchievementStore()
const lottery = useLotteryStore()
const pk = usePkStore()
const world = useWorldStore()

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

// 经济面板状态（市场 / 仓库 / 作坊 / 商城）
const showEconomy = ref(false)
const economyLoading = ref(false)
const economyError = ref('')
const economyBusy = ref(false)

async function openEconomy(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showEconomy.value = true
  economyError.value = ''
  economyLoading.value = true
  try {
    await Promise.all([
      economy.loadMarket(activeCid.value),
      economy.loadMerchant(activeCid.value),
      economy.loadSellData(activeCid.value),
      economy.loadAutoTrans(activeCid.value),
      economy.loadAutoTransHas(activeCid.value),
      economy.loadStore(activeCid.value),
      economy.loadWorkshop(activeCid.value),
      economy.loadShop(),
      economy.loadGoods(),
    ])
  } catch (e) {
    economyError.value = errorMessage(e)
  } finally {
    economyLoading.value = false
  }
}

function closeEconomy(): void {
  showEconomy.value = false
  economyError.value = ''
  economyBusy.value = false
}

async function runEconomy(fn: () => Promise<unknown>, refresh = false): Promise<void> {
  if (!activeCid.value) {
    return
  }
  economyBusy.value = true
  economyError.value = ''
  try {
    await fn()
    if (refresh) {
      await city.refreshResources(activeCid.value)
    }
  } catch (e) {
    economyError.value = errorMessage(e)
  } finally {
    economyBusy.value = false
  }
}

function onEconomyLoadMerchant(): void {
  void runEconomy(() => economy.loadMerchant(activeCid.value))
}

function onEconomyBuyMerchant(p: MerchantTradePayload): void {
  void runEconomy(() => economy.buyMerchant(activeCid.value, p), true)
}

function onEconomySellMerchant(p: MerchantTradePayload): void {
  void runEconomy(() => economy.sellMerchant(activeCid.value, p), true)
}

function onEconomySellUser(p: SellUserPayload): void {
  void runEconomy(() => economy.sellUser(activeCid.value, p), true)
}

function onEconomyBuyUser(id: number): void {
  void runEconomy(() => economy.buyUser(activeCid.value, id), true)
}

function onEconomyCancelSell(id: number): void {
  void runEconomy(() => economy.cancelSell(activeCid.value, id), true)
}

function onEconomyAccelerate(id: number): void {
  void runEconomy(() => economy.accelerateSell(activeCid.value, id), true)
}

function onEconomyLoadBuyList(p: BuyListParams): void {
  void runEconomy(() => economy.loadBuyList(activeCid.value, p))
}

function onEconomyAddAutoTrans(p: AutoTransPayload): void {
  void runEconomy(() => economy.addAutoTrans(activeCid.value, p), true)
}

function onEconomyRemoveAutoTrans(id: number): void {
  void runEconomy(() => economy.removeAutoTrans(activeCid.value, id), true)
}

function onEconomyCancelAutoTrans(id: number): void {
  void runEconomy(() => economy.cancelAutoTrans(activeCid.value, id), true)
}

function onEconomyStoreRate(p: StoreRatePayload): void {
  void runEconomy(() => economy.setStoreRate(activeCid.value, p))
}

function onEconomyStorePack(items: PackItem[]): void {
  void runEconomy(() => economy.packStore(activeCid.value, items), true)
}

function onEconomyWorkshopRefresh(isNeedWuzhu: number): void {
  void runEconomy(() => economy.refreshWorkshop(activeCid.value, isNeedWuzhu), true)
}

function onEconomyWorkshopBuy(gid: number): void {
  void runEconomy(() => economy.buyWorkshopGood(activeCid.value, gid), true)
}

function onEconomyShopBuy(p: ShopBuyPayload, beforeUse: boolean): void {
  void runEconomy(() => (beforeUse ? economy.buyGoodsBeforeUse(p) : economy.buyGoods(p)), true)
}

function onEconomyShopExchange(code: string): void {
  void runEconomy(() => economy.exchangeLiquan(code), true)
}

function onEconomyShopArmorAttr(aid: number): void {
  void runEconomy(() => economy.loadArmorAttr(aid))
}

function onEconomyShopHeroAttr(hid: number): void {
  void runEconomy(() => economy.loadHeroAttr(hid))
}

function onEconomySellGoods(p: SellGoodsPayload): void {
  void runEconomy(() => economy.sellGoods(p), true)
}

// 战斗战报面板状态
const showBattle = ref(false)
const battleLoading = ref(false)
const battleError = ref('')
const battleBusy = ref(false)

async function openBattle(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showBattle.value = true
  battleError.value = ''
  battleLoading.value = true
  battle.clearDetail()
  try {
    await battle.loadList(activeCid.value)
  } catch (e) {
    battleError.value = errorMessage(e)
  } finally {
    battleLoading.value = false
  }
}

function closeBattle(): void {
  showBattle.value = false
  battleError.value = ''
  battleBusy.value = false
}

async function runBattle(fn: () => Promise<unknown>): Promise<void> {
  battleBusy.value = true
  battleError.value = ''
  try {
    await fn()
  } catch (e) {
    battleError.value = errorMessage(e)
  } finally {
    battleBusy.value = false
  }
}

function onBattleSelect(bid: number): void {
  void runBattle(() => battle.loadDetail(bid))
}

function onBattleLoadReports(): void {
  if (!activeCid.value) {
    return
  }
  void runBattle(() => battle.loadReports(activeCid.value))
}

// 任务面板状态
const showTasks = ref(false)
const taskLoading = ref(false)
const taskError = ref('')
const taskBusy = ref(false)

async function openTasks(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showTasks.value = true
  taskError.value = ''
  taskLoading.value = true
  try {
    await task.selectType(activeCid.value, task.activeType)
  } catch (e) {
    taskError.value = errorMessage(e)
  } finally {
    taskLoading.value = false
  }
}

function closeTasks(): void {
  showTasks.value = false
  taskError.value = ''
  taskBusy.value = false
}

async function runTask(fn: () => Promise<unknown>, refresh = false): Promise<void> {
  if (!activeCid.value) {
    return
  }
  taskBusy.value = true
  taskError.value = ''
  try {
    await fn()
    if (refresh) {
      await city.refreshResources(activeCid.value)
    }
  } catch (e) {
    taskError.value = errorMessage(e)
  } finally {
    taskBusy.value = false
  }
}

function onTaskSelectType(type: number): void {
  void runTask(() => task.selectType(activeCid.value, type))
}

function onTaskSelectGroup(group: number): void {
  void runTask(() => task.selectGroup(activeCid.value, group))
}

function onTaskOpenDetail(tid: number): void {
  void runTask(() => task.loadDetail(activeCid.value, tid))
}

function onTaskBack(): void {
  task.clearDetail()
}

function onTaskClaim(tid: number, selectId: number, selectId2: number): void {
  void runTask(() => task.claim(activeCid.value, tid, selectId, selectId2), true)
}

function onTaskDrop(group: number): void {
  void runTask(() => task.drop(activeCid.value, group))
}

function onTaskSysDrop(tid: number): void {
  void runTask(() => task.dropSys(activeCid.value, tid))
}

// 成就面板状态
const showAchievements = ref(false)
const achievementLoading = ref(false)
const achievementError = ref('')
const achievementBusy = ref(false)

async function openAchievements(): Promise<void> {
  showAchievements.value = true
  achievementError.value = ''
  achievementLoading.value = true
  try {
    const overview = await achievement.loadOverview()
    const group = overview.groups[0]?.group ?? 0
    await achievement.loadList(group, 0, achievement.filter, 0)
  } catch (e) {
    achievementError.value = errorMessage(e)
  } finally {
    achievementLoading.value = false
  }
}

function closeAchievements(): void {
  showAchievements.value = false
  achievementError.value = ''
  achievementBusy.value = false
}

async function runAchievement(fn: () => Promise<unknown>): Promise<void> {
  achievementBusy.value = true
  achievementError.value = ''
  try {
    await fn()
  } catch (e) {
    achievementError.value = errorMessage(e)
  } finally {
    achievementBusy.value = false
  }
}

function onAchievementSelectGroup(group: number): void {
  void runAchievement(() =>
    achievement.loadList(group, achievement.activeSubgroup, achievement.filter, 0),
  )
}

function onAchievementSelectFilter(filter: AchievementFilter): void {
  void runAchievement(() =>
    achievement.loadList(achievement.activeGroup, achievement.activeSubgroup, filter, 0),
  )
}

function onAchievementSelectPage(page: number): void {
  void runAchievement(() =>
    achievement.loadList(achievement.activeGroup, achievement.activeSubgroup, achievement.filter, page),
  )
}

function onAchievementOpenDetail(aid: number): void {
  void runAchievement(() => achievement.loadDetail(aid))
}

function onAchievementBack(): void {
  achievement.clearDetail()
}

// 幸运宝匣面板状态
const showLottery = ref(false)
const lotteryLoading = ref(false)
const lotteryError = ref('')
const lotteryBusy = ref(false)

async function openLottery(): Promise<void> {
  showLottery.value = true
  lotteryError.value = ''
  lotteryLoading.value = true
  try {
    await lottery.loadStatus()
  } catch (e) {
    lotteryError.value = errorMessage(e)
  } finally {
    lotteryLoading.value = false
  }
}

function closeLottery(): void {
  showLottery.value = false
  lotteryError.value = ''
  lotteryBusy.value = false
}

async function runLottery(fn: () => Promise<unknown>): Promise<void> {
  lotteryBusy.value = true
  lotteryError.value = ''
  try {
    await fn()
    if (activeCid.value) {
      await city.refreshResources(activeCid.value)
    }
  } catch (e) {
    lotteryError.value = errorMessage(e)
  } finally {
    lotteryBusy.value = false
  }
}

function onLotteryStart(): void {
  void runLottery(() => lottery.startDraw())
}

function onLotteryClaimWin(): void {
  void runLottery(() => lottery.claimWin())
}

function onLotteryReward(): void {
  void runLottery(() => lottery.claimAndRestart())
}

function onLotteryAutoReward(): void {
  void runLottery(() => lottery.autoClaim())
}

function onLotteryRestart(winId: number, winType: number): void {
  void runLottery(() => lottery.restartDraw(winId, winType))
}

function onLotteryUseMoney(count: number): void {
  void runLottery(() => lottery.payMoney(count))
}

// 单机 PK 征战面板状态
const showPk = ref(false)
const pkLoading = ref(false)
const pkError = ref('')
const pkBusy = ref(false)

async function openPk(): Promise<void> {
  showPk.value = true
  pkError.value = ''
  pkLoading.value = true
  try {
    await Promise.all([pk.loadCampaign(), pk.refreshMax()])
  } catch (e) {
    pkError.value = errorMessage(e)
  } finally {
    pkLoading.value = false
  }
}

function closePk(): void {
  showPk.value = false
  pkError.value = ''
  pkBusy.value = false
}

async function runPk(fn: () => Promise<unknown>, refresh = false): Promise<void> {
  if (!activeCid.value) {
    return
  }
  pkBusy.value = true
  pkError.value = ''
  try {
    await fn()
    if (refresh) {
      await city.refreshResources(activeCid.value)
    }
  } catch (e) {
    pkError.value = errorMessage(e)
  } finally {
    pkBusy.value = false
  }
}

function onPkLoadRank(battleId: number, t: number): void {
  void runPk(() => pk.loadRank(battleId, t))
}

function onPkFirstReward(battleId: number, flag: number, rankId: number): void {
  void runPk(() => pk.claimFirst(battleId, flag, rankId), true)
}

function onPkFight(battleId: number, flag: number, level: number, hids: number[]): void {
  void runPk(() => pk.fight(battleId, flag, level, hids), true)
}

function onPkBuy(t: number, count: number): void {
  void runPk(() => pk.buy(t, count), true)
}

function onPkLoadHeroes(page: number, hids: number[]): void {
  void runPk(() => pk.loadHeroes(page, hids))
}

function onPkClearBattle(): void {
  pk.clearBattle()
}

// 内政面板状态（税率 / 征收 / 安抚 / 产出比例）
const showCivil = ref(false)
const civilLoading = ref(false)
const civilError = ref('')
const civilNotice = ref('')
const civilBusy = ref(false)

async function openCivil(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showCivil.value = true
  civilError.value = ''
  civilNotice.value = ''
  civilLoading.value = true
  try {
    await city.loadProduct(activeCid.value)
  } catch (e) {
    civilError.value = errorMessage(e)
  } finally {
    civilLoading.value = false
  }
}

function closeCivil(): void {
  showCivil.value = false
  civilError.value = ''
  civilNotice.value = ''
  civilBusy.value = false
}

// 内政写操作统一包裹：成功文案由 fn 返回（征收/安抚取自后端 message），并刷新资源。
async function runCivil(fn: () => Promise<string | void>): Promise<void> {
  if (!activeCid.value) {
    return
  }
  civilBusy.value = true
  civilError.value = ''
  civilNotice.value = ''
  try {
    const message = await fn()
    civilNotice.value = message ?? ''
    await city.refreshResources(activeCid.value)
  } catch (e) {
    civilError.value = errorMessage(e)
  } finally {
    civilBusy.value = false
  }
}

function onCivilSetTax(tax: number): void {
  void runCivil(async () => {
    await city.setTax(activeCid.value, tax)
    return `税率已更新为 ${tax}%。`
  })
}

function onCivilLevy(resid: number): void {
  void runCivil(async () => (await city.levy(activeCid.value, resid)).message)
}

function onCivilPacify(action: number): void {
  void runCivil(async () => (await city.pacify(activeCid.value, action)).message)
}

function onCivilSetProductRate(payload: ProductRatePayload): void {
  void runCivil(async () => {
    await city.setProductRate(activeCid.value, payload)
    return '产出比例已保存。'
  })
}

// 外城 / 城防面板状态
const showOutCity = ref(false)
const outCityLoading = ref(false)
const outCityError = ref('')

async function openOutCity(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showOutCity.value = true
  outCityError.value = ''
  outCityLoading.value = true
  try {
    await Promise.all([city.loadTroops(activeCid.value), city.loadDefences(activeCid.value)])
  } catch (e) {
    outCityError.value = errorMessage(e)
  } finally {
    outCityLoading.value = false
  }
}

function closeOutCity(): void {
  showOutCity.value = false
  outCityError.value = ''
}

// 世界地图面板状态（M11）。写操作的成功提示可能经 HTTP 400 message 返回 → 统一 errorMessage 展示。
const showWorld = ref(false)
const worldLoading = ref(false)
const worldError = ref('')
const worldNotice = ref('')
const worldBusy = ref(false)

async function openWorld(): Promise<void> {
  if (!activeCid.value) {
    return
  }
  showWorld.value = true
  worldError.value = ''
  worldNotice.value = ''
  worldLoading.value = true
  try {
    // 视野定位到我方城池，并预取收藏/领地/出征兵力。
    await world.centerOnWid(cidToWid(activeCid.value))
    await Promise.all([world.loadFavourites(), world.loadMyFields(), city.loadArmyInfo(activeCid.value)])
  } catch (e) {
    worldError.value = errorMessage(e)
  } finally {
    worldLoading.value = false
  }
}

function closeWorld(): void {
  showWorld.value = false
  worldError.value = ''
  worldNotice.value = ''
  worldBusy.value = false
}

// 读操作（选中/刷新/定位）：错误进 worldError。
async function runWorldLoad(fn: () => Promise<unknown>): Promise<void> {
  worldLoading.value = true
  worldError.value = ''
  try {
    await fn()
  } catch (e) {
    worldError.value = errorMessage(e)
  } finally {
    worldLoading.value = false
  }
}

// 写操作：成功/失败文案均可能经 HTTP 400 message 返回 → 一律 errorMessage 照实展示。
async function runWorldWrite(fn: () => Promise<string | void>, after?: () => Promise<void>): Promise<void> {
  if (!activeCid.value) {
    return
  }
  worldBusy.value = true
  worldError.value = ''
  worldNotice.value = ''
  try {
    const message = await fn()
    worldNotice.value = message ?? ''
  } catch (e) {
    worldNotice.value = errorMessage(e)
  } finally {
    worldBusy.value = false
  }
  if (after) {
    try {
      await after()
    } catch (e) {
      worldError.value = errorMessage(e)
    }
  }
}

function onWorldView(x: number, y: number): void {
  world.moveView(x, y).catch((e) => {
    worldError.value = errorMessage(e)
  })
}

function onWorldSelect(wid: number): void {
  void runWorldLoad(() => world.selectCell(wid, activeCid.value))
}

function onWorldRefresh(): void {
  void runWorldLoad(async () => {
    await world.ensureArea(world.view.x, world.view.y, world.cols, world.rows)
    await world.reloadSelected(activeCid.value)
    await world.loadFavourites()
    await world.loadMyFields()
  })
}

function onWorldBuild(wid: number): void {
  void runWorldWrite(
    async () => {
      await world.buildCity(wid)
      return '筑城指令已提交。'
    },
    async () => {
      await world.reloadSelected(activeCid.value)
      await world.loadMyFields()
      if (activeCid.value) {
        await city.refreshResources(activeCid.value)
      }
    },
  )
}

function onWorldMark(cid: number): void {
  void runWorldWrite(
    async () => {
      await world.mark(cid)
      return '标记指令已提交。'
    },
    () => world.ensureArea(world.view.x, world.view.y, world.cols, world.rows),
  )
}

function onWorldWar(uid: number, cid: number): void {
  void runWorldWrite(
    async () => {
      await world.war(uid, cid)
      return '宣战请求已提交。'
    },
    () => world.reloadSelected(activeCid.value),
  )
}

function onWorldFavourite(cid: number): void {
  void runWorldWrite(() => world.addFavourite(cid), () => world.loadFavourites())
}

function onWorldUnfavourite(id: number): void {
  void runWorldWrite(() => world.removeFavourite(id))
}

function onWorldFavComment(id: number, comments: string): void {
  void runWorldWrite(() => world.setFavComments(id, comments), () => world.loadFavourites())
}

function onWorldGovern(payload: GovernPayload): void {
  void runWorldWrite(
    () => world.govern(payload),
    async () => {
      await world.reloadSelected(activeCid.value)
      if (activeCid.value) {
        await city.refreshResources(activeCid.value)
      }
    },
  )
}

// 出征复用 army dispatch（target_type=2/城池，task 由地图侧任务按钮决定）；成功走 HTTP 200。
async function onWorldDispatch(payload: DispatchPayload): Promise<void> {
  if (!activeCid.value) {
    return
  }
  worldBusy.value = true
  worldError.value = ''
  worldNotice.value = ''
  try {
    await city.dispatch(activeCid.value, payload)
    worldNotice.value = '出征部队已出发。'
  } catch (e) {
    worldError.value = errorMessage(e)
  } finally {
    worldBusy.value = false
  }
}

function onWorldGotoWid(wid: number): void {
  void runWorldLoad(async () => {
    await world.centerOnWid(wid)
    await world.selectCell(wid, activeCid.value)
  })
}

function onWorldGotoCid(cid: number): void {
  const wid = cidToWid(cid)
  void runWorldLoad(async () => {
    await world.centerOnWid(wid)
    await world.selectCell(wid, activeCid.value)
  })
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
  closeEconomy()
  closeBattle()
  closeTasks()
  closeAchievements()
  closeLottery()
  closePk()
  closeCivil()
  closeOutCity()
  closeWorld()
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
        <button class="nav-btn innercity" type="button" title="内政" @click="openCivil"></button>
        <button class="nav-btn outercity" type="button" title="外城" @click="openOutCity"></button>
        <button class="nav-btn map" type="button" title="地图" @click="openWorld"></button>
        <button class="nav-tech" type="button" @click="openTechnics">{{ zhCN.technic.open }}</button>
        <button class="nav-btn army" type="button" title="军事" @click="openArmy"></button>
        <button class="nav-btn hero" type="button" title="武将" @click="openHeroes"></button>
        <button class="nav-armor" type="button" @click="openArmor">{{ zhCN.armor.open }}</button>
        <button class="nav-btn mission" type="button" title="任务" @click="openTasks"></button>
        <button class="nav-btn battle" type="button" title="征战" @click="openPk"></button>
        <button class="nav-btn economy" type="button" title="经济" @click="openEconomy"></button>
        <button class="nav-btn achievement" type="button" title="成就" @click="openAchievements"></button>
        <button class="nav-btn lottery" type="button" title="幸运宝匣" @click="openLottery"></button>
        <button class="nav-btn report" type="button" title="战斗战报" @click="openBattle"></button>
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

    <MarketPanel
      v-if="showEconomy"
      :cid="activeCid"
      :resources="city.resources"
      :market="economy.market"
      :merchant="economy.merchant"
      :buy-list="economy.buyList"
      :sell-data="economy.sellData"
      :auto-trans="economy.autoTrans"
      :auto-trans-has="economy.autoTransHas"
      :store="economy.store"
      :workshop="economy.workshop"
      :shop="economy.shop"
      :goods="economy.goods"
      :gold="city.resources?.gold ?? 0"
      :loading="economyLoading"
      :error="economyError"
      :busy="economyBusy"
      @load-merchant="onEconomyLoadMerchant"
      @buy-merchant="onEconomyBuyMerchant"
      @sell-merchant="onEconomySellMerchant"
      @sell-user="onEconomySellUser"
      @buy-user="onEconomyBuyUser"
      @cancel-sell="onEconomyCancelSell"
      @accelerate="onEconomyAccelerate"
      @load-buylist="onEconomyLoadBuyList"
      @add-autotrans="onEconomyAddAutoTrans"
      @remove-autotrans="onEconomyRemoveAutoTrans"
      @cancel-autotrans="onEconomyCancelAutoTrans"
      @store-rate="onEconomyStoreRate"
      @store-pack="onEconomyStorePack"
      @workshop-refresh="onEconomyWorkshopRefresh"
      @workshop-buy="onEconomyWorkshopBuy"
      @shop-buy="onEconomyShopBuy"
      @shop-exchange="onEconomyShopExchange"
      @shop-armor-attr="onEconomyShopArmorAttr"
      @shop-hero-attr="onEconomyShopHeroAttr"
      @sell-goods="onEconomySellGoods"
      @close="closeEconomy"
    />

    <BattlePanel
      v-if="showBattle"
      :battles="battle.list"
      :detail="battle.detail"
      :reports="battle.reports"
      :selected-id="battle.selectedId"
      :loading="battleLoading"
      :error="battleError"
      :busy="battleBusy"
      @select="onBattleSelect"
      @load-reports="onBattleLoadReports"
      @close="closeBattle"
    />

    <TaskPanel
      v-if="showTasks"
      :groups="task.groups"
      :tasks="task.tasks"
      :detail="task.detail"
      :active-type="task.activeType"
      :active-group="task.activeGroup"
      :detail-tid="task.detailTid"
      :sys-count="task.sysCount"
      :nobility-ok="task.nobilityOk"
      :done-count="task.doneCount"
      :loading="taskLoading"
      :error="taskError"
      :busy="taskBusy"
      @select-type="onTaskSelectType"
      @select-group="onTaskSelectGroup"
      @open-detail="onTaskOpenDetail"
      @back="onTaskBack"
      @claim="onTaskClaim"
      @drop="onTaskDrop"
      @sys-drop="onTaskSysDrop"
      @close="closeTasks"
    />

    <AchievementPanel
      v-if="showAchievements"
      :overview="achievement.overview"
      :list="achievement.list"
      :count="achievement.count"
      :detail="achievement.detail"
      :active-group="achievement.activeGroup"
      :filter="achievement.filter"
      :page="achievement.page"
      :loading="achievementLoading"
      :error="achievementError"
      :busy="achievementBusy"
      @select-group="onAchievementSelectGroup"
      @select-filter="onAchievementSelectFilter"
      @select-page="onAchievementSelectPage"
      @open-detail="onAchievementOpenDetail"
      @back="onAchievementBack"
      @close="closeAchievements"
    />

    <LotteryPanel
      v-if="showLottery"
      :today-count="lottery.todayCount"
      :money-ok="lottery.moneyOk"
      :board="lottery.board"
      :win-info="lottery.winInfo"
      :restart-count="lottery.restartCount"
      :last-win="lottery.lastWin"
      :loading="lotteryLoading"
      :error="lotteryError"
      :busy="lotteryBusy"
      @start="onLotteryStart"
      @claim-win="onLotteryClaimWin"
      @reward="onLotteryReward"
      @auto-reward="onLotteryAutoReward"
      @restart="onLotteryRestart"
      @use-money="onLotteryUseMoney"
      @close="closeLottery"
    />

    <PkPanel
      v-if="showPk"
      :uid="auth.user?.uid ?? 0"
      :campaign="pk.campaign"
      :max="pk.max"
      :rank="pk.rank"
      :hero-page="pk.heroPage"
      :battle="pk.battle"
      :loading="pkLoading"
      :error="pkError"
      :busy="pkBusy"
      @load-rank="onPkLoadRank"
      @first-reward="onPkFirstReward"
      @fight="onPkFight"
      @buy="onPkBuy"
      @load-heroes="onPkLoadHeroes"
      @clear-battle="onPkClearBattle"
      @close="closePk"
    />

    <CivilPanel
      v-if="showCivil"
      :product="city.product"
      :resource="city.resources"
      :loading="civilLoading"
      :error="civilError"
      :notice="civilNotice"
      :busy="civilBusy"
      @set-tax="onCivilSetTax"
      @levy="onCivilLevy"
      @pacify="onCivilPacify"
      @set-product-rate="onCivilSetProductRate"
      @close="closeCivil"
    />

    <OutCityPanel
      v-if="showOutCity"
      :troops="city.troops"
      :defences="city.defences"
      :loading="outCityLoading"
      :error="outCityError"
      @close="closeOutCity"
    />

    <WorldMapPanel
      v-if="showWorld"
      :cells="world.cells"
      :view-x="world.view.x"
      :view-y="world.view.y"
      :cols="world.cols"
      :rows="world.rows"
      :marks="world.marks"
      :selected-wid="world.selectedWid"
      :city="world.selectedCity"
      :field="world.selectedField"
      :invade="world.invade"
      :govern-info="world.governInfo"
      :favourites="world.favourites"
      :user-fields="world.userFields"
      :action-fields="world.actionFields"
      :self-uid="auth.user?.uid ?? 0"
      :my-cid="activeCid"
      :heroes="idleHeroes"
      :soldiers="city.armyInfo?.soldiers ?? []"
      :loading="worldLoading"
      :busy="worldBusy"
      :error="worldError"
      :notice="worldNotice"
      @close="closeWorld"
      @view="onWorldView"
      @select="onWorldSelect"
      @refresh="onWorldRefresh"
      @build-city="onWorldBuild"
      @mark="onWorldMark"
      @war="onWorldWar"
      @favourite="onWorldFavourite"
      @unfavourite="onWorldUnfavourite"
      @fav-comment="onWorldFavComment"
      @govern="onWorldGovern"
      @dispatch="onWorldDispatch"
      @goto-wid="onWorldGotoWid"
      @goto-cid="onWorldGotoCid"
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

/* 任务：原版 topbutton_mission 贴图（自带文字，61×26）。 */
.nav-btn.mission {
  background-image: url('/images/topbutton_mission.png');
}
.nav-btn.mission:hover:not(:active) {
  background-image: url('/images/topbutton_mission_on.png');
}
.nav-btn.mission:active {
  background-image: url('/images/topbutton_mission_down.png');
}

/* 征战（PK）：原版 topbutton_battle 贴图（76×29，无 _down，仅 over/on）。 */
.nav-btn.battle {
  width: 76px;
  height: 29px;
  background-image: url('/images/topbutton_battle.png');
}
.nav-btn.battle:hover:not(:active) {
  background-image: url('/images/topbutton_battle_on.png');
}

/* 内政 / 外城：原版 topbutton_innercity / topbutton_outercity 贴图（76×29，三态）。 */
.nav-btn.innercity {
  width: 76px;
  height: 29px;
  background-image: url('/images/topbutton_innercity.png');
}
.nav-btn.innercity:hover:not(:active) {
  background-image: url('/images/topbutton_innercity_on.png');
}
.nav-btn.innercity:active {
  background-image: url('/images/topbutton_innercity_down.png');
}

.nav-btn.outercity {
  width: 76px;
  height: 29px;
  background-image: url('/images/topbutton_outercity.png');
}
.nav-btn.outercity:hover:not(:active) {
  background-image: url('/images/topbutton_outercity_on.png');
}
.nav-btn.outercity:active {
  background-image: url('/images/topbutton_outercity_down.png');
}

/* 地图：原版 topbutton_map 贴图（76×29，三态）。 */
.nav-btn.map {
  width: 76px;
  height: 29px;
  background-image: url('/images/topbutton_map.png');
}
.nav-btn.map:hover:not(:active) {
  background-image: url('/images/topbutton_map_over.png');
}
.nav-btn.map:active {
  background-image: url('/images/topbutton_map_down.png');
}

/* 经济 / 成就 / 幸运宝匣：原版功能按钮贴图（function_*、lottery_* 三态）。 */
.nav-btn.economy {
  width: 40px;
  height: 25px;
  background-image: url('/images/function_shop.png');
}
.nav-btn.economy:hover:not(:active) {
  background-image: url('/images/function_shop_on.png');
}
.nav-btn.economy:active {
  background-image: url('/images/function_shop_down.png');
}

.nav-btn.achievement {
  width: 40px;
  height: 25px;
  background-image: url('/images/function_achievement.png');
}
.nav-btn.achievement:hover:not(:active) {
  background-image: url('/images/function_achievement_on.png');
}
.nav-btn.achievement:active {
  background-image: url('/images/function_achievement_down.png');
}

.nav-btn.lottery {
  width: 29px;
  height: 25px;
  background-image: url('/images/lottery_box_btn.png');
}
.nav-btn.lottery:hover:not(:active) {
  background-image: url('/images/lottery_box_btn_on.png');
}
.nav-btn.lottery:active {
  background-image: url('/images/lottery_box_btn_down.png');
}

/* 战报：原版 topbutton_report 贴图（源图 116×45，按 88×34 适配顶栏，保持比例与可读性）。
   红/蓝变体（_red/_blue）需未读战报数据源，当前无对应字段，故只用常态。 */
.nav-btn.report {
  width: 88px;
  height: 34px;
  background-image: url('/images/topbutton_report_up.png');
}
.nav-btn.report:hover:not(:active) {
  background-image: url('/images/topbutton_report_over.png');
}
.nav-btn.report:active {
  background-image: url('/images/topbutton_report_down.png');
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

/* 装备：同科技按钮样式（原版装备入口在武将面板内，此处为独立入口）。 */
.nav-armor,
.nav-text {
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

.nav-armor:hover,
.nav-text:hover {
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