# 热血三国 SWF 解包 —— 脚本代码与美术资源关联整理

> 解包工具：JPEXS FFDec 26.3.0；导出目录：`tools/export/`（已被 `.gitignore` 排除）。
> 本文档基于对导出产物的静态分析 + 美术资源视觉识别整理，用于「原版逻辑/视觉 1:1 复刻」参考。
> **覆盖范围**：全项目 132 个 SWF/SWC 源文件（去重后）已全部解包，含 4 个主 SWF、`swf/` 15 子包、100 个验证码、2 个 SWC 组件库、8 个杂项动画、2 个 Flex 框架 RSL、1 个 Adobe 占位。

## 0. 总览

### 0.1 主业务 SWF（游戏核心）

| SWF 包 | 角色 | .as 脚本 | 位图 | 矢量 shape | sprite | 说明 |
|---|---|---:|---:|---:|---:|---|
| `BloodWar.swf` | 主客户端 | 2553 | 648 | 121 | 51 | 全部业务模块（50+ 包） |
| `common2.swf` | 公共组件库 | 383 | 325 | 74 | 22 | 面板/皮肤/通用控件 |
| `ChibiBattle.swf` | 小兵战斗动画 | 1244 | 83 | 25 | 16 | 回合制 Q 版战斗 |
| `BloodBattle.swf` | 战斗引擎 | 559 | 38 | 17 | 15 | 战斗结算逻辑 |
| `swf/*`（15 子包） | UI/皮肤资源 | 121 | 769 | — | — | 详见 §7 |
| `swf/pass/src/*`（100） | 登录验证码 | 300 | 100 | — | — | 详见 §8 |
| `bloodcommon.swc` | 共享类库 | 203 | 1 | — | — | ★含原版权威常量，见 §9 |
| `misc`（8 个动画） | 杂项动画 | 0 | 110 | 28(SVG) | — | 野怪/箭头/加载，见 §10 |
| `framework_3.x.swf`（2） | Flex 框架 RSL | 656/659 | 10 | — | — | Adobe 运行时库，无业务价值 |
| `playerProductInstall.swf` | Adobe 占位 | 1 | 3 | — | — | 无业务价值 |
| `button_up.swc` | Flex 组件库 | 33 | 7 | — | — | Form/Screen/Slide 组件 |

> 全量合计：约 **6100+ 个 .as**、**2200+ 张位图**。

导出目录内每个包结构一致：
```
<Package>/
├── scripts/        反编译的 ActionScript 3 源码（.as）
├── images/         内嵌位图（PNG/JPG），文件名 = <字符ID>_<符号类名>
├── shapes/         矢量图形（.shape）
├── sprites/        精灵（.sprite）
├── morphshapes/    补间形状
├── frames/         帧导出（动画逐帧 PNG）
├── movies/         子影片（.swf）
├── fonts/          字体
├── texts/          静态文本
├── binaryData/     二进制资源（.bin）
└── symbolClass/symbols.csv   ★ 字符ID → 符号类名 映射表（关联核心）
```

---

## 1. 脚本 ↔ 美术 的三条关联机制

原版采用 Flex(FXG) + AS3 编译产物，美术资源与代码通过以下三种方式绑定：

### 机制 A：`[Embed]` 内嵌资源（编译期绑定）
美术图片在编译时被打包进 SWF，反编译后表现为一个继承 `BitmapAsset` 的类，用 `[Embed]` 指向 `/_assets/` 内的位图。

**关联链**：`symbols.csv` 提供 `字符ID → 类名`，类名内嵌原始路径，`[Embed]` 指向 `images/<ID>_<类名>.png`。

脚本片段（[CityInnerPanel_cityhall.as](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/CityInnerPanel_cityhall.as)）：
```as3
package {
   import mx.core.BitmapAsset;
   [Embed(source="/_assets/324_CityInnerPanel_cityhall.png")]
   public class CityInnerPanel_cityhall extends BitmapAsset { }
}
```
对应 `symbols.csv`：`324;"CityInnerPanel_cityhall"` → 磁盘文件 `images/324_CityInnerPanel_cityhall.png`。

### 机制 B：具名链接类（Linkage，运行时 `new`/`getDefinitionByName`）
矢量 shape/sprite/movieclip 以「导出为 AS3 类」方式链接，代码中直接 `new ClassName()` 实例化。命名约定 `<父类>_$<部件>`（如按钮三态 `button_commander_up/over/down`）。

### 机制 C：运行时 URL 动态加载（美术资源在服务器，非内嵌）
大量业务图片（建筑详情图、兵种图、武将立绘、道具图标）不内嵌于 SWF，而是运行时按命名规则拼 URL 加载。

**关联链**：`Global.getImagePathFunc()` → `ExternalInterface.call("getImagePath")` 由宿主 HTML/JS 注入资源根路径，再拼接分类文件名。

[Global.as:515](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/Global.as)：
```as3
public static function getImagePathFunc() : String {
   var _loc1_:String = ExternalInterface.call("getImagePath");
   return _loc1_ == null ? "" : _loc1_;
}
```

---

## 2. 分类一：城内建筑美术（CityInnerPanel）

**关联方式**：机制 A（内嵌位图）。
**聚合类**：[CityInnerPanel.as](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/CityInnerPanel.as) 将 17 个建筑位图类注册为 Flex 绑定属性：

```as3
_1420907484cityhall    = CityInnerPanel_cityhall;
_333150870barrack      = CityInnerPanel_barrack;
_36682261institute     = CityInnerPanel_institute;
_1420460619citywall    = CityInnerPanel_citywall;
_751672484townwall     = CityInnerPanel_townwall;
// ... barn/blacksmith/forceyard/hotel/house/inn/market/recruithall/stable/temple/tower/warhouse
```

### 建筑位图清单与视觉识别

| 符号类名 | 文件 | 视觉识别描述 | 对应游戏建筑 |
|---|---|---|---|
| `CityInnerPanel_cityhall` | [324.png](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/324_CityInnerPanel_cityhall.png) | 等距 45° 视角，红墙黄琉璃瓦四合院式宫殿群，中轴主殿+两侧配殿，石阶基座，规模宏大 | 官府（城主府，bid 1） |
| `CityInnerPanel_citywall` | [624.png](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/624_CityInnerPanel_citywall.png) | 等距菱形青砖城墙环，四角与四面设敌台/城门洞，垛口清晰，色调土黄偏灰，体量大（145KB） | 城墙（内城，高级） |
| `CityInnerPanel_townwall` | [648.png](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/648_CityInnerPanel_townwall.png) | 等距菱形木栅栏寨墙，间隔设木质瞭望塔/哨棚，棕褐木色，规模较城墙小 | 寨墙（外城，低级） |
| `CityInnerPanel_barrack` | 479.png | 军营建筑（待识别） | 兵营（bid 8） |
| `CityInnerPanel_institute` | 798.png | 书院建筑 | 书院（bid 7） |
| `CityInnerPanel_forceyard` | 415.png | 校场/演武场 | 校场（bid 10） |
| `CityInnerPanel_warhouse` | 477.png | 军械/仓库 | 仓库（bid 9） |
| `CityInnerPanel_barn` | 668.png | 粮仓 | 农田/粮仓 |
| `CityInnerPanel_market` | 400.png | 市集 | 市场 |
| `CityInnerPanel_blacksmith` | 870.png | 铁匠铺 | 铁匠铺 |
| `CityInnerPanel_stable` | 344.png | 马厩 | 马厩 |
| `CityInnerPanel_recruithall` | 363.png | 招兵处 | 征兵处 |
| `CityInnerPanel_temple` | 703.png | 庙宇 | 庙宇 |
| `CityInnerPanel_tower` | 751.png | 塔楼 | 瞭望塔 |
| `CityInnerPanel_hotel` | 768.png | 客栈/旅馆 | 旅馆 |
| `CityInnerPanel_inn` | 610.png | 酒馆 | 酒馆 |
| `CityInnerPanel_house` | 829.png | 民居 | 民居（bid 6） |

> **复刻要点**：建筑格子图另有一套运行时 URL 图（机制 C），命名 `images/building_intro_<bid>.png`（建造/升级弹窗详情图）、`images/building_cityhall.png` / `building_citywall.png` / `building_townwall.png` / `building_outercity.png` / `building_outertown.png`（城内格子 `BuildingGrid` 位图）。见 [BuildingGrid.as:150](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/Building/BuildingGrid.as)。

---

## 3. 分类二：登录与主视觉

**关联方式**：机制 A（内嵌 JPG）。

| 文件 | 视觉识别 | 用途 |
|---|---|---|
| [847_login.jpg](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/847_Login.LoginDialog__embed_mxml_images_board_login_jpg_1633673007.jpg) | 冷青色调三国战场：右侧持方天画戟、插翎羽的黑甲武将（吕布形象）为主视觉，左侧白马武将（赵云）与战旗，左上「熱血三國®」红金 Logo，含「账号/密码/记住账号」表单位与底部健康游戏忠告 | 登录界面背景 |
| 618_login2.jpg | 登录背景第二变体（153KB） | 登录备选图 |
| 495_sequence.jpg | 过场/序列图 | 加载页 |
| 645_battle_header.jpg | 战斗面板头图 | 战斗弹窗标题 |

符号类名：`Login.LoginDialog__embed_mxml_images_board_login_jpg_1633673007`（`embed_mxml` 前缀表明来自 Flex MXML 声明式内嵌）。

---

## 4. 分类三：UI 通用美术（css 内嵌）

**关联方式**：机制 A，命名 `BloodWar__embed_css_images_<分类>_<名>_png_<hash>`。
`embed_css` 表示来自 Flex CSS 主题声明。共 556 个，按目录分类：

| 分类目录 | 数量 | 内容 |
|---|---:|---|
| `images/button` | 69 | 通用按钮（green/red/gold/null 等态） |
| `images/topbutton` | 54 | 顶栏导航按钮（mycity/army/hero/union/mission/map，含 up/over/down/on 三态） |
| `images/function` | 42 | 底栏功能图标（help/charge/friend/report/achievement/system/mail/stat，含 on/down 态） |
| `images/map` | 30 | 世界/小地图 |
| `images/influence` | 29 | 势力/国家面板 |
| `images/lottery` | 28 | 抽奖 |
| `images/battle` | 24 | 战斗槽位/箭头 |
| `images/channel` | 20 | 聊天频道 |
| `images/luoyangBattle` | 20 | 洛阳战 |
| `images/chibi` | 15 | Q 版战斗控件 |
| `images/mycity` | 12 | 我的城（building 三态按钮） |

代表资源视觉识别：
- [355_lottery_turntable.png](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/355_BloodWar__embed_css_images_lottery_turntable_png_1591883600.png)：抽奖转盘底图，青蓝渐变圆形转盘+回纹环，外圈 8 个黑色奖品槽位，四角金色卷草纹装饰。
- [483_hero_frame.png](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/images/483_BloodWar__embed_css_images_hero_frame_png_43682454.png)：武将头像框，深墨绿近黑底+银灰描边，底部浅灰信息条，竖版 3:4。

---

## 5. 分类四：兵种 / 武将 / 道具美术（运行时 URL，机制 C）

这些资源**不内嵌于 SWF**，由脚本按命名规则动态拼接加载，是复刻时最容易遗漏的部分。

### 5.1 兵种图（Army）
[ArmyDraftItem.as:349](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/Army/ArmyDraftItem.as)：
```as3
return Global.getImagePathFunc() + "images/army_" + mDraftState.sid + ".png";
```
| 命名规则 | 引用脚本 | 说明 |
|---|---|---|
| `images/army_<sid>.png` | ArmyDraftItem / BattleArmyItem | 兵种小图标（sid=兵种ID，对应 cfg_soldiers.sid） |
| `images/army_<sid>_big.png` | ArmyDraftDialog / ArmySoldierItem | 兵种大图 |
| `images/battlectrl_<action>.png` | BattleArmyItem | 战场动作精灵（行军/攻击/防御等） |
| `images/defence_<did>.png` | BattleDefenceItem | 防御工事图 |

### 5.2 武将立绘（Hero）
[HeroDetailDialog.as:2209](file:///c:/Users/wst_k/Desktop/workspace/rxsg/tools/export/BloodWar/scripts/Hero/HeroDetailDialog.as)：
```as3
mHeroImageName = Global.getImagePathFunc() + "images/hero/hero_" + (mHero.sex == 0 ? "girl_" : "boy_") + mHero.face.toString() + ".jpg";
```
| 命名规则 | 说明 |
|---|---|
| `images/hero/hero_boy_<face>.jpg` | 男性武将立绘（face=头像编号，对应 heroes.face） |
| `images/hero/hero_girl_<face>.jpg` | 女性武将立绘 |
| `images/herox/hero_<sexname>_<face>.jpg` | 变体目录（GroundHeroItem/InfoHeroItem） |
| `images/player/player_<sex>_<face+1>.jpg` | 玩家角色头像 |
| `images/hero_card_{purple,red,yellow,blue}.png` | 武将品质卡框（Global.HERO_CARD_IMAGES） |

### 5.3 道具 / 技能 / 成就
| 命名规则 | 引用脚本 | 说明 |
|---|---|---|
| `images/item_<gid>.png` | BlackSmith/GoodsItem, SmallItem | 道具图标（gid=货物ID） |
| `images/item_equip_default.png` 等 | SmallItem | 装备占位图 |
| `images/heroskill/skill_<a>_<b>.png` | Armor/ChangeArmorItem | 技能图标 |
| `images/heroskill/equip_smallitem.png` | SkillItem | 装备小件 |
| `images/achievement/<id\|image>.png` | AchieveGuide | 成就图标 |
| `images/achievement/titlebk.png` / `prgsImg.png` | AchieveGuide/AchieveProgress | 成就标题底/进度图 |

---

## 6. 分类五：战斗动画美术（ChibiBattle / battleRound / trooper）

**关联方式**：机制 B（具名链接类）+ 逐帧导出。

- `ChibiBattle.swf`：Q 版战斗控件内嵌图（`ChibiBattle__embed_css____images_chibi_*`），含 `citycombobox`/`heroborder`/`transport_normal` 等；另有 `MenuCheck/MenuBranch/MenuRadio` 等菜单九宫符号。
- `BloodBattle.swf`：战斗结算引擎脚本（559 个 .as），美术占比小。
- `common2` 的 `images/chibi_*`：`quitBattle_{Normal,over,Click}`、`treasure_{normal,choose,click}`、`battle_treasure_click`、`battle_task_choose` —— 小兵战斗的退出/宝物/任务选择按钮三态。
- 外部动画包（`battleRound` / `heroBattleEffect` / `trooper`）由运行时加载，清单见 §7.2。

---

## 7. 分类六：`swf/` 子包资源清单（15 个）

每个子包导出至 `tools/export/swf/<name>.swf/`，按内容分四类：

### 7.1 Flex 皮肤/组件包（机制 B：具名链接类）

| 子包 | sprite | 关键符号 | 内容 |
|---|---:|---|---|
| `UIParts.swf`（110KB） | 47 | `button_red_up/base/over/down`、`button_mycity_up`、`button_myinfo_up`… | UI 按钮四态精灵集合 |
| `skin.swf`（19KB） | 177 | `Panel_controlBarBackgroundSkin`、`ColorPicker_*Skin`、`LinkButton_downSkin`… | Flex 控件皮肤库（mx 样式类） |
| `bloodwarskin.swf` | 35 | `button_base/down/up` + `Form/Slide/Screen` | 游戏按钮皮肤 |
| `bloodwarskin2.swf` | 2 | `button_red_down/normal` | 红色按钮补充 |
| `bloodwar.swf` | 31 | `Form/Slide/Screen/幻灯片1/演示文稿` | 旧版 Flex 2 壳（演示文稿式布局组件） |
| `button_up.swf` | — | 同 `button_up.swc` 内嵌层 | 按钮组件 |
| `scene.swf` | 1 | `empty_shadow` | 空场景占位（被 `[Embed] "/_assets/assets.swf"` 引用） |
| `unActiveHero.swf` | 1 | — | 未激活武将灰显遮罩 |

### 7.2 战斗动画包

| 子包 | 内容 |
|---|---|
| `battleRound.swf`（32KB） | 回合数动画 `round_1`…`round_6`（6 帧，282 文件，含逐帧 PNG） |
| `heroBattleEffect.swf`（25KB） | 武将战斗特效 4 类：`Baoji`(暴击)、`Gedang`(格挡)、`Poji`(破击)、`Shanbi`(闪避)，各含 sprite+位图 |
| `trooper.swf` / `trooperReverse.swf`（9KB×2） | 兵种行军动画精灵（正向/反向各 6 图） |
| `fight.swf`（1KB） | 战斗占位小图 |

### 7.3 杂项
- `announce_entry.swf`（14KB）：公告入口图标（2 图）
- `empty.swf`（0KB）：空文件占位

---

## 8. 分类七：登录验证码（swf/pass/src，100 个）

每个 `N.swf`（N=1..100）导出为 `tools/export/pass/N.swf/`，含：
- `scripts/swfpass.as`：验证码逻辑类，暴露 `passValue` 与 `isValid()`。
- `frames/`：验证码字符渲染帧 PNG。

关联方式：机制 A/B，属独立小型 SWF，登录时随机加载其一。

---

## 9. ★ bloodcommon.swc —— 原版权威常量（重大发现）

**位置**：根目录 `bloodcommon.swc`（392KB）。ZIP 层含 `catalog.xml`（125KB 组件目录）、`library.swf`（383KB）、`locale/en_US/*.properties`；内嵌 `library.swf` 解包至 `tools/export/bloodcommon.swc/library_swf/`（203 个 .as）。

**业务脚本**（非 mx 框架部分）：
```
scripts/
├── City.as          城池共享类
├── Define.as        ★ 全局常量定义（权威）
├── Global.as        全局状态
├── User.as          玩家状态
├── Utils.as         工具
├── Building/BuildingState.as   建筑状态共享类
└── mx/**            Flex 框架类（与 framework RSL 重复）
```

### 9.1 `Define.as` 原版权威常量摘录

```as3
public static const GAME_SPEED_RATE:int = 1;
public static const GRID_DISTANCE:int = 60000;
public static const BUILDING_MAX_LEVEL:int = 10;
public static const TECHNIC_MAX_LEVEL:int = 10;
public static const NPC_UID_END:int = 897;
public static const HERO_FEE_RATE:int = 20;
// …
```

**原版 20 建筑 ID 枚举**（与 legacy `server/game/common.php` 完全一致）：

| bid | 常量 | 建筑 | bid | 常量 | 建筑 |
|---:|---|---|---:|---|---|
| 1 | `ID_BUILDING_FARMLAND` | 农田 | 11 | `ID_BUILDING_OFFICE` | 官署 |
| 2 | `ID_BUILDING_WOOD` | 伐木场 | 12 | `ID_BUILDING_HONGLU` | 鸿胪寺 |
| 3 | `ID_BUILDING_ROCK` | 采石场 | 13 | `ID_BUILDING_MARKET` | 市场 |
| 4 | `ID_BUILDING_IRON` | 铁矿 | 14 | `ID_BUILDING_BLACKSMITH` | 铁匠铺 |
| 5 | `ID_BUILDING_HOUSE` | 民房 | 15 | `ID_BUILDING_WORKSHOP` | 工坊 |
| 6 | `ID_BUILDING_GOVERNMENT` | 官府 | 16 | `ID_BUILDING_BARN` | 粮仓 |
| 7 | `ID_BUILDING_COLLEGE` | 书院 | 17 | `ID_BUILDING_STORE` | 仓库 |
| 8 | `ID_BUILDING_GROUND` | 校场 | 18 | `ID_BUILDING_DAK` | 驿站 |
| 9 | `ID_BUILDING_ARMY` | 军营 | 19 | `ID_BUILDING_BALEFIRE` | 烽火台 |
| 10 | `ID_BUILDING_HOTEL` | 客栈 | 20 | `ID_BUILDING_WALL` | 城墙 |

> **复刻警示**：原版客户端/服务端均使用上表 **20 建筑 bid**；重写版后端硬编码为 10 建筑映射（1官府/2农田/…/8兵营/9仓库/10校场，见 `collegeBID=7`/`barracksBID=8`）。两套映射**不同源**，数据库种子按重写映射播种、成本曲线经 bid 重映射移植（详见 §11 与 `backend/migrations/0004_seed_normalized.sql` 头注）。若后续要求 1:1 对齐原版，应整体切换为本表 20 bid 并重导数据。

---

## 10. 杂项动画资源（misc，8 个）

导出至 `tools/export/misc/<name>/`，均为 Flash 逐帧动画（无 .as 脚本，机制 B 纯图形）：

| 文件 | 帧/精灵 | 视觉识别 | 用途 |
|---|---|---|---|
| `monster_level_1.swf` | 2 sprite×21 帧 | 蓝甲持长枪士兵立绘（等距小像素风） | 1 级野怪战斗单位 |
| `monster_level_2.swf` | 同上 | 同模板换色 | 2 级野怪 |
| `monster_level_3.swf` | 同上 | 与 1 级视觉相同（原版美术复用） | 3 级野怪 |
| `arrow_1~4.swf` | 4 帧/个 | 暗红底米黄描边箭头：1=上、2=右、3=下、4=左 | 世界地图行军方向指示 |
| `loading_runing.swf` | 6 帧 | 黑底红马绿袍持大刀骑兵奔跑（关羽形象） | 加载/行军中动画 |

> 注：`frontend/public/images/` 下同名 10 个 SWF 与 `images/` 逐字节一致（哈希校验），为前端静态副本，未重复解包。

---

## 11. 复刻映射建议（美术资源 → 重写版）

| 原版资源 | 重写版落点 | 备注 |
|---|---|---|
| `CityInnerPanel_*`（17 建筑内嵌图） | `frontend/src/assets/city/` | 城内格子渲染，按 bid 映射（官府1/农田2/…/校场10） |
| `building_intro_<bid>.png` | 建造/升级弹窗 | 机制 C，需从服务器原图获取 |
| `army_<sid>.png` / `_big` | 兵营面板 | sid 对齐 `cfg_soldiers.sid` |
| `hero_boy/girl_<face>.jpg` | 武将详情 | face 对齐 `heroes.face` |
| `embed_css_images_button/topbutton/function` | 通用 UI 皮肤 | 可直接复用 PNG |
| `login.jpg` / Logo | 登录页 | 含「熱血三國」商标，注意版权 |
| `UIParts/skin/bloodwarskin` 按钮四态精灵 | 前端按钮皮肤 | 逐 sprite 导出 PNG 复用 |
| `battleRound`（round_1~6）/ `heroBattleEffect`（暴击/格挡/破击/闪避） | 战斗面板动画 | 帧序列 → CSS 动画或 `<img>` 序列 |
| `monster_level_1~3`（21 帧/sprite） | 野怪战斗单位 | 帧序列导出在 `misc/*/sprites/` |
| `arrow_1~4`（4 帧）/ `loading_runing`（6 帧） | 行军指示/加载动画 | 同上 |
| `Define.as` 常量 | 后端 `internal/game` | 倍速/上限/费率权威值，校对重写版常量 |

> **关键提醒**：机制 C 的运行时图片（建筑详情/兵种/武将/道具）在仓库 `server/images/` 中**并不存在**（该目录仅 20 个官网静态图）。原版这些美术资源由已下线的游戏服务器提供，复刻时需另行获取原始图包，或以内嵌的 `CityInnerPanel_*` 作为城内视觉的替代来源。

---

## 附：符号表查询

`symbols.csv` 格式：`<字符ID>;"<符号类名>"`。用类名反查位图文件：
`images/<ID>_<类名>.png`（ID 即第一列）。示例：
```
324;"CityInnerPanel_cityhall"  →  images/324_CityInnerPanel_cityhall.png
```
