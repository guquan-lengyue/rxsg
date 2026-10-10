<script setup lang="ts">
// BuildingExchangeDialog —— 资源地转换（对齐原版 BuildingExchangePanel / startChangeBuilding）。
// 仅接收 props、向外 emit；接口调用由 CityView 编排。
// 目标候选来源已确认为固定 4 种资源地（原版 Define.RES_TYPE = 占位 + 农田/伐木场/采石场/铁矿，
// 对应 legacy bid 1..4，经重写映射即新库 building_id 2..5）。
// 文案取原版 locale：buildingExchangePanel_title=资源地转换、_change_res=原资源地、_change_taget=目标类型、
// _change_des=使用“地变符”进行资源地转化时，资源地的等级不变、_change_submit=确定、
// _select=请选择要转换成的类型、_same_ret=类型相同，无需转化、_Define_Res_1..4=农田/伐木场/采石场/铁矿。
import { ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { zhCN } from '@/lang/zh-CN'

const props = defineProps<{
  bid: number
  name: string
  busy: boolean
  error: string
}>()
const emit = defineEmits<{
  (e: 'exchange', targetbid: number): void
  (e: 'close'): void
}>()

// 目标资源地：新库 building_id（2农田/3伐木场/4采石场/5铁矿），与后端 isResourceField 一致。
const TARGETS: { bid: number; name: string }[] = [
  { bid: 2, name: zhCN.city.resFarmland },
  { bid: 3, name: zhCN.city.resWood },
  { bid: 4, name: zhCN.city.resRock },
  { bid: 5, name: zhCN.city.resIron },
]

const selected = ref(0)
const message = ref('')

function submit(): void {
  if (props.busy) {
    return
  }
  if (selected.value < 1) {
    message.value = zhCN.city.exchangeSelect
    return
  }
  if (selected.value === props.bid) {
    message.value = zhCN.city.exchangeSame
    return
  }
  message.value = ''
  emit('exchange', selected.value)
}
</script>

<template>
  <SkinDialog :title="zhCN.city.exchangeTitle" @close="emit('close')">
    <div class="row">
      <span class="label">{{ zhCN.city.exchangeRes }}</span>
      <span>{{ name }}</span>
    </div>
    <div class="row">
      <span class="label">{{ zhCN.city.exchangeTarget }}</span>
      <select v-model.number="selected">
        <option :value="0">{{ zhCN.city.exchangeSelect }}</option>
        <option v-for="t in TARGETS" :key="t.bid" :value="t.bid">{{ t.name }}</option>
      </select>
    </div>
    <p class="desc">{{ zhCN.city.exchangeDes }}</p>
    <p v-if="message" class="hint error">{{ message }}</p>
    <p v-if="error" class="hint error">{{ error }}</p>

    <div class="actions">
      <button class="ok" :disabled="busy" @click="submit">{{ zhCN.city.ok }}</button>
    </div>
  </SkinDialog>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
  font-size: 14px;
}

.label {
  color: var(--text-dim);
}

select {
  min-width: 140px;
  padding: 5px 8px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.desc {
  margin: 8px 0;
  color: var(--text-dim);
  font-size: 13px;
}

.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 8px;
}

.actions button {
  min-width: 64px;
  padding: 5px 16px;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.ok {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.ok:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.hint.error {
  color: var(--danger);
  font-size: 13px;
}
</style>
