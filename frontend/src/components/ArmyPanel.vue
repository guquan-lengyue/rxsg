<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { formatLeft } from '@/render/cityGrid'
import type { ArmyInfo, DispatchPayload, Field, Hero, March } from '@/types'

const props = defineProps<{
  info: ArmyInfo | null
  fields: Field[]
  marches: March[]
  heroes: Hero[]
  loading: boolean
  error: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'draft', sid: number, count: number): void
  (e: 'stop-draft', qid: number): void
  (e: 'dissolve', sid: number, count: number): void
  (e: 'dispatch', payload: DispatchPayload): void
  (e: 'recall', id: number): void
  (e: 'close'): void
}>()

const tab = ref<'draft' | 'march'>('draft')

// 每兵种的数量输入（征兵/解散共用）。
const counts = reactive<Record<number, number>>({})
// 出征编队。
const alloc = reactive<Record<number, number>>({})
const targetId = ref(0)
const task = ref(3)
const heroId = ref(0)
const localError = ref('')

const soldiers = computed(() => props.info?.soldiers ?? [])
const queues = computed(() => props.info?.queues ?? [])

function nameOf(sid: number): string {
  return soldiers.value.find((s) => s.sid === sid)?.sname ?? `兵种${sid}`
}

const allocTotal = computed(() => Object.values(alloc).reduce((a, b) => a + (b || 0), 0))

function marchSoldiers(m: March): string {
  return Object.entries(m.soldiers)
    .filter(([, c]) => c > 0)
    .map(([sid, c]) => `${nameOf(Number(sid))}×${c}`)
    .join(' ')
}

function taskText(t: number): string {
  return t === 4 ? zhCN.army.occupy : zhCN.army.plunder
}

function onDraft(sid: number): void {
  emit('draft', sid, counts[sid] || 0)
}

function onDissolve(sid: number): void {
  emit('dissolve', sid, counts[sid] || 0)
}

function onDispatch(): void {
  localError.value = ''
  if (!targetId.value) {
    localError.value = zhCN.army.target
    return
  }
  const picked: Record<string, number> = {}
  for (const [sid, c] of Object.entries(alloc)) {
    if (c > 0) {
      picked[sid] = c
    }
  }
  if (!Object.keys(picked).length) {
    localError.value = zhCN.army.selectSoldier
    return
  }
  emit('dispatch', {
    hero_id: heroId.value,
    target_type: 1,
    target_id: targetId.value,
    task: task.value,
    soldiers: picked,
  })
  for (const sid of Object.keys(alloc)) {
    alloc[Number(sid)] = 0
  }
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal">
      <header class="modal-head">
        <strong>{{ zhCN.army.title }}</strong>
        <button class="close" @click="emit('close')">×</button>
      </header>

      <div class="tabs">
        <button :class="{ active: tab === 'draft' }" @click="tab = 'draft'">
          {{ zhCN.army.tabDraft }}
        </button>
        <button :class="{ active: tab === 'march' }" @click="tab = 'march'">
          {{ zhCN.army.tabMarch }}
          <span v-if="marches.length" class="badge">{{ marches.length }}</span>
        </button>
      </div>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>

      <template v-else-if="info">
        <!-- 征兵 -->
        <template v-if="tab === 'draft'">
          <div class="meta">
            <span>{{ zhCN.army.people }} {{ info.people }}/{{ info.people_max }}</span>
            <span>{{ zhCN.army.barracksLevel }} Lv{{ info.barracksLevel }}</span>
          </div>

          <template v-if="queues.length">
            <p class="section">{{ zhCN.army.queues }}</p>
            <ul class="list">
              <li v-for="q in queues" :key="q.qid" class="item">
                <div class="row">
                  <strong>{{ q.sname }} ×{{ q.count }}</strong>
                  <span class="state">
                    {{ q.state === 1 ? zhCN.army.training : zhCN.army.waiting }}
                  </span>
                </div>
                <div class="row">
                  <span class="dim">{{ formatLeft(q.time_left) }}</span>
                  <button class="ghost" :disabled="busy" @click="emit('stop-draft', q.qid)">
                    {{ zhCN.army.cancel }}
                  </button>
                </div>
              </li>
            </ul>
          </template>

          <p class="section">{{ zhCN.army.empty }}</p>
          <ul class="list">
            <li v-for="s in soldiers" :key="s.sid" class="item">
              <div class="row">
                <strong>{{ s.sname }}</strong>
                <span class="dim">{{ zhCN.army.owned }} {{ s.count }}</span>
              </div>
              <div class="row dim small">
                <span>攻{{ s.ap }} 防{{ s.dp }} 速{{ s.speed }}</span>
                <span>
                  {{ zhCN.army.need }} {{ zhCN.resource.wood }}{{ s.woodNeed }}
                  {{ zhCN.resource.rock }}{{ s.rockNeed }}
                  {{ zhCN.resource.iron }}{{ s.ironNeed }}
                  {{ zhCN.resource.food }}{{ s.foodNeed }}
                  {{ zhCN.resource.gold }}{{ s.goldNeed }}
                  人{{ s.peopleNeed }}
                </span>
              </div>
              <p v-if="!s.can_draft && s.no_draft_msg" class="hint error small">
                {{ s.no_draft_msg }}
              </p>
              <div class="row">
                <input v-model.number="counts[s.sid]" type="number" min="0" class="num" />
                <div class="btns">
                  <button
                    class="primary"
                    :disabled="busy || !s.can_draft || !(counts[s.sid] > 0)"
                    @click="onDraft(s.sid)"
                  >
                    {{ zhCN.army.draft }}
                  </button>
                  <button
                    class="ghost"
                    :disabled="busy || !(counts[s.sid] > 0) || counts[s.sid] > s.count"
                    @click="onDissolve(s.sid)"
                  >
                    {{ zhCN.army.dissolve }}
                  </button>
                </div>
              </div>
            </li>
          </ul>
        </template>

        <!-- 出征 -->
        <template v-else>
          <p class="section">{{ zhCN.army.target }}</p>
          <p v-if="!fields.length" class="hint">{{ zhCN.army.emptyFields }}</p>
          <ul v-else class="list">
            <li
              v-for="f in fields"
              :key="f.id"
              class="item target"
              :class="{ picked: targetId === f.id }"
              @click="targetId = f.id"
            >
              <div class="row">
                <strong>{{ f.name }} Lv{{ f.level }}</strong>
                <span class="dim">
                  {{ zhCN.army.guard }} {{ f.guard_power }}
                  <span v-if="f.owner_uid">{{ zhCN.army.occupied }}</span>
                </span>
              </div>
            </li>
          </ul>

          <div class="row">
            <span class="dim">{{ zhCN.army.task }}</span>
            <select v-model.number="task">
              <option :value="3">{{ zhCN.army.plunder }}</option>
              <option :value="4">{{ zhCN.army.occupy }}</option>
            </select>
          </div>

          <div class="row">
            <span class="dim">{{ zhCN.army.hero }}</span>
            <select v-model.number="heroId">
              <option :value="0">{{ zhCN.army.noHero }}</option>
              <option v-for="h in heroes" :key="h.hid" :value="h.hid">{{ h.name }}</option>
            </select>
          </div>

          <p class="section">{{ zhCN.army.selectSoldier }}（{{ allocTotal }}）</p>
          <div class="alloc">
            <label v-for="s in soldiers" :key="s.sid" class="alloc-item">
              <span>{{ s.sname }}</span>
              <input
                v-model.number="alloc[s.sid]"
                type="number"
                min="0"
                :max="s.count"
                class="num"
              />
              <span class="dim">/{{ s.count }}</span>
            </label>
          </div>

          <p v-if="localError" class="hint error">{{ localError }}</p>
          <div class="actions">
            <button class="primary" :disabled="busy" @click="onDispatch">
              {{ zhCN.army.dispatch }}
            </button>
          </div>

          <p class="section">{{ zhCN.army.marches }}</p>
          <p v-if="!marches.length" class="hint">{{ zhCN.army.emptyMarches }}</p>
          <ul v-else class="list">
            <li v-for="m in marches" :key="m.id" class="item">
              <div class="row">
                <strong>{{ m.hero_name || zhCN.army.noHero }}</strong>
                <span class="state">
                  {{ m.state === 0 ? zhCN.army.outbound : zhCN.army.returning }}
                </span>
              </div>
              <div class="row dim small">
                <span>{{ m.target_name }} · {{ taskText(m.task) }}</span>
                <span>{{ formatLeft(m.time_left) }}</span>
              </div>
              <div class="row">
                <span class="dim small">{{ marchSoldiers(m) }}</span>
                <button
                  class="ghost"
                  :disabled="busy || m.state !== 0"
                  @click="emit('recall', m.id)"
                >
                  {{ zhCN.army.recall }}
                </button>
              </div>
            </li>
          </ul>
        </template>
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
  max-width: 560px;
  max-height: 86vh;
  padding: 16px;
  overflow-y: auto;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.modal-head strong {
  color: var(--accent);
  font-size: 16px;
}

.close {
  color: var(--text-dim);
  background: transparent;
  border: none;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}

.tabs {
  display: flex;
  gap: 8px;
  border-bottom: 1px solid var(--panel-border);
}

.tabs button {
  padding: 6px 14px;
  color: var(--text-dim);
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
}

.tabs button.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}

.badge {
  margin-left: 6px;
  padding: 0 6px;
  font-size: 12px;
  color: #10202e;
  background: var(--accent);
  border-radius: 8px;
}

.meta {
  display: flex;
  justify-content: space-between;
  color: var(--text-dim);
  font-size: 13px;
}

.section {
  margin: 6px 0 0;
  color: var(--text-dim);
  font-size: 13px;
  border-bottom: 1px solid var(--panel-border);
  padding-bottom: 4px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.target {
  cursor: pointer;
}

.target.picked {
  border-color: var(--accent);
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
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

.num {
  width: 72px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.btns {
  display: flex;
  gap: 8px;
}

.alloc {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}

.alloc-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.actions {
  display: flex;
  justify-content: flex-end;
}

.actions button,
.btns button {
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

select {
  padding: 4px 8px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}
</style>