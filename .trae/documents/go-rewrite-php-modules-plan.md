# Go 重写计划：PHP 服务端 → Golang（核心玩法全量）

## Context

「热血三国」重写工程：legacy PHP（`server/`，243 文件/1778 函数，AMF RPC）逐模块迁移到 Go 后端（`backend/`，gin+sqlx）。现状：auth/city/building/technic/army/hero 六模块已有 REST 实现（31 端点），远程库 rxsg_test 已建 20 表并导入种子；**0 测试文件**。本次按用户决策：范围=核心玩法全量（排除渠道支付/独立聊天服/跨服）、API 沿用现有 REST 约定、测试全部连真库（config.yaml）。业务逻辑以三份走查文档为基准：[服务端逻辑-PHP全量走查.md](file:///c:/Users/wst_k/Desktop/workspace/rxsg/docs/服务端逻辑-PHP全量走查.md)、[服务端逻辑-PHP走查.md](file:///c:/Users/wst_k/Desktop/workspace/rxsg/docs/服务端逻辑-PHP走查.md)、[游戏流程-脚本分析.md](file:///c:/Users/wst_k/Desktop/workspace/rxsg/docs/游戏流程-脚本分析.md)。

铁律：1:1 复刻原逻辑与数值公式，不修原版 bug、不调平衡；不在 bug/设计缺陷上纠结。

## 复用的既有模式（所有新模块照抄）

- 骨架：`internal/<mod>/{service.go,handler.go}`，`NewService(*db.DB)` / `NewHandler(svc)` / `Register(rg *gin.RouterGroup)`，在 [main.go](file:///c:/Users/wst_k/Desktop/workspace/rxsg/backend/cmd/server/main.go) protected 组挂载
- DB：`internal/db`（FetchOne/FetchRows/Exec/Insert/Exists/Now，select \* + `model.Int/Int64/Str` 容错取值）
- 错误：`internal/httpx`（`{code,message}` 统一格式，BadRequest/Forbidden）
- 并发锁：`internal/lock`（对应 legacy newLockUser 同玩家串行）
- 结算：惰性 tick（进面板先 Settle，对应 legacy cfg_js Handle*）
- 路由约定：`/api/v1/cities/:cid/<mod>/...`（GET info / POST 动作）

## 模块批次（按依赖排序，每批 = DDL迁移 + service + handler + 真库测试 + 注册路由）

### M1 城市内政补全（CityFunc.php:75-439）
- 收税 `changeTax`（tax∈[0,100]，`morale_stable=100-tax-complaint`）、征收 `levyResource`（民望阈值拒绝、按人口×全局速率）、安抚 `pacifyPeople`（冷却+耗粮升 morale 降 complaint）、产出结算 `getCityProduct`（建筑等级+using_people+科技lv×10+将领内政+buff）
- 表：cities 加 tax/complaint/morale 列迁移；city_resources 对齐产出字段
- 路由：`/cities/:cid/city/{tax,levy,pacify,product}`

### M2 道具系统（GoodsFunc.php useGoods:28-294）
- cfg_goods 定义表 + user_goods 背包 + `useGoods` 按 gid 分发（资源类/加速类/经验类/募兵令/换皮）；被 M4/M5/M6/M7 依赖
- 表：`cfg_goods`、`user_goods`；种子从 legacy 数值表移植
- 路由：`/goods/{list,use}`

### M3 武将深化（HeroFunc.php / HeroExpr.php）
- 四维属性（统/内/武/智）：`regenerateHeroAttri:1397`（=base+add+装备+强化+镶嵌）、加点 `addHeroPoint:225`/洗点 `clearHeroPoint:274`、任命三职位（general/chief/counsellor，影响征兵速度与产出）
- 历练公式 1:1 修正：`hours×level×6000+rand(1,1000)`（修身）/`×12000`（闯荡）——替换现 ExprExpPerHour=40 简化模型（HeroExpr.php:310-317）
- 表：heroes 扩展四维+point 列、cfg_hero_growth 种子
- 路由：`/heroes/{detail,point/add,point/clear,assign}` + 改造 expr/start

### M4 酒馆招募（HotelFunc.php:234-322）
- 招募池生成（按酒馆等级、last_reset_recruit+blocksize 刷新节奏、活动將概率）、`recruitHero` 花费元宝、招贤榜
- 表：`tavern_pool`、cfg_hero_npc（NPC 武将模板）
- 路由：`/cities/:cid/tavern/{pool,recruit,reset}`

### M5 装备（EquipmentFunc.php / ArmorFunc.php）
- 穿戴加成链、强化 `doStrong:179`（成功率=`suc_value×(1+succ_add/100)`:328）、熔炼 `combineArmor:1488`（lv1-7=75/35/15/10/5/3/1%:1553）、耐久
- 表：`cfg_armor`、`user_armors`、`hero_armors`（穿戴位）
- 路由：`/armors/{list,wear,unwear,strong,combine}`

### M6 经济建筑（Market/Store/Barn/WorkShop/Shop）
- 市场 `buyFromMerchant:129-197`（配额/金价/扣金加资源）、仓库 `doGetStoreInfo:5-36`（等级+存储科技容量）/`modifyStoreRate:39-58`（比例≤100）、粮仓溢出、工坊器械、商城元宝购买
- 表：cfg_market 价格曲线、city_res_rate
- 路由：`/cities/:cid/{market,store,shop}/...`

### M7 战斗 1:1（OLdBattleCron.php + BattleFunc.php + ReportFunc）
- **替换 `army.resolveBattle` 简化模型**为原版回合引擎：初始化（speed/range/hp/ap/dp+四维加成+满统比例，:44-100）→ 回合循环 → 伤害 `shanghai=count×ap×ap/(ap+target_dp)`（:291-333，城墙/普通分算）→ 伤亡/胜负/掠夺（仓库保护比例）/占领（NPC vs 玩家城差异）
- 计策 TrickFunc、城防 DefenceFunc（加固耐久）
- 战报：battle_reports 扩展 content 字段（XML/JSON 明细，对齐 sys_report）
- 路由：现有 dispatch/fields/marches 结算逻辑重写 + `/reports/{list,detail,read}`

### M8 任务/成就（TaskFunc/AchivementFunc）
- 任务类型枚举、`checkGoalComplete:28`（事件点上报）、`completeTask:703` 领奖；成就触发点
- 表：cfg_tasks、user_tasks、cfg_achivements、user_achivements
- 路由：`/tasks/{list,complete}`、`/achivements`

### M9 单机活动（PKFunc/LotteryFunc/ShaChangFunc）
- PK 决斗（发起/应战/伤亡惩罚）、抽奖奖池权重（LotteryFunc:20 函数）、沙场（config/shachang.php 参数）
- 表：pk_records、cfg_lottery_pool、shachang_state
- 路由：`/pk/...`、`/lottery/draw`、`/shachang/...`

**明确排除**（本范围外，按用户决策）：**社交（联盟 UnionFunc/邮件 MailFunc/好友 FriendFunc）整块不做**、**排行榜 RankFunc 不做**、渠道支付（外部平台）、独立聊天服（ChatSocket）、跨服（AssembleKuaFu）、BattleNet 独立进程（其公式并入 M7 主进程结算）、防沉迷 Adult、GM WizardFunc。

## 迁移与数据

- 新增 `backend/migrations/0005_modules.sql` 起按批次追加（幂等 DDL + cfg 种子从 legacy `cfg_*` 数值移植，延续 bid 重映射策略）
- 用现有 `cmd/migrate` 执行；每批次先跑迁移再跑测试

## 测试（用户决策：全部真库）

- 新建 `backend/internal/testutil/db.go`：加载 config.yaml → 连接远程 rxsg_test → 提供 `NewTestDB(t)` + 唯一测试账号工厂（`t_<rand>` passport，bcrypt 固定密码，t.Cleanup 删数据）
- 全部测试文件打 `//go:build integration`；`go test -tags integration ./...` 为唯一测试命令（当前 0 测试 → M1 起每模块 service 层 ≥5 用例：正常路径/边界/校验失败/并发锁/结算幂等）
- 性能验收：`hey`/`ab` 对 `/api/v1/cities/5` 与 M7 战斗结算压测，与 PHP 版（如可本地起）或基线 RT 对比记录到 docs

## 节奏与验收

每模块完成标准：迁移可重放 + `go vet` 干净 + 真库集成测试全绿 + curl 冒烟（登录→操作→断言响应与 DB 状态）+ 在 `docs/复刻对照表.md`（新建）登记「PHP 函数 ↔ Go 函数 ↔ 公式差异」。

## 关键文件

- 修改：`backend/cmd/server/main.go`（挂新路由）、`internal/army/service.go`（M7 结算重写）、`internal/hero/service.go`（M3 公式修正）、`internal/city/service.go`（M1）
- 新增：`internal/{goods,tavern,equip,market,task,achieve,pk,lottery,shachang,report}/`、`internal/testutil/`、`migrations/0005+`
