<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { img, itemIcon } from '@/assets/img'
import type { UserGood } from '@/api/economy'
import type { BagArmor, HeroArmorItem, HeroState } from '@/types'
import type { StrongPayload, SynthPayload } from '@/api/armor'

const props = defineProps<{
  bag: BagArmor[]
  heroes: HeroState[]
  heroArmors: Record<number, HeroArmorItem[]>
  cid: number
  gold: number
  /** 背包道具（材料升级页签的目标宝石来源；GET /goods）。 */
  goods: UserGood[]
  loading: boolean
  error: string
  notice: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'equip', hid: number, sid: number, spart: number): void
  (e: 'offload', hid: number, spart: number): void
  (e: 'repair', sid: number): void
  (e: 'renovate', sid: number): void
  (e: 'sell', sid: number): void
  (e: 'chaijie', sid: number): void
  (e: 'strong', p: StrongPayload): void
  (e: 'combine', mainSid: number, sub1: number, sub2: number, goodsFlag: number): void
  (e: 'init-holes', sid: number): void
  (e: 'open-hole', sid: number, gid: number, pos: number, useType: number, count: number): void
  (e: 'embed', sid: number, pos: number, gid: number): void
  (e: 'synth', p: SynthPayload): void
  (e: 'load-hero', hid: number): void
}>()

const tab = ref<'bag' | 'wear' | 'strong' | 'combine' | 'hole' | 'synth'>('bag')
const tabs = [
  ['bag', zhCN.armor.tabBag],
  ['wear', zhCN.armor.tabWear],
  ['strong', zhCN.armor.tabStrong],
  ['combine', zhCN.armor.tabCombine],
  ['hole', zhCN.armor.tabHole],
  ['synth', zhCN.armor.tabSynth],
] as const

const selectedSid = ref(0)
const selected = computed(() => props.bag.find((a) => a.sid === selectedSid.value) ?? null)

// 品质色（cfg_armor.type 1灰…7红）
const typeColors = ['', '#9aa0a6', '#f2f2f2', '#67c23a', '#409eff', '#b37efc', '#e6a23c', '#f56c6c']
function typeColor(t: number): string {
  return typeColors[t] ?? '#fff'
}

// 穿戴：spart = part*10（legacy 客户端按部位传值）
const equipHid = ref(0)
const inCityHeroes = computed(() =>
  props.heroes.filter((h) => h.state === 0 || h.state === 1 || h.state === 7 || h.state === 8),
)

function doEquip(): void {
  const a = selected.value
  if (!a || equipHid.value <= 0) {
    return
  }
  emit('equip', equipHid.value, a.sid, a.part * 10)
}

// 武将穿戴页
const wearHid = ref(0)
const wearList = computed(() => props.heroArmors[wearHid.value] ?? [])
function onWearHidChange(): void {
  if (wearHid.value > 0) {
    emit('load-hero', wearHid.value)
  }
}

// 强化表单（203天工符/204乾坤宝珠可选带）
const strongForm = reactive({ good1: 0, good2: 0 })
function doStrong(): void {
  const a = selected.value
  if (!a) {
    return
  }
  emit('strong', {
    cid: props.cid,
    gid1: 203,
    good1_count: strongForm.good1,
    gid2: 204,
    good2_count: strongForm.good2,
    sid: a.sid,
    is_zuoji: a.part === 12 ? 1 : 0,
  })
}

// 熔炼：主件 + 两个同 armorid 同 combine_level 副件
const combineSub1 = ref(0)
const combineSub2 = ref(0)
const combineGoods = ref(0)
const subCandidates = computed(() => {
  const m = selected.value
  if (!m) {
    return []
  }
  return props.bag.filter(
    (a) =>
      a.sid !== m.sid &&
      a.armorid === m.armorid &&
      a.combine_level === m.combine_level &&
      a.sid !== combineSub1.value &&
      a.sid !== combineSub2.value,
  )
})
function doCombine(): void {
  const m = selected.value
  if (!m || combineSub1.value <= 0 || combineSub2.value <= 0) {
    return
  }
  emit('combine', m.sid, combineSub1.value, combineSub2.value, combineGoods.value)
}

// 打孔/镶嵌
const holeForm = reactive<{ gid: number; pos: number; useType: number; count: number; embedGid: number }>({
  gid: 201,
  pos: 0,
  useType: 0,
  count: 1,
  embedGid: 300,
})
const holes = computed(() => (selected.value?.embed_holes ?? '').split(',').filter((s) => s !== ''))
const pearls = computed(() => (selected.value?.embed_pearls ?? '').split(',').filter((s) => s !== ''))

// 孔位占位图（仅视觉占位；沿用既有 embed_holes/embed_pearls 数据，不改语义）。
//   已镶嵌宝珠 → item_default_embedPearl（embeds 无宝珠等级字段，先回退；有等级时可用 embed_pearl_bg{n}）
//   已开孔(h==='0') → item_default_embedArmorPearl_open
//   锁定(h==='1'/'2'/'4') → item_default_embedArmorPearl_lock / _lock2 / _lock3（对应初/高/特级打孔器，参照 BlackSmith/BSCommandDialog.as set5EmbedPearls）
//   不开(h==='3') 及其他 → item_default_embedArmorPearl
// 说明：这批 embed_*/item_default_* 在 tools/asset-index.json 中 embed 族 referencedByScript=false。
function holeImage(i: number): string {
  const pearl = pearls.value[i]
  if (pearl && pearl !== '0') {
    return img('item_default_embedPearl.png')
  }
  switch (holes.value[i]) {
    case '0':
      return img('item_default_embedArmorPearl_open.png')
    case '1':
      return img('item_default_embedArmorPearl_lock.png')
    case '2':
      return img('item_default_embedArmorPearl_lock2.png')
    case '4':
      return img('item_default_embedArmorPearl_lock3.png')
    default:
      return img('item_default_embedArmorPearl.png')
  }
}

function selectArmor(a: BagArmor): void {
  selectedSid.value = a.sid
}

// 材料升级 / 宝石合成（原版 BSCommandDialog.synthStuff；页签 3）
// 目标宝石取背包中已有的宝珠/宝石（gid 区间对齐后端 armor/embed.go getDismantleCount：
//   前 10 级镶嵌宝珠 300-379、聚魂珠 10830-10839、后 5 级 17500-17539）。
function isGem(gid: number): boolean {
  return (gid >= 300 && gid <= 379) || (gid >= 10830 && gid <= 10839) || (gid >= 17500 && gid <= 17539)
}
const gems = computed(() => props.goods.filter((g) => isGem(g.gid)))

const synthForm = reactive<{ gid: number; count: number; dj: number; pearlProtect: number }>({
  gid: 0,
  count: 1,
  dj: 0,
  pearlProtect: 0,
})

function doSynth(): void {
  if (synthForm.gid <= 0 || synthForm.count <= 0) {
    return
  }
  emit('synth', {
    cid: props.cid,
    gid: synthForm.gid,
    count: synthForm.count,
    dj: synthForm.dj,
    pearl_protect: synthForm.pearlProtect,
  })
}

// 宝石图标缺失回退（item_{gid}.png）。
const failedIcons = ref<Set<number>>(new Set())
function markIconFailed(gid: number): void {
  const next = new Set(failedIcons.value)
  next.add(gid)
  failedIcons.value = next
}
function iconOk(gid: number): boolean {
  return !failedIcons.value.has(gid)
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal u-modal">
      <header class="u-title">
        <strong>{{ zhCN.armor.title }}</strong>
        <span class="sub dim small">{{ zhCN.armor.gold }} {{ gold }}</span>
        <button class="u-close" title="关闭" @click="emit('close')"></button>
      </header>

      <nav class="tabs">
        <button
          v-for="[key, label] in tabs"
          :key="key"
          class="tab"
          :class="{ on: tab === key }"
          @click="tab = key"
        >
          {{ label }}
        </button>
      </nav>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>
      <p v-if="notice" class="hint">{{ notice }}</p>

      <!-- 背包 -->
      <template v-if="tab === 'bag'">
        <p v-if="!bag.length" class="hint">{{ zhCN.armor.empty }}</p>
        <ul v-else class="list">
          <li
            v-for="a in bag"
            :key="a.sid"
            class="item"
            :class="{ sel: a.sid === selectedSid }"
            @click="selectArmor(a)"
          >
            <div class="row">
              <strong :style="{ color: typeColor(a.type) }">{{ a.name }}</strong>
              <span class="state">
                {{ zhCN.armor.parts[a.part] ?? a.part }} ·
                {{ zhCN.armor.strongLv }}{{ a.strong_level }} ·
                {{ zhCN.armor.combineLv }}{{ a.combine_level }}
              </span>
              <span class="dim small">
                {{ zhCN.armor.dur }} {{ Math.ceil(a.hp / 10) }}/{{ a.hp_max }}
                {{ zhCN.armor.needLv }}{{ a.hero_level }}
              </span>
            </div>
          </li>
        </ul>
        <div v-if="selected" class="detail">
          <div class="row">
            <select v-model.number="equipHid">
              <option :value="0">{{ zhCN.armor.pickHero }}</option>
              <option v-for="h in inCityHeroes" :key="h.hid" :value="h.hid">
                {{ h.name }} Lv{{ h.level }}
              </option>
            </select>
            <button class="primary" :disabled="busy || equipHid <= 0" @click="doEquip">
              {{ zhCN.armor.equip }}
            </button>
            <button class="ghost" :disabled="busy" @click="emit('repair', selected.sid)">
              {{ zhCN.armor.repair }}
            </button>
            <button class="ghost" :disabled="busy" @click="emit('renovate', selected.sid)">
              {{ zhCN.armor.renovate }}
            </button>
            <button class="ghost" :disabled="busy" @click="emit('sell', selected.sid)">
              {{ zhCN.armor.sell }}
            </button>
            <button class="ghost" :disabled="busy" @click="emit('chaijie', selected.sid)">
              {{ zhCN.armor.chaijie }}
            </button>
          </div>
        </div>
      </template>

      <!-- 穿戴 -->
      <template v-else-if="tab === 'wear'">
        <div class="row">
          <select v-model.number="wearHid" @change="onWearHidChange">
            <option :value="0">{{ zhCN.armor.pickHero }}</option>
            <option v-for="h in inCityHeroes" :key="h.hid" :value="h.hid">
              {{ h.name }} Lv{{ h.level }}
            </option>
          </select>
        </div>
        <p v-if="wearHid <= 0" class="hint">{{ zhCN.armor.pickHero }}</p>
        <p v-else-if="!wearList.length" class="hint">{{ zhCN.armor.noWear }}</p>
        <ul v-else class="list">
          <li v-for="w in wearList" :key="w.spart" class="item">
            <div class="row">
              <strong :style="{ color: typeColor(w.type) }">{{ w.name }}</strong>
              <span class="dim small">{{ zhCN.armor.parts[w.part] ?? w.part }}</span>
              <button
                class="ghost"
                :disabled="busy"
                @click="emit('offload', wearHid, w.spart)"
              >
                {{ zhCN.armor.offload }}
              </button>
            </div>
          </li>
        </ul>
      </template>

      <!-- 强化 -->
      <template v-else-if="tab === 'strong'">
        <p v-if="!selected" class="hint">{{ zhCN.armor.pickArmor }}</p>
        <div v-else class="detail">
          <div class="row">
            <strong :style="{ color: typeColor(selected.type) }">{{ selected.name }}</strong>
            <span class="state">{{ zhCN.armor.strongLv }}{{ selected.strong_level }}</span>
            <span class="dim small">{{ zhCN.armor.strongVal }}{{ selected.strong_value }}</span>
          </div>
          <div class="row">
            <label class="dim small">
              <input v-model.number="strongForm.good1" type="checkbox" :true-value="1" :false-value="0" />
              {{ zhCN.armor.useTianGong }}
            </label>
            <label class="dim small">
              <input v-model.number="strongForm.good2" type="checkbox" :true-value="1" :false-value="0" />
              {{ zhCN.armor.useQianKun }}
            </label>
            <button class="primary" :disabled="busy" @click="doStrong">{{ zhCN.armor.strong }}</button>
          </div>
        </div>
        <ul class="list mini">
          <li v-for="a in bag" :key="a.sid" class="item row" @click="selectArmor(a)">
            <span :style="{ color: typeColor(a.type) }">{{ a.name }}</span>
            <span class="dim small">+{{ a.strong_level }}</span>
          </li>
        </ul>
      </template>

      <!-- 熔炼 -->
      <template v-else-if="tab === 'combine'">
        <p v-if="!selected" class="hint">{{ zhCN.armor.pickArmor }}</p>
        <div v-else class="detail">
          <div class="row">
            <strong :style="{ color: typeColor(selected.type) }">{{ selected.name }}</strong>
            <span class="state">{{ zhCN.armor.combineLv }}{{ selected.combine_level }}</span>
          </div>
          <div class="row">
            <select v-model.number="combineSub1">
              <option :value="0">{{ zhCN.armor.pickSub }}</option>
              <option v-for="c in subCandidates" :key="c.sid" :value="c.sid">
                {{ c.name }}（{{ zhCN.armor.sid }}{{ c.sid }}）
              </option>
            </select>
            <select v-model.number="combineSub2">
              <option :value="0">{{ zhCN.armor.pickSub }}</option>
              <option v-for="c in subCandidates" :key="c.sid" :value="c.sid">
                {{ c.name }}（{{ zhCN.armor.sid }}{{ c.sid }}）
              </option>
            </select>
            <label class="dim small">
              <input v-model.number="combineGoods" type="checkbox" :true-value="1" :false-value="0" />
              {{ zhCN.armor.useProtect }}
            </label>
            <button
              class="primary"
              :disabled="busy || combineSub1 <= 0 || combineSub2 <= 0"
              @click="doCombine"
            >
              {{ zhCN.armor.combine }}
            </button>
          </div>
        </div>
        <ul class="list mini">
          <li v-for="a in bag" :key="a.sid" class="item row" @click="selectArmor(a)">
            <span :style="{ color: typeColor(a.type) }">{{ a.name }}</span>
            <span class="dim small">{{ zhCN.armor.combineLv }}{{ a.combine_level }}</span>
          </li>
        </ul>
      </template>

      <!-- 打孔 / 镶嵌 -->
      <template v-else-if="tab === 'hole'">
        <p v-if="!selected" class="hint">{{ zhCN.armor.pickArmor }}</p>
        <div v-else class="detail">
          <div class="row">
            <strong :style="{ color: typeColor(selected.type) }">{{ selected.name }}</strong>
            <button v-if="!selected.embed_holes" class="primary" :disabled="busy" @click="emit('init-holes', selected.sid)">
              {{ zhCN.armor.activateHoles }}
            </button>
          </div>
          <div v-if="holes.length" class="row holes">
            <span
              v-for="(h, i) in holes"
              :key="i"
              class="hole"
              :class="{ open: h === '0' }"
              :title="pearls[i] && pearls[i] !== '0' ? pearls[i] : h === '0' ? '◇' : '◆' + h"
            >
              <img class="hole-img" :src="holeImage(i)" alt="" />
            </span>
          </div>
          <div class="row">
            <input v-model.number="holeForm.pos" type="number" min="0" class="num" :placeholder="zhCN.armor.pos" />
            <input v-model.number="holeForm.gid" type="number" class="num" :placeholder="zhCN.armor.gid" />
            <select v-model.number="holeForm.useType">
              <option :value="0">{{ zhCN.armor.dismantle1 }}</option>
              <option :value="1">{{ zhCN.armor.dismantleReturn }}</option>
            </select>
            <input v-model.number="holeForm.count" type="number" min="1" class="num" :placeholder="zhCN.armor.count" />
            <button class="ghost" :disabled="busy" @click="emit('open-hole', selected.sid, holeForm.gid, holeForm.pos, holeForm.useType, holeForm.count)">
              {{ zhCN.armor.openHole }}
            </button>
          </div>
          <div class="row">
            <input v-model.number="holeForm.pos" type="number" min="0" class="num" :placeholder="zhCN.armor.pos" />
            <input v-model.number="holeForm.embedGid" type="number" class="num" :placeholder="zhCN.armor.pearlGid" />
            <button class="primary" :disabled="busy" @click="emit('embed', selected.sid, holeForm.pos, holeForm.embedGid)">
              {{ zhCN.armor.embed }}
            </button>
          </div>
        </div>
        <ul class="list mini">
          <li v-for="a in bag" :key="a.sid" class="item row" @click="selectArmor(a)">
            <span :style="{ color: typeColor(a.type) }">{{ a.name }}</span>
            <span class="dim small">{{ a.embed_holes || zhCN.armor.noHoles }}</span>
          </li>
        </ul>
      </template>

      <!-- 材料升级 / 宝石合成 -->
      <template v-else>
        <p v-if="!gems.length" class="hint">{{ zhCN.armor.pickGem }}</p>
        <ul v-else class="list mini">
          <li
            v-for="g in gems"
            :key="g.gid"
            class="item row"
            :class="{ sel: g.gid === synthForm.gid }"
            @click="synthForm.gid = g.gid"
          >
            <img v-if="iconOk(g.gid)" class="gem-icon" :src="itemIcon(g.gid)" alt="" @error="markIconFailed(g.gid)" />
            <span>{{ g.name || `${zhCN.armor.gid} ${g.gid}` }}</span>
            <span class="dim small">×{{ g.count }}</span>
          </li>
        </ul>
        <div v-if="synthForm.gid > 0" class="detail">
          <div class="row">
            <input v-model.number="synthForm.count" type="number" min="1" class="num" :placeholder="zhCN.armor.count" />
            <label class="dim small">
              <input v-model.number="synthForm.dj" type="checkbox" :true-value="1" :false-value="0" />
              {{ zhCN.armor.useQiaoShou }}
            </label>
            <label class="dim small">
              <input v-model.number="synthForm.pearlProtect" type="checkbox" :true-value="1" :false-value="0" />
              {{ zhCN.armor.usePearlProtect }}
            </label>
            <button class="primary" :disabled="busy || synthForm.count <= 0" @click="doSynth">
              {{ zhCN.armor.synth }}
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.55);
}

.modal {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  max-width: 640px;
  max-height: 86vh;
  padding: 16px;
  overflow-y: auto;
}

.sub {
  flex: 1;
  text-align: right;
}

.tabs {
  display: flex;
  gap: 6px;
}

.tab {
  padding: 5px 14px;
  color: var(--text-dim);
  background: transparent;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.tab.on {
  color: #10202e;
  background: var(--accent);
  border-color: var(--accent);
}

.list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.list.mini {
  max-height: 180px;
  overflow-y: auto;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  cursor: pointer;
}

.item.sel {
  border-color: var(--accent);
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  flex-wrap: wrap;
}

.detail {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.dim {
  color: var(--text-dim);
}

.small {
  font-size: 12px;
}

.state {
  color: var(--accent);
  font-size: 13px;
}

.holes {
  gap: 6px;
}

.hole {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 28px;
  color: var(--text-dim);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.hole.open {
  color: var(--accent);
  border-color: var(--accent);
}

.hole-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.gem-icon {
  width: 26px;
  height: 26px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--bg);
}

select,
.num {
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

button {
  padding: 5px 14px;
  border-radius: 4px;
  cursor: pointer;
}

.primary {
  color: #10202e;
  background: var(--accent);
  border: 1px solid var(--accent);
}

.ghost {
  color: var(--text);
  background: transparent;
  border: 1px solid var(--panel-border);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}
</style>
