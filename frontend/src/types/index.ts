// 镜像 Go 后端 internal/model 的 DTO（JSON tag 一一对应）。

export interface User {
  uid: number
  passport: string
  passtype: string
  name: string
  group: number
  state: number
  money: number
  honour: number
  nobility: string
  lastcid: number
  regtime: number
  officepos: number
  union_id: number
  rank: number
}

export interface City {
  cid: number
  uid: number
  name: string
  isSpecial: number
  spacialSid: number
}

export interface CityResource {
  cid: number
  wood: number
  woodAdd: number
  woodMax: number
  rock: number
  rockAdd: number
  rockMax: number
  iron: number
  ironAdd: number
  ironMax: number
  food: number
  foodAdd: number
  foodMax: number
  foodArmyUse: number
  gold: number
  goldRate: number
  goldMax: number
  people: number
  peopleMax: number
  peopleStable: number
  peopleWorking: number
  peopleBuilding: number
  morale: number
  tax: number
  complaint: number
  heroFee: number
  vacation: number
  forbidden: number
}

export interface Building {
  cid: number
  bid: number
  x: number
  y: number
  level: number
  state: number
  state_starttime: number
  state_endtime: number
  state_timeleft: number
  bname: string
  buildingDescription: string
  level_description: string
  using_people: number
}

// 单个建筑的下一级升级信息（对齐后端 building.UpgradeInfo）。
export interface UpgradeInfo {
  bid: number
  name: string
  level: number
  level_description: string
  woodNeed: number
  rockNeed: number
  ironNeed: number
  foodNeed: number
  goldNeed: number
  peopleNeed: number
  upgradeTime: number
  canUpgrade: boolean
  no_upgrade_msg: string
}

// 单建筑详情（对齐后端 building.Detail）。
export interface BuildingDetail {
  bid: number
  name: string
  x: number
  y: number
  level: number
  state: number
  state_endtime: number
  state_timeleft: number
  next: UpgradeInfo | null
}

export interface Technic {
  tid: number
  level: number
}

// 单个科技的完整状态（对齐后端 technic.Item，字段沿用 legacy TechnicState 命名）。
export interface TechnicState {
  tid: number
  tname: string
  cid: number
  description: string
  level: number
  sharelevel: number
  state: number
  state_endtime: number
  state_timeleft: number
  can_upgrade: boolean
  no_upgrade_msg: string
  levelDescription: string
  nextLevelDescription: string
  woodNeed: number
  rockNeed: number
  ironNeed: number
  foodNeed: number
  goldNeed: number
  upgrade_time: number
}

export interface TechnicInfo {
  technics: TechnicState[]
  collegeCount: number
}

export interface CitySoldier {
  cid: number
  sid: number
  count: number
}

export interface CityDefence {
  cid: number
  did: number
  count: number
}

export interface Hero {
  hid: number
  uid: number
  cid: number
  name: string
  sex: number
  face: number
  state: number
  level: number
  herotype: number
  command_base: number
  affairs_base: number
  bravery_base: number
  wisdom_base: number
  loyalty: number
  force: number
  force_max: number
  energy: number
  energy_max: number
  curCid: number
}

// 武将面板（对齐后端 hero 包 DTO，state 采用 legacy 值集 {0,1,4,7,8,10,11}）。
export interface HeroExpr {
  id: number
  expr_type: number
  expr_name: string
  hours: number
  state: number
  started_at: number
  end_at: number
  time_left: number
  carrymoney: number
  acc_times: number
}

export interface HeroExprType {
  type: number
  name: string
  min_hour: number
  max_hour: number
  hour_money: number
  hour_gold: number
}

export interface HeroState {
  hid: number
  name: string
  sex: number
  face: number
  hero_type: number
  npc_id: number
  level: number
  exp: number
  state: number
  loyalty: number
  hero_health: number
  command_base: number
  command_add_on: number
  bravery_base: number
  bravery_add: number
  bravery_add_on: number
  wisdom_base: number
  wisdom_add: number
  wisdom_add_on: number
  affairs_base: number
  affairs_add: number
  affairs_add_on: number
  attack_base: number
  attack_add_on: number
  defence_base: number
  defence_add_on: number
  level_total_exp: number
  upgrade_exp: number
  need_exp: number
  can_upgrade: boolean
  no_upgrade_msg: string
  expr: HeroExpr | null
}

export interface HeroInfo {
  heroes: HeroState[]
  exprTypes: HeroExprType[]
  toomany?: string
}

export interface Province {
  province: string
  jun: string
}

export interface Alarm {
  uid: number
  task: number
  mail: number
}

// 军事：兵营征兵与部队出征（对齐后端 army 包 DTO）。
export interface DraftSoldier {
  sid: number
  sname: string
  count: number
  hp: number
  ap: number
  dp: number
  speed: number
  woodNeed: number
  rockNeed: number
  ironNeed: number
  foodNeed: number
  goldNeed: number
  peopleNeed: number
  draft_time: number
  can_draft: boolean
  no_draft_msg: string
}

export interface ArmyQueue {
  qid: number
  sid: number
  sname: string
  count: number
  state: number
  time_left: number
}

export interface ArmyInfo {
  x: number
  y: number
  barracksLevel: number
  soldiers: DraftSoldier[]
  queues: ArmyQueue[]
  people: number
  people_max: number
}

export interface March {
  id: number
  hero_id: number
  hero_name: string
  target_type: number
  target_id: number
  target_name: string
  task: number
  state: number
  soldiers: Record<string, number>
  start_at: number
  arrive_at: number
  back_at: number
  time_left: number
}

export interface Field {
  id: number
  name: string
  level: number
  owner_uid: number
  guard_power: number
}

export interface DispatchPayload {
  hero_id: number
  target_type: number
  target_id: number
  task: number
  soldiers: Record<string, number>
}

export interface BaseInfo {
  user: User
  resource: CityResource
  heroes: Hero[]
  troops: CitySoldier[]
  defences: CityDefence[]
  alarm: Alarm
  openLottery: number
}

// 客栈招募池（对齐后端 tavern 包 DTO；sex 沿用 legacy：0=女 1=男）。
export interface Recruit {
  id: number
  name: string
  sex: number
  face: number
  level: number
  affairs_base: number
  bravery_base: number
  wisdom_base: number
  command_base: number
  affairs_add: number
  bravery_add: number
  wisdom_add: number
  command_add: number
  loyalty: number
  gold_need: number
  hero_type: number
  is_act_hero: boolean
}

export interface HotelInfo {
  hotel_level: number
  hotel_name: string
  office_pos: number
  can_jiejiao: boolean
  recruits: Recruit[]
  tip?: string
}

export interface CityDetail {
  city: City
  base: BaseInfo
  buildings: Building[]
  technics: Technic[]
  province: Province
}

export interface LoginResponse {
  token: string
  expiresIn: number
  user: User
}

export interface LoginAnnouncement {
  content: string
}

export interface APIError {
  code: string
  message: string
}

// —— 装备（对齐后端 armor 包 DTO）——

export interface BagArmor {
  sid: number
  armorid: number
  name: string
  part: number
  type: number
  hero_level: number
  value: number
  hp: number
  hp_max: number
  ori_hp_max: number
  hid: number
  strong_level: number
  strong_value: number
  strong_times: number
  combine_level: number
  embed_holes: string
  embed_pearls: string
  best_quality: string
  deified: number
  active_special: number
  attribute: string
  tieid: number
  description: string
}

export interface HeroArmorItem {
  sid: number
  spart: number
  armorid: number
  name: string
  part: number
  type: number
}

export interface EquipResult {
  hid: number
  command_add_on: number
  affairs_add_on: number
  bravery_add_on: number
  wisdom_add_on: number
  speed_add_on: number
  attack_add_on: number
  defence_add_on: number
  armors: HeroArmorItem[]
}

export interface StrongResult {
  started: boolean
  outcome: number // 0成功 1归零 2降级 3完好/无损
  strong_value: number
  end_level: number
  best_quality: string
  msg: string
}

export interface CombineResult {
  success: number
  goods_flag: number
  new_combine_level: number
}

export interface EmbedResult {
  started: number
  msg: string
  pearls: string
}