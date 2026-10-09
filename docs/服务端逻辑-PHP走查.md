# 热血三国 · Legacy PHP 服务端逻辑走查

> 依据：`server/` 目录 243 个 PHP 文件静态走查（amfphp 网关 + `game/*Func.php` 函数库 + `cron/` 定时任务）。
> 每条结论附 文件:行号 证据。用途：作为 Go 重写版的行为对照基准（**只读走查，未修改任何 legacy 代码**）。

## 0. 架构总览

```
Flash 客户端 ──AMF──▶ server/amfphp/gateway.php ──▶ 服务类
                        Command::sendCommand        (game/Command.php)
                        CityCommand               (城市场景 RPC)
                        BattleCommand             (战斗场景 RPC)
                             │ function_exists($name) 反射式路由
                             ▼
                        game/*Func.php（240 个全局函数，按 module 命名）
                             │ sql_query() / lib/DB.php
                             ▼
                        MySQL（sys_* 持久表 + mem_* 内存态表）
     ┌─────────────────────────────────────────────┐
     │ BattleNet（独立战斗服）：回合结算在其上计算， │
     │ 主服仅 initBattle 下发、sendBattleReport 回收 │
     └─────────────────────────────────────────────┘
     cron / NaoZhong.php 闹钟驱动：行军到达、建造完成、科技完成、历练结算
```

- 网关：`amfphp/gateway.php:103-152`（生产常量、服务类路径、字符集、gzip、loose mode）。
- 鉴权根：`checkUserAuth($uid,$sid)` 比对 `sessions/{uid}` 文件 + `sys_sessions` 表：`game/global.php:4-10`。
- 报文解密：xxtea（`game/decode.php:40-74`，`decodeDataNow($data,$key)`，密钥在 `config/key.php`）。

---

## 1. 请求路由机制

### 1.1 sendCommand 报文解析（game/Command.php:47-94）
```
param = [uid, sid, type, ...]
type==1 (DIALOG): [receiverID, receiverParam, commandFunc, ...args]
                  → function_exists($commandFunc) ? $commandFunc($uid,$cid,$param)
type==2 (GLOBAL): [commandFunc, ...args] → $commandFunc($uid,$param)
```
- 白名单：仅 `$GLOBALS['openfunc']` 内函数可被调用（防任意函数执行）。
- **并发锁**：每次调用 `newLockUser($uid,$commandFunc)` / `newUnlockUser`（`userlock/` 目录文件锁），同一玩家串行处理——重写版以单用户事务/内存锁对应。
- 异常包装：`command_not_found` / `command_exception`（:81-93）。
- 动态加载：`WizardFunc.php` 支持运行期扩展命令（:104-153）。

### 1.2 三入口差异
| 入口 | 报文 | 路由 | 证据 |
|---|---|---|---|
| `Command::sendCommand` | [uid,sid,type,…] | 通用业务函数 | `Command.php:47-94` |
| `CityCommand` | [uid,cid,sid,type] | type→`getCityInfoRes/Hero/Army/Defence`，更新在线 | `CityCommand.php:11-67` |
| `BattleCommand` | [uid,cid,sid,type] | 战斗服数据拉取（type=0→`getCityInfoRes`），含维护检查 | `BattleCommand.php:8-43` |

### 1.3 命令→函数命名约定
客户端 action 名即服务端全局函数名（如 `startUpgradeBuilding`、`startDraftQueue`、`recruitHero`），分布于对应 `*Func.php`；`interface.php`/`common_interface.php` 声明接口契约。

---

## 2. 登录与角色

### 2.1 doLogin（game/Login.php:28-159）
1. 解析 `loginType`（0=passport 渠道验证 / 1=其他）、`passtype`（渠道号）。
2. 校验版本与服务器状态（维护/开服时间）。
3. 渠道验证：`passport/{4399,renren,myrxsg,passtype,adult,...}.php` 各自 HTTP 回调验证（`其它类型的用户/` 为更多渠道变体）。
4. 新角色：写 `sys_user`、`sys_sessions`、`sys_online`，创建 `sessions/{uid}` 文件。
5. `checkQueue` 排队（对应客户端 `onCheckQueue`）。

### 2.2 创建角色（game/createrole.php）
初始城池 + 默认建筑（官府/农田等基础等级）+ 初始武将 + 初始资源写库；`addPassport.php` 绑定渠道账号。

### 2.3 在线与账号状态
`account_status.php`（封号/冻结）、`activeuser.php`（激活）、`getUserLogin/GetUserOnline/GetCCU` 等统计脚本读 `sys_online`。

---

## 3. 数据层

- `lib/DB.php`、`mysql.php`、`database.php`、`db_utils.php`：`sql_query()` 全局封装（mysql_* 风格，字符串拼接 SQL——legacy 注入风险原样保留，不修复）。
- 表命名：`sys_*`（持久：sys_user/sys_city/sys_building/sys_troops/sys_report/sys_hero…）、`mem_*`（内存态镜像：mem_city_resource/mem_building_upgrading/mem_city_draft/mem_technic_upgrading/mem_city_reinforce…）、`cfg_*`（配置：cfg_building_level/cfg_technic_level/cfg_soldier/cfg_soldier_condition/cfg_hero_level…）。
- 会话：文件 `sessions/{uid}` + 表 `sys_sessions` 双轨。
- 缓存：`lib/Cache/Lite`（PEAR 系）。

---

## 4. 主城核心玩法算法

### 4.1 建筑（BuildingFunc.php:355-719）
- **建造/升级同一入口** `startUpgradeBuilding`（:355-568）：校验目标等级、当前 `state`、等级上限（官府等级约束其他建筑，`BUILDING_MAX_LEVEL=10`）、资源、人口 `using_people`、前置建筑/科技/道具；耗时 = `cfg_building_level.upgrade_time`；写 `sys_building.state=1` + `mem_building_upgrading.state_endtime`。
- `stopUpgradeBuilding`（:571-603）：仅 `state=1` 可停，按规则回退资源。
- 拆除（:643-719）：`state=1`（在建）或 `state=2`（已拆）禁止；拆除置 `state=2` 并记录起止时间（重建冷却）。
- **状态机**：`0 正常 / 1 建造·升级中 / 2 拆除中`。

### 4.2 科技（TechnicFunc.php:50-229）
- 展示与校验（:50-167）：读 `cfg_technic / sys_technic / sys_city_technic`，`state_timeleft` 由 endtime-now 计算；上限 10 级；资源需求 `cfg_technic_level`；前置（建筑/科技/任务）校验；`has_one_upgrading` 时禁止并行。
- `startUpgradeTechnic`（:180-229）：本城已有研究任务则拒绝（单队列）。
- 书院等级决定**可并行研究数 collegeCount**（客户端 CollegeDialog 同源展示）。

### 4.3 征兵（SoldierFunc.php:9-280）
- **速度公式** `getSoldierSpeedRate`（:9-83）：
  ```
  speed_add = 制造科技 lv*10(器械) / 练兵技巧 lv*10(士兵) + 将领 bravery
  finalSpeed = 1 / (1 + 0.01*speed_add + speedfactor) * (1 - skill_rate)
  司隶地区特殊 -30%
  ```
- `startDraftQueue`（:245-280）：参数含 `double`（=1 资源翻倍、速度系数 1.5）；校验兵营等级、`cfg_soldier_condition` 前置、资源、人口 `people_need`；队列 `state=0 排队 / 1 训练中`（mem_city_draft）。
- 完成结算：`ReportCron.php:347-464` 扫描 `mem_city_draft` 到期入城。

### 4.4 出征与行军（BattleFunc.php / TroopFunc.php）
- 行军时间（`BattleFunc.php:1507-1589`）：
  ```
  单程时间 = pathLength / minSpeed + 宿营时间
  pathLength = 162000（两格距离常量；格坐标差 × GRID_DISTANCE=60000 折算）
  minSpeed = 部队最慢兵种 speed
  逐鹿中原活动固定 60 秒
  ```
  写 `sys_troops.pathtime / endtime`。
- 部队回调状态机（`TroopFunc.php:357-394`）：`state=0` 去程（重算 endtime）、`state=2/4` 返程（按 pathtime 更新双方城市资源）、`state=3` 战斗中**禁止召回**。
- 出征类型：掠夺/占领/增援/巡逻（`battleTroopDispatch:1965`、`battlePatrol:2132`、`addArmyFromReinforce:2090`）。
- 加速行军：`fasterArmy:2816` / `fastRemoteArmy:2870`（道具抵扣）。

### 4.5 战斗结算（BattleNet 独立战斗服）
- 主服职责边界：`BattleNetServices.php` 提供 `initBattle:28`（下发双方城池/武将/兵种/装备数据）、`sendBattleReport:138`（战报回写 `sys_report`）、`addHeroExpService:181`、`reduceHeroArmorHP:288`、`quitBattleNet:202`。
- **回合/伤害/暴击等结算在 BattleNet 进程内计算**（`BattleNetFunc.php`/`BattleNetGateway.php` 为通信层；`OLdBattleCron.php` 81KB 为旧版同进程结算引擎，含完整回合公式）。
- 战报：`ReportFunc.php` 查询 + `ReportCron.php` 定时生成；`report*.php` 为 Web 报表页。
- 重写版差异：Go 版以 `army.resolveBattle` 简化模型（战力对比+按比例伤亡）替代，非逐回合。

### 4.6 城池资源与税收（CityFunc.php）
- 产出公式（:75-120）：
  ```
  基础产出 = f(资源建筑等级, using_people)
  科技加成 = level*10（对应资源科技）
  其他加成 = 将领内政/统率、俸禄、状态符
  → food_add_base / wood_add_base / rock_add_base / iron_add_base
  ```
- 税收（:269-280）：`tax ∈ [0,100]` 写 `mem_city_resource.tax`；
  `morale_stable = GREATEST(0, LEAST(100 - tax - complaint, 100))` —— 税率越高民望越降。
- `getCityInfoRes/Hero/Army/Defence`（CityCommand 四分支）聚合返回。

### 4.7 野地（GroundFunc.php）
- 守军按野地等级从 `cfg_soldier` 生成；占领（改 owner_uid）与掠夺（只拿资源）两条路径；刷新逻辑由 cron 驱动（`updateMap.php` 地图块维护）。

---

## 5. 武将与成长

- **升级**（HeroFunc.php:121-215）：阈值读 `cfg_hero_level.total_exp/upgrade_exp`；升级后 `regenerateHeroAttri` + `insertHeroBaseAdds` 重算属性（四维：统率/武力/智力/政治 + 成长）。
- **历练**（HeroExpr.php）：
  - 开始（HeroFunc.php:2007-2068）：校验时长/次数/类型，费用 `hour_money/hour_gold × 小时`，插 `sys_hero_expr`，武将置历练状态。
  - 结算（HeroExpr.php:17-50, 307-321）：cron `HandleHeroHexprs` 扫描；经验公式 **`expAdd = hours × level × 6000`（普通）或 `× 12000`（高级）**——重写版常量 `ExprExpPerHour=40` 为简化模型，数值不同源。
- **招募**（HotelFunc.php:234-322）：删除旧池→按酒馆等级随机生成武将池（`last_reset_recruit` + `blocksize` 控制刷新），活动武将概率、招贤榜记录。
- **职位**：主将/城守/军师三职位影响征兵速度（bravery）、产出（内政）等。
- 技能：`HeroSkillFunc.php`（武将技能加成，战斗公式中 `skill_rate` 减速项来源）。

---

## 6. 经济与装备

| 系统 | 文件 | 核心逻辑 |
|---|---|---|
| 道具 | GoodsFunc.php（188KB） | 道具表 `sys_goods`/`cfg_goods`，使用效果按 type 分发（加速/资源/经验/宝箱） |
| 市场 | MarketFunc.php | 配额售卖、加速（对应客户端 `ID_BUILDING_MARKET` dialog 命令） |
| 仓库 | StoreFunc.php | 容量=仓库等级曲线；超出部分可被掠夺（保护比例） |
| 商城 | ShopFunc.php | 元宝消费→道具 |
| 充值 | pay*.php + 51SDK/ | 渠道回调（51pay/51paysucc/51shipping）→加元宝 |
| VIP | VipFunc.php | 等级权益（队列数/产量加成） |
| 装备 | EquipmentFunc.php + ArmorFunc.php | 打造（铁匠铺）、强化耐久（战斗服 `reduceHeroArmorHP` 回调扣耐久）、穿戴属性加成 |

---

## 7. 社交与活动

- **联盟** UnionFunc.php（98KB）：创建/审批/成员/联盟科技/援助；聊天走独立 ChatServer（`config/chathost.php` 配置地址，客户端 ChatSocket 直连）。
- 邮件 MailFunc.php（附件领取走 GoodsFunc 分发）、好友 FriendFunc.php。
- 活动矩阵：竞技场（BattleFunc `joinToPVPBattle:3393`）、PK 决斗 PKFunc.php、洛阳战 LuoyangFunc.php、沙场 ShaChang*.php + `config/shachang.php`、结婚 MarrySystemFunc.php、过关斩将/抽奖 LotteryFunc.php、成就 AchivementFunc.php、任务 TaskFunc.php（115KB，任务类型枚举+完成判定由事件点调用）、计策 TrickFunc.php。
- 跨服：AssembleKuaFu.php（跨服集结）。

---

## 8. 定时任务与闹钟

- **调度入口** `cfg_js.php:11-28`（每次 RPC 顺带惰性 tick）：依次 `HandleTroop`（行军到达回调）、`HandleBattle`、`HandleHeroHexprs`（历练结算）等——**混合惰性结算**，非纯 crontab。
- `NaoZhong.php:6-68`：闹钟类型（建筑升级完成/拆除完成/科技完成/招募完成/城防完成），到期 `log_building_technic_state` 推送状态变化。
- `ReportCron.php:347-464`：扫描 `mem_technic_upgrading / mem_city_draft / mem_city_reinforce` 到期记录执行结算。
- `cron/upbattle|upbuilding|upround|upTechnic|uptroop`：外部 cron 脚本占位（内容为空/极小，实际结算在 cfg_js 惰性触发）。
- `OLdBattleCron.php`（81KB）：旧版同进程战斗结算引擎（BattleNet 化之前的完整回合公式库，保留作对照）。
- `updateMap.php`：世界地图块数据维护。

---

## 9. 表清单（走查中确认的核心表）

| 域 | 表 |
|---|---|
| 账号会话 | sys_user, sys_sessions, sys_online, passport(渠道) |
| 城池 | sys_city, mem_city_resource, sys_city_hero, city_soldiers(驻军) |
| 建筑科技 | sys_building, mem_building_upgrading, sys_technic, sys_city_technic, mem_technic_upgrading |
| 军事 | sys_troops(pathtime/endtime/state), mem_city_draft, mem_city_reinforce, ground(野地) |
| 武将 | sys_hero, sys_hero_expr, cfg_hero_level |
| 战斗战报 | sys_report(from_battlenet 标记来源), battlefield |
| 经济 | sys_goods, cfg_goods, 商店/市场/仓库表 |
| 配置 | cfg_building_level, cfg_technic_level, cfg_soldier, cfg_soldier_condition |

---

## 10. 与 Go 重写版对照结论

| 项 | Legacy PHP | Go 重写版 | 一致性 |
|---|---|---|---|
| 命令路由 | `function_exists` 反射 + openfunc 白名单 + 文件锁 | REST 路由 + SessionStore | 等价简化 |
| 建筑状态机 | 0/1/2 + mem_building_upgrading | buildings.state + state_start/end_at | ✓ |
| 征兵速度 | 科技 lv*10 + bravery + 司隶 -30% | 仅 bravery（`speedRate`） | 简化（无科技/地域表） |
| 行军时间 | pathLength=162000/minSpeed | 固定 marchDistance=3000 折算 | 简化（无世界坐标） |
| 战斗结算 | BattleNet 逐回合 + OLdBattleCron 公式 | resolveBattle 战力对比模型 | **不同源**，如需 1:1 应移植 OLdBattleCron 公式 |
| 历练经验 | hours×level×6000/12000 | ExprExpPerHour=40 | **不同源** |
| 税收民望 | morale=clamp(100-tax-complaint) | 未实现 complaint 项 | 部分 |
| 结算驱动 | cfg_js 惰性 tick + NaoZhong 闹钟 | SettleDraft 等惰性结算 | ✓ 同思想 |
| 并发控制 | newLockUser 文件锁（同玩家串行） | 事务 + 单实例 | 等价 |

> 数值差异项（战斗公式、历练公式）为重写版有意简化；若后续要求 1:1，对照源分别位于 `OLdBattleCron.php` 与 `HeroExpr.php:307-321`。
