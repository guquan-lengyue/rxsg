<script setup lang="ts">
// 马厩/坐骑面板（bid=16，对齐 legacy Barn/BarnDialog.as + BarnCommandDialog.as 的
// 坐骑选择 / 坐骑装备 / 坐骑升级页签）。面板只负责展示与发事件（props in / events out），
// 读写由 CityView 执行并回灌 notice/error。
// 文案取自原版 LocaleChinese（Barn_*）。
import { computed, ref, watch } from 'vue'

import BuildingOps from '@/components/BuildingOps.vue'
import SkinDialog from '@/components/SkinDialog.vue'
import { armor, img, itemIcon } from '@/assets/img'
import type { BagArmor, BarnGood, BuildingDetail } from '@/types'

const props = defineProps<{
  mounts: BagArmor[]
  unactive: BagArmor[]
  goods: BarnGood[]
  slotGoods: BarnGood[]
  detail: BuildingDetail | null
  loading: boolean
  error: string
  notice: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'load-goods', xilianIndex: number): void
  (e: 'load-slot', zuojiType: number, armorid: number): void
  (e: 'embed', sid: number, pos: number, gid: number): void
  (e: 'unlade', sid: number, gid: number, pos: number): void
  (e: 'upgrade', sid: number, isProtected: boolean): void
  // 建筑操作（与坐骑升级 upgrade 区分，避免同名事件参数冲突）。
  (e: 'building-upgrade'): void
  (e: 'stop'): void
  (e: 'destroy'): void
  (e: 'destroy-all'): void
  (e: 'cancel-destroy'): void
  (e: 'close'): void
}>()

// 5 个坐骑槽位（对齐 Barn_ToolTip_0..4：盔/铠/鞍/绳/蹄铁）。
const slotNames = ['马盔', '马铠', '马鞍', '马绳', '马蹄铁']

// 洗练符 gid（对齐 BarnFunc.php:10 barnXilianGids：12079/12080/12081 三选一）。
const xilianGids = [12079, 12080, 12081]

const fallbackIcon = img('item_horse_default.png')
const lockIcon = img('item_default_embedArmorPearl_lock.png')

// 图标缺失时回退占位（仅触发一次，避免死循环）。
function onIconError(e: Event): void {
  const el = e.target as HTMLImageElement
  if (el.dataset.fallback === '1') {
    return
  }
  el.dataset.fallback = '1'
  el.src = fallbackIcon
}

const xilianIndex = ref(0)
const selectedSid = ref(0)
const selectedPos = ref(0)
const selectedGid = ref(0)
const useProtect = ref(false)

const selectedMount = computed<BagArmor | null>(
  () => props.mounts.find((m) => m.sid === selectedSid.value) ?? null,
)

// 已佩戴的坐骑装备（embed_pearls 串逐位 = 槽位 pos）。
const pearls = computed<number[]>(() =>
  (selectedMount.value?.embed_pearls ?? '').split(',').map((v) => Number(v) || 0),
)
function gidAt(pos: number): number {
  return pearls.value[pos] ?? 0
}

// 默认选中第一匹坐骑（背包内 part=12）。
watch(
  () => props.mounts,
  (list) => {
    if (!list.some((m) => m.sid === selectedSid.value)) {
      selectedSid.value = list[0]?.sid ?? 0
    }
  },
  { immediate: true },
)

// 切换坐骑/槽位时按 zuoji_type(=pos+1) 拉取可镶嵌坐骑装备。
watch(
  () => [selectedSid.value, selectedPos.value] as const,
  () => {
    const m = selectedMount.value
    if (m) {
      emit('load-slot', selectedPos.value + 1, m.armorid)
    }
  },
  { immediate: true },
)

function onXilianChange(): void {
  emit('load-goods', xilianIndex.value)
}

function selectSlot(pos: number): void {
  selectedPos.value = pos
  selectedGid.value = 0
}

function toggleGid(gid: number): void {
  selectedGid.value = selectedGid.value === gid ? 0 : gid
}

function doEmbed(): void {
  const m = selectedMount.value
  if (!m || selectedGid.value <= 0) {
    return
  }
  emit('embed', m.sid, selectedPos.value, selectedGid.value)
}

function doUnlade(pos: number): void {
  const m = selectedMount.value
  const gid = gidAt(pos)
  if (!m || gid <= 0) {
    return
  }
  emit('unlade', m.sid, gid, pos)
}

function doUpgrade(): void {
  const m = selectedMount.value
  if (!m) {
    return
  }
  emit('upgrade', m.sid, useProtect.value)
}

const hasMount = computed(() => props.mounts.length > 0)
</script>

<template>
  <SkinDialog title="马厩" wide @close="emit('close')">
    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>
    <p v-else-if="notice" class="u-hint is-notice">{{ notice }}</p>

    <!-- 坐骑选择 -->
    <div class="u-row">
      <span class="label">坐骑选择</span>
      <select v-model.number="selectedSid" class="u-select" :disabled="!hasMount">
        <option v-for="m in mounts" :key="m.sid" :value="m.sid">
          {{ m.name }}（强化{{ m.strong_level }} 熔炼{{ m.combine_level }}）
        </option>
      </select>
      <span v-if="!hasMount" class="u-hint">背包中暂无坐骑</span>
    </div>

    <!-- 建筑操作（升级 / 拆除 / 取消拆除），与通用建筑面板单一实现 -->
    <BuildingOps
      v-if="detail"
      :detail="detail"
      :busy="busy"
      @upgrade="emit('building-upgrade')"
      @stop="emit('stop')"
      @destroy="emit('destroy')"
      @destroy-all="emit('destroy-all')"
      @cancel-destroy="emit('cancel-destroy')"
    />

    <div v-if="selectedMount" class="u-row">
      <img class="mount-img" :src="armor(selectedMount.armorid)" alt="" @error="onIconError" />
      <span class="label">耐久</span>
      <span>{{ selectedMount.hp }}/{{ selectedMount.hp_max }}</span>
    </div>

    <!-- 5 个坐骑槽位（盔/铠/鞍/绳/蹄铁）：始终渲染，无坐骑时空态占位 -->
    <ul class="slots">
      <li
        v-for="(label, pos) in slotNames"
        :key="pos"
        class="slot"
        :class="{ 'is-active': pos === selectedPos }"
        :title="label"
      >
        <button class="slot-btn" type="button" @click="selectSlot(pos)">
          <img
            class="slot-img"
            :src="gidAt(pos) > 0 ? itemIcon(gidAt(pos)) : lockIcon"
            :alt="label"
            @error="onIconError"
          />
          <span class="slot-label">{{ label }}</span>
        </button>
        <button
          v-if="gidAt(pos) > 0"
          class="u-btn u-btn--red slot-unlade"
          type="button"
          :disabled="busy"
          @click="doUnlade(pos)"
        >
          卸载
        </button>
      </li>
    </ul>

    <!-- 洗练符（3 选 1）：始终渲染区域与标题，便于验证布局 -->
    <div class="u-row">
      <span class="label">洗练符</span>
      <select v-model.number="xilianIndex" class="u-select" @change="onXilianChange">
        <option v-for="(gid, i) in xilianGids" :key="gid" :value="i">洗练符（{{ gid }}）</option>
      </select>
    </div>

    <template v-if="selectedMount">
      <!-- 马厩道具（按所选洗练符返回） -->
      <ul v-if="goods.length" class="goods">
        <li v-for="g in goods" :key="g.gid" class="good">
          <img class="good-img" :src="itemIcon(g.gid)" :alt="g.name" @error="onIconError" />
          <span class="good-name">{{ g.name || `道具 ${g.gid}` }}</span>
          <span class="u-hint">×{{ g.count }}</span>
        </li>
      </ul>

      <!-- 当前槽位可镶嵌的坐骑装备 -->
      <p class="u-hint">当前槽位（{{ slotNames[selectedPos] }}）可选坐骑装备：</p>
      <ul v-if="slotGoods.length" class="goods">
        <li
          v-for="g in slotGoods"
          :key="g.gid"
          class="good selectable"
          :class="{ 'is-active': selectedGid === g.gid }"
          @click="toggleGid(g.gid)"
        >
          <img class="good-img" :src="itemIcon(g.gid)" :alt="g.name" @error="onIconError" />
          <span class="good-name">{{ g.name || `装备 ${g.gid}` }}</span>
          <span class="u-hint">×{{ g.count }}</span>
        </li>
      </ul>
      <p v-else class="u-hint">当前槽位暂无可镶嵌的坐骑装备</p>

      <!-- 未激活坐骑（后端仅返回装备列表段，原版 6 段属性聚合未实现） -->
      <p class="u-hint">
        未激活坐骑：
        <template v-if="unactive.length">
          {{ unactive.map((m) => m.name).join('、') }}
        </template>
        <template v-else>你没有驯服坐骑，请到马厩驯服</template>
      </p>
    </template>
    <p v-else class="u-hint">你没有驯服坐骑，请到马厩驯服</p>

    <template #footer>
      <label class="protect">
        <input v-model="useProtect" type="checkbox" />
        使用升级保护符
      </label>
      <button class="u-btn u-btn--gold" type="button" :disabled="busy || !selectedMount || selectedGid <= 0" @click="doEmbed">
        装备
      </button>
      <button class="u-btn u-btn--confirm" type="button" :disabled="busy || !selectedMount" @click="doUpgrade">
        升级
      </button>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.u-hint.is-notice {
  color: var(--accent);
}

.mount-img {
  width: 48px;
  height: 48px;
  object-fit: contain;
}

.slots {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  margin: 8px 0;
  padding: 0;
  list-style: none;
}

.slot {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 6px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.slot.is-active {
  border-color: var(--accent);
}

.slot-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 0;
  background: transparent;
  border: none;
}

.slot-img {
  width: 40px;
  height: 40px;
  object-fit: contain;
}

.slot-label {
  font-size: 12px;
  color: var(--text-dim);
}

.slot-unlade {
  min-width: 0;
  height: 22px;
  padding: 0 8px;
  font-size: 12px;
}

.goods {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 6px;
  margin: 6px 0;
  padding: 0;
  list-style: none;
}

.good {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  font-size: 13px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.good.selectable {
  cursor: pointer;
}

.good.selectable.is-active {
  border-color: var(--accent);
}

.good-img {
  width: 24px;
  height: 24px;
  object-fit: contain;
}

.good-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.protect {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--text-dim);
}
</style>
