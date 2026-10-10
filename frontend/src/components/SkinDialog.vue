<script setup lang="ts">
// SkinDialog —— 原版弹窗外壳（board_popup 九宫格 + popup_title 标题条 + lottery_close 关闭钮）。
// 新增面板（M6–M9）统一复用本组件，保证与原版弹窗观感一致；
// 内容区与底部按钮区分别用默认插槽与 #footer 插槽。
defineProps<{
  title: string
  /** 宽面板（如市场/战报） */
  wide?: boolean
  /** 是否允许点击遮罩关闭（默认允许） */
  maskClosable?: boolean
  /** 正文区随内容增高（解除 .u-scroll 的 60vh 上限；地图等需完整可见的面板使用）。默认沿用 60vh 上限。 */
  grow?: boolean
}>()

const emit = defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <div class="skin-mask" @click.self="maskClosable === false ? undefined : emit('close')">
    <section class="u-modal skin-dialog" :class="{ 'is-wide': wide }">
      <header class="u-title">
        <strong>{{ title }}</strong>
        <button class="u-close" type="button" title="关闭" @click="emit('close')"></button>
      </header>
      <div class="skin-body u-scroll" :class="{ 'is-grow': grow }">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="skin-foot">
        <slot name="footer" />
      </footer>
    </section>
  </div>
</template>

<style scoped>
.skin-mask {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(6, 9, 14, 0.55);
}

.skin-dialog {
  display: flex;
  flex-direction: column;
  width: min(560px, 100%);
  max-height: calc(100vh - 48px);
  padding: 8px 10px 10px;
  /* 内容不得绘制到标题条之上（子面板若设了高 z-index/绝对定位，滚动时曾遮挡页签） */
  overflow: hidden;
}

.skin-dialog.is-wide {
  width: min(860px, 100%);
}

.skin-body {
  flex: 1 1 auto;
  min-height: 0;
  padding: 8px 2px;
  overflow-y: auto;
}

/* 解除 .u-scroll 的 60vh 上限，使正文随内容增高（父级 .skin-dialog 的 max-height 仍兜底）。 */
.skin-body.is-grow {
  max-height: none;
}

.skin-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
}
</style>
