# 热血三国 · Legacy PHP 服务端全量走查

> 覆盖 `server/` 全部 **243 个 .php 文件**（含 amfphp 框架 62、lib 13、config 5、game 163）。逐文件登记，共 **1778 个函数 / 112 个函数式文件**。
> 本文是 [服务端逻辑-PHP走查.md](file:///c:/Users/wst_k/Desktop/workspace/rxsg/docs/服务端逻辑-PHP走查.md)（机制摘要版）的**全量逐文件版**。证据格式 `文件:行号`，路径省略前缀 `server/`。
> 只读走查，未修改任何 legacy 代码。

## 0. 文件分布与规模

| 目录 | 文件数 | 说明 |
|---|---:|---|
| `amfphp/` | 62 | AMFPHP 2.x RPC 框架（含 19 个 adapter） |
| `lib/` | 13 | 数据库封装 + PEAR + Cache_Lite |
| `config/` | 5 | 运行配置 |
| `game/`（根） | 158 | 业务函数库 + 渠道 + 运维脚本 |
| `game/51SDK·agents·passport·xiaonei·utils·secret·sessions` | ~35 | 渠道 SDK / 会话 |
| `game/`（cron·report_data 等） | 少量 | 定时任务占位 / 报表数据 |

---

## 1. 框架层 `amfphp/`（62 文件）

### 1.1 入口与请求链
| 文件 | 职责 |
|---|---|
| `gateway.php:103-152` | **AMF 总入口**：加载 globals，配置服务类路径/字符集/gzip/loose mode → `$gateway->service()` |
| `globals.php` | 框架全局配置（生产常量） |
| `json.php` / `xmlrpc.php` | JSON-AMF / XML-RPC 备用网关入口（游戏未用） |
| `DiscoveryService.php` | Flash Builder 服务发现（IDE 工具用，运行时无关） |
| `core/amf/app/Gateway.php:51-168` | 网关核心：filter 链（deserial→auth→batch→debug→serialize）+ action 链（adapter→class→security→exec）；读 raw POST |
| `core/amf/app/Actions.php:21-214` | 服务方法解析（`Service.method` → classPath/methodName）+ 执行；**普通调用前先 `doBloodFilter()`**（:146-214） |
| `core/amf/app/BloodWarFilter.php:1-16` | ★**游戏定制唯一改动**：`escape_single_quote()`+`doBloodFilter()`，对入参字符串 `addslashes()`——全局 SQL 注入缓解层 |
| `core/amf/app/Filters.php` | 默认过滤器链实现 |
| `core/shared/app/php5Executive.php:55-70` | `doMethodCall()`：反射调用服务方法，异常包装为 MessageException |
| `core/shared/app/{BasicActions,BasicGateway,Constants,Globals,php4Executive}.php` | 框架基础（php4Executive 为 PHP4 兼容，未用） |

### 1.2 AMF 序列化 `core/amf/`（10 文件）
`io/{AMFBaseDeserializer,AMFDeserializer(16KB),AMFBaseSerializer,AMFSerializer(28KB)}.php`——AMF0/AMF3 编解码；`util/{AMFObject,DateWrapper,DescribeService,PageAbleResult,TraceHeader,WrapperClasses}.php`——请求对象/调试头/分页结果封装。均为标准 AMFPHP，无游戏逻辑。

### 1.3 共享层 `core/shared/`
- `util/`：`MethodTable.php`(14KB 方法表缓存)、`JSON.php`(23KB Services_JSON)、`Authenticate.php`(HTTP 鉴权)、`CharsetHandler.php`(编码转换)、`MessageBody/Header.php`、`NetDebug.php`、`functions.php`、`CompatPhp4/5.php`。
- `exception/`：`MessageException.php`(AMF 异常→客户端)、`php4/5Exception.php`。
- `adapters/`（19 文件）：`RecordSetAdapter.php` + 各数据库适配器（mysql/mysqli/pdo/pgsql/oci8/mssql/sqlite/…各 1KB）——**仅用于把查询结果适配为 AMF RecordSet，游戏主路径不用**（游戏用 lib/DB）。

### 1.4 `core/json/`、`core/xmlrpc/`
各 2 文件（Actions/Gateway），JSON-RPC 与 XML-RPC 变体入口，游戏未启用。

> **框架层结论**：唯一游戏定制是 `BloodWarFilter.php` 的 addslashes 全局转义；其余为原生 AMFPHP 2.0。

---

## 2. 数据层 `lib/`（13 文件）+ `config/`（5 文件）

| 文件 | 职责与要点 |
|---|---|
| `lib/DB.php`（44KB） | ★主数据封装：`sql_query/sql_fetchrow/sql_fetch_one/sql_fetch_one_cell/sql_fetch_rows/insert/update` 等全局函数族；mysql_* 直连、字符串拼接 SQL、`utf8` 设置 |
| `lib/database.php`（22KB） | PEAR::DB 风格封装类：`DB::connect()`（:52-69）、`DB_FETCHMODE_ASSOC`、`verifyQuery()`（:89-113，未转义引号/分号告警）、`query()`（:214-222）——备用数据层 |
| `lib/db_utils.php` | 表级 CRUD helper |
| `lib/mysql.php` | mysql_* 薄封装（连接复用） |
| `lib/DB/common.php`(73KB)/`DB/mysql.php`(33KB) | PEAR::DB 驱动（database.php 依赖） |
| `lib/PEAR.php`(34KB) | PEAR 基类（error handling） |
| `lib/Cache/Lite.php`(26KB)+`Lite/{File,Function,Output}.php` | 文件缓存（配置/模板缓存） |
| `lib/curl.php` / `httpUtils.php` | HTTP 外呼（渠道验证/支付回调） |
| `config/db.php` | 数据库连接参数（host/port/user/pwd/name） |
| `config/key.php` | ★xxtea 密钥 + 渠道密钥（与客户端 `Command.as` 加密对应） |
| `config/key_constants.php` | 常量版密钥定义（与 key.php 二选一加载） |
| `config/chathost.php` | 聊天服务器地址（客户端 ChatSocket 直连用） |
| `config/shachang.php` | 沙场玩法参数 |

---

## 3. RPC 核心与全局（`game/` 根，框架侧）

| 文件 | 函数数 | 要点 |
|---|---:|---|
| `Command.php` | 1 | `sendCommand:47-165`：解析 `[uid,sid,type]`；type=1 dialog（receiverID+commandFunc）、type=2 global；`openfunc` 白名单 + `newLockUser` 文件锁 + 耗时日志 + 异常包装 |
| `CityCommand.php` | 1 | 城池 RPC：type→`getCityInfoRes/Hero/Army/Defence`，更新在线 |
| `BattleCommand.php` | 1 | 战斗 RPC：维护检查 + type=0→`getCityInfoRes` |
| `global.php` | 7 | `checkUserAuth:4-10`（sessions/{uid} 文件+sys_sessions 双轨）、`getCityInfoRes`（资源/在线/buff 聚合） |
| `common.php` | - | `$GLOBALS` 初始化：IP/时间/编码/`GAME_SPEED_RATE`/资源税收价格常量/建筑 ID 枚举（L47-66） |
| `common_interface.php` | 5 | `InterfaceConstants` + `checkSignValid()`（合法 IP+MD5 key+时间窗签名） |
| `interface.php` | - | 客户端接口契约声明 |
| `dbinc.php` | - | 数据库引入引导 |
| `decode.php` | 5 | `xxtea_decrypt`+`decodeDataNow:40-74`（报文解密） |
| `RSA.php` | 15 | RSA 加解密库（支付签名用） |
| `secret/SGCrypto.php` | 6 | 加密工具（渠道/敏感数据） |
| `checkuser.php`/`forlogin.php`/`pass.php` | - | 登录前置检查/验证码占位 |
| `ConfigInfo.php`/`cfg_end.php` | 1/0 | 配置收尾（0KB 空壳） |
| `HttpClient.php` | 26 | HTTP 客户端（外呼渠道/战斗服） |
| `ExternService.php` | 6 | 外部服务桥（`getImagePath` 等 JS 桥对应） |
| `DataCenter.php`/`DataCenterStastic.php` | 8/0 | 数据仓库统计 |

---

## 4. 登录 / 创角 / 账号

| 文件 | 函数数 | 要点 |
|---|---:|---|
| `Login.php` | 10 | `doLogin:28-159`：版本/服务器状态校验→loginType 分支→`@include("./passport/$passtype.php")` 渠道验证→写 sys_user/sys_sessions/sys_online→建 sessions/{uid}；`checkQueue` 排队；`getLoginAnnouncement` |
| `createrole.php` | 7 | 新角色：初始城池+基础建筑+初始武将+初始资源写表 |
| `addPassport.php` | - | 渠道账号绑定 |
| `xiaonei_createrole.php`/`xiaonei_create_js.php` | - | 校内(renren)渠道创角变体 |
| `account_status.php` | 1 | 封号/冻结状态 |
| `activeuser.php`/`GetActivatedUsers.php`/`GetValidUsers.php`/`GetTransferUsers.php` | 1/1/1/3 | 激活/有效/转移用户查询 |
| `junzhu.php` | - | 君主（VIP/官职）逻辑 |
| `adult.php`（passport）/`AdultFunc.php` | 1/5 | 防沉迷实名验证 |
| `passport/{12ha,4399,myrxsg,passtype,renren,testPassport}.php` | ~1 | 各渠道验证协议（签名/回调 URL/字段映射）；`其它类型的用户/` 6 文件为历史渠道变体备份 |
| `agents/`（11 文件） | - | 官方渠道 SDK：`AgentServiceFactory` 路由→`official/{OfficialService(15),MailService(8),TimeInfoService(9),UnionService(5),UserService(7),Verify,Data}`、`implay/ImPlayService(8)`、`tongxuewang/TongxueService(7)`；`BaseService(17)` 公共 HTTP+签名基类 |
| `get91WanUserInfo.php`/`xget_info_xof.php`/`xrxget_info_xofrx.php` | - | 91wan/其他联运平台用户信息拉取 |

---

## 5. 主城玩法

| 文件 | 函数数 | 核心逻辑与公式 |
|---|---:|---|
| `CityFunc.php` | 29 | `getCityInfo:14-45`（四分支+被入侵判定）；`getCityProduct:75-234` 产出=f(建筑等级,using_people)+科技lv*10+将领内政/统率+buff+技能；`setCityProductRate:237-291`；`changeTax` → `morale_stable=100-tax-complaint`；`levyResource:345-405`（征收，低民望拒绝）；`pacifyPeople:406-439`（安抚：耗粮升 morale 降 complaint） |
| `BuildingFunc.php` | 15 | `getBuildingSpeedRate:15-67`（科技+城守内政+buff+技能+修为）；`getBuildingMaxLevel:97-236`（按城型上限）；`startUpgradeBuilding:354-568`（坐标/唯一/上限/资源/人口/官府等级/并行数/前置全校验→扣资源写 sys_building+mem_building_upgrading）；`stopUpgradeBuilding:571-603`；拆除:643-719（state 0/1/2 状态机） |
| `TechnicFunc.php` | 6 | `:50-167` 展示+校验（cfg_technic/sys_technic/sys_city_technic，上限10，has_one_upgrading 禁并行）；`startUpgradeTechnic:180-229`；collegeCount 并行研究数 |
| `SoldierFunc.php` | 16 | `getSoldierSpeedRate:9-83`：`finalSpeed=1/(1+0.01*(科技lv*10+bravery)+speedfactor)*(1-skill_rate)`，司隶-30%；`startDraftQueue:245-280`（double=1→资源×2、速度×1.5；cfg_soldier_condition 前置；state 0/1） |
| `GroundFunc.php` | 42 | 野地生成/占领/掠夺/放弃/守军刷新（等级→cfg_soldier 数量） |
| `WorldFunc.php` | 22 | 世界地图块加载（GRID_DISTANCE=60000 坐标→地块 SQL）、行军线、迁入迁城 |
| `updateMap.php` | 4 | 地图块数据维护 |
| `StoreFunc.php` | 3 | `doGetStoreInfo:5-36`（仓库等级+存储科技+sys_city_res_add 容量）；`modifyStoreRate:39-58`（四资源比例≤100） |
| `MarketFunc.php` | 19 | `buyFromMerchant:129-197`（配额/金价/元宝礼金/用户锁/日志/任务进度）；商人刷新 |
| `BarnFunc.php` | 5 | 粮仓（溢出与损耗） |
| `OfficeFunc.php` | 10 | 官署（俸禄/收税命令/加速） |
| `WorkShop.php` | 4 | 工坊（器械制造） |
| `DefenceFunc.php` | 8 | 城防加固（耐久/城防兵） |
| `BufferFunc.php` | 7 | buff 系统（增益状态挂载/到期） |
| `CityMergeFunc.php` | 14 | 城市合并（迁城重叠处理） |

---

## 6. 军事 / 战斗

| 文件 | 函数数 | 核心逻辑 |
|---|---:|---|
| `BattleFunc.php` | **105** | 出征 `startBattleTroop:1305-1509`（战场/城池/荣誉/兵力/军旗/将领/兵量校验）；行军 `:1507-1589`（单程=pathLength(162000)/minSpeed+宿营；逐鹿60s）；召回 `callBackArmy:1751-1818`、`callBackToField:1865-1920`（区分赤壁远程）；增援 `addArmyFromReinforce:2090`、dispatch:1965、patrol:2132；加速 `fasterArmy:2816-2866`（道具136，endtime-10s）、fastRemote/Menghuo/ChiBi:2783-2928；副本 `joinToBattle:3206-3357`（邀请/重复/跨服队列/上限/荣誉/任务）、joinToPVPBattle:3393、joinToBattle9001:3364；战利 `dropBattleGoods:2603`；功勋 `getBattleNetExploit:677`、`medalChange:903`；`enhanceArmy:3565`；`checkBattleResult:3624`；`quitBattle:2266` |
| `OLdBattleCron.php` | 13 | ★旧版同进程回合引擎（1:1 对照源）：`:44-100` 初始化双方 speed/range/hp/ap/dp + 将领四维加成 + 满统兵力比例；`:291-333` **伤害公式 `shanghai=count*ap*ap/(ap+target_dp)`**，城墙/普通分别算 siwang 与剩余兵 |
| `BattleNetServices.php` | 40 | 主服↔战斗服协议全函数：`initBattle:28`、`initChibi:55`、`getHeroInfo(4Chibi):65/107`、`checkChibiHero:124`、`sendBattleReport:138`(写 sys_report from_battlenet=1)、`sendCBBattleReport:154`(=2)、`sendCBBattleMail:170`、`addHeroExpService:181`、`sendTroopAlarm:187`、`saveHeroToCity:193`、`quitBattleNet:202`/`quitChibiNet:231`/`quitBattleNetByUid:336`、`reduceBattleGoods:256`、`reduceHeroArmorHP:288`、`getHeroAttriAndBuf:277`、`isInBattle:301`、`getPt*`（平台对战信息 :341-388） |
| `BattleNetFunc.php`/`BattleNetGateway.php`/`BattleNetCallBack.php` | 5/1/1 | 战斗服 socket 通信/网关/回调 |
| `ChibiNetFunc.php` | 1 | 赤壁（小兵）战斗服桥 |
| `TroopFunc.php` | 20 | 部队状态机 `:357-394`（state 0去程/2·4返程按pathtime更新双方资源/3战斗中禁回调）；`reGeneratePathTime:2723` |
| `call_back_troops.php` | - | 行军到期回调入口（cron 调用） |
| `NaoZhong.php` | 10 | 闹钟类型枚举（建筑升级/拆除/科技/招募/城防完成）+ `log_building_technic_state` 推送 |
| `TrickFunc.php` | 26 | 计策全类型与效果公式（战斗前配置，配合 ID_TACTICS_DIALOG） |
| `PKFunc.php` | 43 | 决斗/PK：发起/应战/惩罚/红名/荣誉 |
| `ShaChang.php`/`ShaChangFunc.php` | 18/6 | 沙场玩法（config/shachang.php 参数） |
| `LuoyangFunc.php` | 23 | 洛阳争夺战（报名/攻城/奖励） |
| `AssembleKuaFu.php` | 16 | 跨服集结 |
| `WizardFunc.php` | 8 | GM/调试命令（运行期动态加载，见 Command.php:104-153） |

---

## 7. 武将 / 装备

| 文件 | 函数数 | 核心逻辑 |
|---|---:|---|
| `HeroFunc.php` | 53 | 四维（统率/内政/勇武/智谋）：`upgradeHero:121-215`（cfg_hero_level.total_exp/upgrade_exp 阈值）；`addHeroPoint:225`/`clearHeroPoint:274`（加点/洗点）；`regenerateHeroAttri:1397`（属性=base+add+装备+强化+镶嵌，如 bravery=bravery_base+bravery_add+braveryAdd :1413）；历练开始 `:2007-2068`（hour_money/hour_gold 计费）；任命三职位 |
| `HeroExpr.php` | 53 | 历练结算：`finishHeroExpr:42`；★经验 `getHeroBaseExp:310-317`——修身 `hours*level*6000+rand(1,1000)`、闯荡 `hours*level*12000+rand(1,1000)`；随机事件 |
| `HeroExprinc.php` | - | 历练引入引导 |
| `HeroSkillFunc.php` | 10 | 技能类型/学习/升级规则（战斗 skill_rate 来源） |
| `HotelFunc.php` | 53 | 酒馆招募 `:234-322`（删旧池→按等级随机生成→last_reset_recruit+blocksize 刷新节奏→活动將概率→招贤榜）；重置/征辟 |
| `EquipmentFunc.php` | 42 | 装备：`doStrong:179`（强化成功率=`cfg_strong_probability.suc_value*(1+succ_add/100)` :328）；`combineArmor:1488`（熔炼概率 lv1-7=75/35/15/10/5/3/1% :1553）；耐久/穿戴 |
| `ArmorFunc.php` | 38 | 防具/坐骑装备属性与套装 |
| `OffloadArmor.php` | 2 | 卸下装备 |
| `npc_armor_script_kjkjdfqeijioja.php` | 5 | ★定性：**NPC 城守将装备生成脚本**（热血套/名将套分配），乱码文件名仅为混淆，**无后门/注入** |

---

## 8. 经济 / 道具

| 文件 | 函数数 | 核心逻辑 |
|---|---:|---|
| `GoodsFunc.php` | **133** | 道具总库：`useGoods:28-103` 按 gid 分发（神农锄/鲁班斧/开山锤/玄铁炉=资源；税吏鞭/徭役令/募兵令/南蛮山越匈奴战书/王者兵符/城池换皮/成长卷 :237-294）；加速类/经验类/宝箱类；道具表 sys_goods/cfg_goods |
| `ShopFunc.php` | 12 | 商城分类/元宝定价/购买入库 |
| `VipFunc.php` | 4 | VIP 等级权益（队列数/产量） |
| `RewardFunc.php` | 3 | 奖励发放 |
| `LotteryFunc.php` | 20 | 抽奖奖池权重算法 |
| `pay.php` | - | `:4-67` 到账：解析 p→IP/签名校验→pay_log→sys_user.money+log_money+pay_day_money |
| `pay_gold.php`/`rexue-pay.php`(5)/`paygift.php`/`actpaygift.php` | - | 元宝/赠送活动 |
| `51pay/51paysucc/51send/51shipping.php`/`51utils.php`(16) | - | 51wan 联运支付链 |
| `51SDK/{51api_php5_restlib(26),openapp_51(9),appinclude}.php` | - | 51 平台 REST SDK |
| `xiaonei/{guid.class(8),xiaonei-util(4),rexue-pay-success}.php` | - | 校内网支付/用户 GUID |
| `getMonthMoney/GetCoinRem/GetSpendCoinInfo/GetUserPay.php` | - | 消费统计 |

---

## 9. 社交 / 任务 / 活动

| 文件 | 函数数 | 核心逻辑 |
|---|---:|---|
| `UnionFunc.php` | 67 | 联盟：`createUnion:54`、`applyJoin:159`、`updateUnionRank:114`；成员/贡献/科技/援助/战争状态 |
| `MailFunc.php` | 17 | 邮件收发/附件（走 GoodsFunc 分发） |
| `FriendFunc.php` | 8 | 好友增删查 |
| `RankFunc.php` | 16 | 排行榜（多榜类型+刷新） |
| `TaskFunc.php` | 54 | 任务：`checkGoalComplete:28`、`completeTask:703`；类型枚举/进度/领奖 |
| `TaskFuncAdd.php` | 9 | 任务扩展/新增模板 |
| `AchivementFunc.php` | 3 | 成就触发点 |
| `ActFunc.php` | 25 | 活动总控（开服活动/节日） |
| `MarrySystemFunc.php` | 26 | 结婚系统（结缘/婚礼/加成） |
| `StatFunc.php` | 8 | 统计报表 |
| `UserFunc.php` | 25 | 玩家资料/设置/免战/元宝金两 |
| `GuideFunc.php`/`guide/` | 2 | 新手引导步骤（根文件近空，逻辑在包内） |

---

## 10. 战报 / 结算 / 定时

| 文件 | 函数数 | 核心逻辑 |
|---|---:|---|
| `cfg_js.php` | **40** | ★调度+配置混合：`:11-22` 定时任务清单（出征/战斗/将领历练/天灾人祸/科技/军队/城防/自动运输/市场交易/史诗任务/洛阳战）；`:29-80` `HandleTroop`（读 sys_troops→解析资源→addCityResources→报告文案）；含 `$GLOBALS` openfunc 白名单/sendCommand 文案 |
| `ReportCron.php` | 22 | `:20-274` `updateCityCalamity`（扫 mem_city_resource 处理粮/金/木/石/铁/人口溢出→报告→调民心）；`:347-464` 扫 mem_technic_upgrading/mem_city_draft/mem_city_reinforce 到期结算 |
| `ReportFunc.php` | 6 | 战报查询/分页 |
| `report/report0/report1.php`+`reporthead/reportroot.html` | - | Web 战报展示页 |
| `report_data/` | - | 报表数据 |
| `cron/{upbattle,upbuilding,upround,upTechnic,uptroop}` | - | 外部 cron 占位（实际结算走 cfg_js 惰性 tick） |

---

## 11. 公共库 / 运维 / 统计 / 残留

| 文件 | 函数数 | 要点 |
|---|---:|---|
| `utils.php` | **118** | ★公共函数总库：时间格式化（MakeTimeLeftString）、资源计算、随机、日志、getServerTime、SQL helper 等 |
| `UtilsExtend.php` | 51 | 扩展工具（业务 helper 补充） |
| `utils/{StringUtils,URLUtils}.php` | - | 字符串/URL 小工具 |
| `GetCCU/GetUserLogin/GetUserOnline(5)/GetUsersOnlineCount.php` | - | 在线/登录统计脚本 |
| `getBlockData.log`(41KB)/`getBlockData_sql.log`/`getCityInfo.log` | - | ★地图块/城池查询**运行日志**（非代码，记录 SQL 与耗时） |
| `updateEH.php`/`repair_xiuwei.php` | - | 修数据脚本（EH=装备/修为修复） |
| `sessions/waigua/997.php` | 4 | ★**定性：运维作弊/测试改数据脚本**——`mysql_connect(localhost,root,root)` 硬编码本地库，按 passport 直改 `sys_user state/nobility/money/honour` + `replace into sys_building` 批量造满级建筑；**无鉴权、SQL 注入、放在 sessions/ 属遗留后门，生产环境应删除**（重写版不复现） |
| `test.php` | 18 | ★**开发调试残留**：将领排行 HTML 输出 + 大量注释掉的 cfg_npc_hero 统计 SQL；非生产路径 |
| `autofortest.php` | - | 自动化测试脚本残留 |
| `lang.php`(225KB)/`lang_tw.php`(95KB) | - | 简体中文/繁体中文语言包（`$lang` 数组，键名按模块，覆盖全部 UI/提示文案） |
| `amfphp/DiscoveryService.php`（game 内副本） | 4 | 服务发现（与框架层重复） |

---

## 12. 安全与质量观察（原样记录，不修复）

| 类别 | 位置 | 现象 |
|---|---|---|
| SQL 注入面 | 全库 | 字符串拼接 SQL（`sql_query("...$var...")`）；仅靠 `BloodWarFilter.php` addslashes + `verifyQuery` 缓解 |
| 硬编码凭据 | `sessions/waigua/997.php:5` | `root/root` 明文连本地库 |
| 无鉴权改数据 | `997.php:19-30+` | 直接改 money/nobility/造满级建筑——**后门脚本** |
| 动态 include | `Login.php:28+` | `@include("./passport/$passtype.php")`，passtype 若未白名单可致文件包含 |
| 危险函数 | `Command.php:104` | `function_exists` 反射调用（已用 openfunc 白名单约束） |
| 调试残留 | `test.php`/`autofortest.php`/`*.log` | 生产目录混入测试与日志 |

> 以上为 legacy 既有事实，按「不修复原版 bug」原则**仅登记**；重写版 Go 后端以参数化查询 + 白名单路由 + 无后门脚本对应。

---

## 13. 全量覆盖核对

- 243 个 .php 文件：框架 62 + lib 13 + config 5 + game 163，本文 §1-§12 逐类登记完毕。
- 1778 个函数（112 文件）：Top 大户 BattleFunc(105)、GoodsFunc(133)、utils(118)、cfg_js(40)、HeroFunc(53)、HotelFunc(53)、HeroExpr(53)、TaskFunc(54)、UnionFunc(67)、EquipmentFunc(42)、GroundFunc(42)、PKFunc(43) 均已按功能分组覆盖。
- 可疑文件定性：`npc_armor_script_*`=NPC装备脚本(安全)、`waigua/997.php`=改数据后门、`test.php`/`autofortest.php`=调试残留、`*.log`=运行日志。
