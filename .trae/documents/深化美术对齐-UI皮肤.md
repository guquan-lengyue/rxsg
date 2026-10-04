# 深化美术对齐 —— 原版 UI 皮肤化

## Context（背景）
原版「热血三国」前端美术已整体拷入 `frontend/public/images/`，并已做完"先换图"（城池网格 building_intro、武将头像、兵种/科技/资源图标、登录页皮肤），均已提交（最近一次 `f577d51`）。

但重编码 Vue 的**界面骨架仍是现代扁平卡片**（青灰圆角卡、纯色描边按钮），与原版木纹/铜色战争美术观感差距大。本阶段在**保持现 Vue 布局结构不变**的前提下，把 UI 的皮肤换成原版贴图，让观感贴近原版。

权威映射来自仓库根 `common2.css`（原版样式表）：
- 弹窗背景 `board_popup.png`（9-slice），标题条 `popup_title.png`（9-slice）
- 顶栏模块按钮 `topbutton_hero/army`（自带文字）
- Tab `channel_tab1..4`（up/hl/sl 三态）
- 城池底部 tab `mycity_building/hero/...`
- 关闭小按钮 `lottery_close`（26×26，up/over/select）

用户已拍板：顶栏尽量用原版贴图（科技无原图，用 `topicon_tactic` 图标+文字合成）；弹窗关闭用原版 `lottery_close` 贴图。

## 方案（保持布局不变，只换皮肤）

### B1 · 全局主题 & 页面背景
文件：`frontend/src/styles.css`
- `body`/`.city-page` 背景换成 `bj.jpg` 深色战争底图（cover + 暗层），替换纯色底。
- 微调 CSS 变量（`--panel/--panel-border/--accent`）到铜色/木纹系，保持现有组件引用不变（变量驱动，改动面小）。

### B2 · 弹窗/面板 9-slice 皮肤
文件：`HeroPanel/ArmyPanel/TechnicPanel/BuildingPanel.vue`
- 抽出通用皮肤类（在 `styles.css` 定义 `.u-modal`、`.u-title`、`.u-close`），4 个弹窗组件引用，避免重复。
- `.modal`/`.panel` 背景用 `board_popup.png`：`border-image: url(/images/board_popup.png) <slice> fill / <width> round`，slice 按实际 10px 角值，浏览器微调；去掉扁平圆角卡片底。
- header 用 `popup_title.png` 做标题条背景（9-slice）。
- 关闭按钮替换为 `lottery_close` 三态（normal/`_over` hover/`_select` 按下）。

### B3 · 顶栏模块按钮（CityView）
文件：`frontend/src/views/CityView.vue`
- 武将 → `topbutton_hero`（up/on/down），军事 → `topbutton_army`（up/on/down）；用 `<span class="img-btn">` + CSS 背景贴图 + hover/按下切换上三层。
- 科技：`topbutton_battle` 语义不符（自带"战场"文字），改用**图标合成按钮**——`topicon_tactic.png` 图标 + 文字铺铜色边框，风格贴近 topbutton。
- 顶栏容器背景换原版 bar 风格（若无不直接对齐就保留变量色 + 铜边框）。

### B4 · Tab 皮肤
文件：`ArmyPanel.vue`（当前仅此处有双 tab：征兵/出征）
- 用 `channel_tab1`（up）、`_hl`(hover)、`_sl`(选中) 做占位图档（三态背景）。

### B5 · 资源条容器
文件：`ResourceBar.vue`
- 容器背景用 `board_tip.png`（20×20，9-slice）做小面板底色，图标已就位。

## 关键文件清单
- `frontend/src/styles.css` — 全局主题变量 + 通用 `.u-modal/.u-title/.u-close` 皮肤类
- `frontend/src/views/CityView.vue` — 顶栏模块按钮、背景
- `frontend/src/components/HeroPanel.vue` / `ArmyPanel.vue` / `TechnicPanel.vue` / `BuildingPanel.vue` — 弹窗/面板皮肤 + 关闭
- `frontend/src/components/ResourceBar.vue` — 容器皮肤
- `frontend/scripts/check-assets.mjs` — 新贴图引用核对（已有相对路径修复）
- 资源统一经 `frontend/src/assets/img.ts` 的 `img()` 引用

## 验证
1. `cd frontend && npm run check:assets`：新增贴图引用全部命中 public/images，无 MISSING。
2. `cd frontend && npm run build`（vue-tsc --noEmit + vite build）通过。
3. 浏览器 E2E（后端 8080 / 前端 5173 已在跑；账号 `hero_8954dde2` / `test123456`）：登录进城 → 顶栏模块按钮呈现原版贴图且 hover/按下有态；分别打开 科技/军事/武将 面板，确认 `board_popup/popup_title` 背景贴图加载、`lottery_close` 有关闭态、无 404、无控制台报错；截图留证。
4. 阶段性提交（中文 `asset: ...`，不 push），工作区保持干净。

## 风险与取舍
- topbutton 自带文字 → 武将/军事直接用原图；科技只能图标+文字合成（已获用户同意）。
- 9-slice 的 slice 数值需按 board_popup/popup_title 实际角大小在浏览器微调，容器尺寸自适应。
- 原版大窗口帧在 `swf/UIParts.swf` 中（Flash），无法直接取；用 `board_popup`(41×44)/`board_tip`(20×20) 小帧 9-slice 代替，观感略细但仍为原版贴图。