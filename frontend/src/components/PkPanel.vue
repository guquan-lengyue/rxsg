<script setup lang="ts">
import { computed, ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { heroFace, img } from '@/assets/img'
import type { PkBattle, PkCampaign, PkHero, PkHeroPage, PkLevel, PkMax, PkPrize, PkRankRow } from '@/api/pk'

const props = defineProps<{
  uid: number
  campaign: PkCampaign | null
  max: PkMax | null
  rank: PkRankRow[]
  heroPage: PkHeroPage | null
  battle: PkBattle | null
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'load-rank', battleId: number, type: number): void
  (e: 'first-reward', battleId: number, flag: number, rankId: number): void
  (e: 'fight', battleId: number, flag: number, level: number, hids: number[]): void
  (e: 'buy', type: number, count: number): void
  (e: 'load-heroes', page: number, hids: number[]): void
  (e: 'clear-battle'): void
  (e: 'close'): void
}>()

const tab = ref<'battle' | 'rank'>('battle')
const selectedBattleId = ref(0)
const flag = ref(0)
const selectedLevel = ref(0)
const selectedHids = ref<number[]>([])
const heroPageNo = ref(1)
const buyCount = ref(1)
const failed = ref<Record<number, boolean>>({})

const heroesOfFlag = computed<PkHero[]>(() =>
  flag.value === 1 ? props.campaign?.specialHeroes ?? [] : props.campaign?.normalHeroes ?? [],
)

// 战役下拉：battleDesc 提供 id/description。
const battleOptions = computed(() => props.campaign?.battles ?? [])

// 当前战役 + 难度下的关卡（按 levelid 分组）。
const levels = computed<{ levelid: number; heroes: PkHero[] }[]>(() => {
  const map = new Map<number, PkHero[]>()
  for (const h of heroesOfFlag.value) {
    if (h.battleid !== selectedBattleId.value) {
      continue
    }
    const list = map.get(h.levelid) ?? []
    list.push(h)
    map.set(h.levelid, list)
  }
  return Array.from(map.entries())
    .map(([levelid, heroes]) => ({ levelid, heroes: heroes.sort((a, b) => a.area - b.area) }))
    .sort((a, b) => a.levelid - b.levelid)
})

const currentLevelHeroes = computed<PkHero[]>(() => {
  const hit = levels.value.find((l) => l.levelid === selectedLevel.value)
  return hit ? hit.heroes : []
})

const progressText = computed(() => {
  const lv: PkLevel | null = props.max?.level ?? props.campaign?.level ?? null
  if (!lv) {
    return '—'
  }
  const n = String(lv.normal)
  const s = String(lv.special)
  return `普通 ${n.slice(0, -2) || 1}-${n.slice(-2)} / 精英 ${s.slice(0, -2) || 1}-${s.slice(-2)}`
})

const junling = computed(() => props.max?.junling ?? props.campaign?.goods.junling ?? 0)
const wuzhuqian = computed(() => props.max?.wuzhuqian ?? props.campaign?.goods.wuzhuqian ?? 0)

const selectableHeroes = computed(() => {
  const page = props.heroPage
  if (!page) {
    return [] as { hid: number; name: string; sex: number; face: number; level: number; bravery: number }[]
  }
  const list = page.heroes.map((h) => ({
    hid: h.hid,
    name: h.name,
    sex: h.sex,
    face: h.face,
    level: h.level,
    bravery: h.bravery,
  }))
  if (page.kingHero) {
    list.unshift({
      hid: page.kingHero.hid,
      name: page.kingHero.name,
      sex: page.kingHero.sex,
      face: page.kingHero.face,
      level: page.kingHero.level,
      bravery: page.kingHero.bravery,
    })
  }
  return list
})

function heroAvatar(sex: number, face: number): string {
  return heroFace(sex, face)
}

function onAvatarError(hid: number): void {
  failed.value = { ...failed.value, [hid]: true }
}

function selectBattle(id: number): void {
  selectedBattleId.value = id
  selectedLevel.value = 0
}

function toggleHero(hid: number): void {
  const list = selectedHids.value
  if (list.includes(hid)) {
    selectedHids.value = list.filter((h) => h !== hid)
  } else if (list.length < 3) {
    selectedHids.value = [...list, hid]
  }
}

function loadHeroes(): void {
  emit('load-heroes', heroPageNo.value, selectedHids.value)
}

function changeHeroPage(delta: number): void {
  const next = heroPageNo.value + delta
  if (next < 1) {
    return
  }
  heroPageNo.value = next
  loadHeroes()
}

function onFight(): void {
  if (selectedBattleId.value && selectedLevel.value && selectedHids.value.length >= 3) {
    emit('fight', selectedBattleId.value, flag.value, selectedLevel.value, [...selectedHids.value])
  }
}

function onLoadRank(): void {
  if (selectedBattleId.value) {
    emit('load-rank', selectedBattleId.value, flag.value)
  }
}

function prizeName(p: PkPrize): string {
  return p.name || (p.isArmor ? `装备${p.id}` : `道具${p.gid}`)
}

function prizeIcon(p: PkPrize): string {
  return p.isArmor ? img(`armor/${p.id}.png`) : img(`item_${p.gid}.png`)
}

function isSelf(uid: number): boolean {
  return uid === props.uid
}

const battleReport = computed(() => props.battle?.report ?? [])
</script>

<template>
  <SkinDialog title="单机征战" wide @close="emit('close')">
    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <template v-else>
      <div class="status">
        <span>军令：{{ junling }}</span>
        <span>武力钺：{{ wuzhuqian }}</span>
        <span>进度：{{ progressText }}</span>
      </div>

      <div class="u-tabs">
        <button class="u-tab" :class="{ 'is-active': tab === 'battle' }" type="button" @click="tab = 'battle'">
          征战
        </button>
        <button class="u-tab" :class="{ 'is-active': tab === 'rank' }" type="button" @click="tab = 'rank'">
          首通榜
        </button>
      </div>

      <!-- 战斗结果 -->
      <section v-if="battle" class="result">
        <header class="r-head">
          <strong :class="battle.winer === 1 ? 'win' : 'lose'">
            {{ battle.winer === 1 ? '战斗胜利' : '战斗失败' }}
          </strong>
          <button class="u-btn u-btn--cancel" type="button" @click="emit('clear-battle')">关闭战报</button>
        </header>
        <p class="u-hint">总回合数：{{ battle.totalNum }}</p>
        <ul v-if="battle.reward.length" class="rewards">
          <li v-for="(r, idx) in battle.reward" :key="idx">
            <span>{{ prizeName(r) }} ×{{ r.count }}</span>
          </li>
        </ul>
        <div class="report u-scroll">
          <div v-for="(round, ri) in battleReport" :key="ri" class="round">
            <span class="rlabel">第{{ ri + 1 }}回合</span>
            <div v-for="(hit, hi) in round" :key="hi" class="hit">
              <span>#{{ hit.attackhid }} → #{{ hit.resisthid }}</span>
              <span class="dmg">伤害 {{ hit.damage }}</span>
              <span class="blood">剩余 {{ hit.blood }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- 征战 -->
      <template v-else-if="tab === 'battle'">
        <div class="battles">
          <button
            v-for="b in battleOptions"
            :key="b.id"
            class="u-btn"
            :class="b.id === selectedBattleId ? 'u-btn--gold' : 'u-btn--green'"
            type="button"
            @click="selectBattle(b.id)"
          >
            {{ b.description || `战役${b.id}` }}
          </button>
        </div>

        <div class="u-tabs">
          <button class="u-tab" :class="{ 'is-active': flag === 0 }" type="button" @click="flag = 0">普通</button>
          <button class="u-tab" :class="{ 'is-active': flag === 1 }" type="button" @click="flag = 1">精英</button>
        </div>

        <div v-if="selectedBattleId" class="levels">
          <button
            v-for="l in levels"
            :key="l.levelid"
            class="level"
            :class="{ 'is-active': l.levelid === selectedLevel }"
            type="button"
            @click="selectedLevel = l.levelid"
          >
            第{{ l.levelid }}关
          </button>
          <span v-if="!levels.length" class="u-hint">该战役暂无关卡</span>
        </div>

        <!-- NPC 站位 -->
        <div v-if="currentLevelHeroes.length" class="npc">
          <span class="d-label">守将站位</span>
          <div class="npc-list">
            <div v-for="h in currentLevelHeroes" :key="h.hid" class="npc-hero">
              <img
                v-if="!failed[h.hid]"
                class="avatar"
                :src="heroAvatar(h.sex, h.face)"
                :alt="h.name"
                @error="onAvatarError(h.hid)"
              />
              <span class="stand">{{ h.area }}</span>
              <span class="hname">{{ h.name }}</span>
              <span class="hstat">武{{ h.bravery_base + h.bravery_add }} 智{{ h.wisdom_base + h.wisdom_add }}</span>
            </div>
          </div>
        </div>

        <!-- 出战将领选择 -->
        <div class="d-block">
          <div class="d-head">
            <span class="d-label">出战将领（已选 {{ selectedHids.length }}/3）</span>
            <div class="pager">
              <button class="u-btn u-btn--green" type="button" @click="changeHeroPage(-1)">上页</button>
              <span>{{ heroPageNo }}</span>
              <button class="u-btn u-btn--green" type="button" @click="changeHeroPage(1)">下页</button>
              <button class="u-btn u-btn--yellow" type="button" @click="loadHeroes">加载</button>
            </div>
          </div>
          <div class="heroes">
            <button
              v-for="h in selectableHeroes"
              :key="h.hid"
              class="hero"
              :class="{ 'is-active': selectedHids.includes(h.hid) }"
              type="button"
              @click="toggleHero(h.hid)"
            >
              <img
                v-if="!failed[h.hid]"
                class="avatar"
                :src="heroAvatar(h.sex, h.face)"
                :alt="h.name"
                @error="onAvatarError(h.hid)"
              />
              <span class="hname">{{ h.name }}</span>
              <span class="hstat">Lv{{ h.level }} 武{{ h.bravery }}</span>
            </button>
            <span v-if="!selectableHeroes.length" class="u-hint">点击「加载」获取将领</span>
          </div>
        </div>

        <div class="actions">
          <div class="buy">
            <label>购买军令</label>
            <input v-model.number="buyCount" class="u-input" type="number" min="1" />
            <button class="u-btn u-btn--yellow" type="button" :disabled="busy" @click="emit('buy', 0, buyCount)">
              购买
            </button>
          </div>
          <button
            class="u-btn u-btn--red"
            type="button"
            :disabled="busy || selectedHids.length < 3 || !selectedLevel"
            @click="onFight"
          >
            出战
          </button>
        </div>
      </template>

      <!-- 首通榜 -->
      <template v-else>
        <div class="actions">
          <button class="u-btn u-btn--green" type="button" @click="onLoadRank">刷新榜单</button>
        </div>
        <ul class="ranks">
          <li v-for="r in rank" :key="r.rankid" class="rank-row">
            <span class="rk">{{ r.rankid }}</span>
            <span class="who">{{ r.userName || (isSelf(r.uid) ? '我' : `玩家${r.uid}`) }}</span>
            <span class="tm">{{ r.formatTime }}</span>
            <span v-if="r.rewardGood" class="prize">
              <img
                v-if="!failed[9000 + r.rankid]"
                class="icon"
                :src="prizeIcon(r.rewardGood)"
                alt="奖励"
                @error="onAvatarError(9000 + r.rankid)"
              />
              {{ prizeName(r.rewardGood) }}×{{ r.rewardCount }}
            </span>
            <button
              v-if="isSelf(r.uid)"
              class="u-btn u-btn--gold"
              type="button"
              :disabled="busy"
              @click="emit('first-reward', r.battleid, r.type, r.rankid)"
            >
              领取
            </button>
          </li>
          <li v-if="!rank.length" class="u-hint">暂无记录，点击「刷新榜单」查询</li>
        </ul>
      </template>
    </template>
  </SkinDialog>
</template>

<style scoped>
.status {
  display: flex;
  gap: 16px;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--text-dim);
}

.battles,
.levels {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0;
}

.level {
  padding: 4px 10px;
  color: var(--text-dim);
  font-size: 13px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.level.is-active {
  color: var(--accent);
  border-color: var(--accent);
}

.npc-list,
.heroes {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.npc-hero,
.hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  width: 72px;
  padding: 6px 4px;
  color: var(--text-dim);
  font-size: 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.hero {
  cursor: pointer;
}

.hero.is-active {
  color: var(--accent);
  border-color: var(--accent);
}

.avatar {
  width: 40px;
  height: 40px;
  object-fit: cover;
  border-radius: 4px;
}

.stand {
  color: var(--accent);
}

.hname {
  color: var(--text);
}

.hstat {
  font-size: 11px;
}

.d-block {
  padding: 8px 10px;
  margin-top: 8px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.d-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.d-label {
  color: var(--accent);
  font-size: 13px;
}

.pager {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-dim);
}

.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
}

.buy {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}

.u-input {
  width: 60px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.result {
  margin-top: 8px;
}

.r-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.r-head .win {
  color: #6fbf73;
}

.r-head .lose {
  color: var(--danger);
}

.rewards {
  margin: 0 0 6px;
  padding: 0;
  list-style: none;
  font-size: 13px;
  color: var(--accent);
}

.report {
  max-height: 240px;
}

.round {
  padding: 4px 0;
  border-bottom: 1px solid var(--panel-border);
}

.rlabel {
  color: var(--text-dim);
  font-size: 12px;
}

.hit {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--text-dim);
}

.dmg {
  color: #e08a4b;
}

.blood {
  color: var(--accent);
}

.ranks {
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.rank-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  margin-bottom: 4px;
  font-size: 13px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.rk {
  width: 20px;
  color: var(--accent);
  text-align: center;
}

.who {
  flex: 1;
  min-width: 0;
  color: var(--text);
}

.tm,
.prize {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--text-dim);
  font-size: 12px;
}

.icon {
  width: 20px;
  height: 20px;
  object-fit: contain;
}
</style>
