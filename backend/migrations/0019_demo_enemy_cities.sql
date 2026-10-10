-- 0019_demo_enemy_cities.sql —— 合成演示数据：两座敌方城池
--
-- 目的：为世界地图的「目标操作按 flag 关系条件渲染」（R6）提供可正面验证的目标。
--   演示库原本只有 1 座己方普通城（cid=5 / wid=5），R6 新增的
--   出征(掠夺)、宣战、标记、治理 等条件分支无法被实际触发。
--
-- 依据后端 flag 判定（internal/world/service.go:391-449 cityFlag / npcOrPersonalFlag）：
--   * 座 A：NPC 城（uid=896 < npcUIDEnd=897）且 citytype=1 → flag=5
--           → canPlunder(flag∈{4,5,6,9}) ✔ / canMark(citytype>0) ✔ / canGovern(flag∈[2,8] && citytype∈[1,4]) ✔
--   * 座 B：玩家式城（uid=1001 ≥ 897）且 citytype=0、无联盟关系 → 走 npcOrPersonalFlag 其余分支（默认 flag=7）
--           → canWar(flag===7) ✔
-- 说明：**这两座城是合成演示数据**（非原版数据），仅用于本地验证 UI 分支；城池名/NPC 为示意。
-- 幂等：REPLACE INTO（与 0004/0014 的种子写法一致）。

-- NPC/敌方君主（users 结构见 0003_normalized.sql）
REPLACE INTO `users` (`id`, `passport`, `password_hash`, `nickname`, `state`, `money`, `honour`, `nobility`, `created_at`) VALUES
(896,  'npc_896',  '', '黄巾军', 0, 0, 0, '', 0),
(1001, 'npc_1001', '', '董卓',   0, 0, 0, '', 0);

-- 城池（type＝citytype：1=郡城 / 0=普通；is_special=0）
REPLACE INTO `cities` (`id`, `user_id`, `name`, `is_special`, `type`, `general_hero_id`, `chief_hero_id`, `counsellor_hero_id`) VALUES
(1010, 896,  '虎牢关', 0, 1, 0, 0, 0),
(1011, 1001, '长安',   0, 0, 0, 0, 0);

-- 城池资源（缺失会导致城池信息查询异常，故显式给一份）
REPLACE INTO `city_resources` (`city_id`, `wood`, `wood_max`, `rock`, `rock_max`, `iron`, `iron_max`, `food`, `food_max`, `gold`, `gold_max`, `people`, `people_max`, `morale`, `tax`, `complaint`, `vacation`) VALUES
(1010, 100000, 500000, 100000, 500000, 100000, 500000, 100000, 500000, 10000, 100000, 1000, 5000, 100, 0, 0, 0),
(1011, 100000, 500000, 100000, 500000, 100000, 500000, 100000, 500000, 10000, 100000, 1000, 5000, 100, 0, 0, 0);

-- 世界地格（type=0 城池；ownercid 指向上面的城池；wid 与演示城 cid=5 同在 block 0）
REPLACE INTO `mem_world` (`wid`, `type`, `ownercid`, `state`, `level`, `province`, `jun`) VALUES
(10, 0, 1010, 0, 5, 1, 1),
(11, 0, 1011, 0, 3, 1, 1);
