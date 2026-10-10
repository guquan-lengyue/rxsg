-- 0020_m13c_barn.sql
-- R2：马厩（bid=16）建筑配置 + 演示城建筑（幂等）。
--
-- 背景：legacy common.php 与 AS Define.as:64 均定义 ID_BUILDING_BARN=16（马厩/坐骑，
--   对应 BarnDialog），但重写版 cfg_buildings 只到 14（15/16 空缺）→ 马厩无法从建筑点击进入。
--   本迁移沿用 legacy bid=16 补齐 cfg_buildings/cfg_building_levels，并给演示城 cid=5 补一座。
--
-- inner=1 依据：马厩在城内——legacy ID_BUILDING_BARN=16 由 CityInnerPanel 内城渲染并经
--   DialogManager.getBuildingDialog 选 BarnDialog（docs/SWF前端逻辑还原/02-城内与建筑.md §1.3/§3.6）。
--
-- cfg_building_levels 为【合成数值，非原版精确还原】：本地无 cfg_building_level 的权威 dump，
--   沿用 0010/0018 先例合成 10 级成本曲线，仅用于打通建造/升级/拆除流程。
--
-- 幂等：REPLACE INTO / DELETE+INSERT；演示建筑用 INSERT ... WHERE NOT EXISTS（对齐 0010）。

SET NAMES utf8mb4;

-- ── cfg_buildings：马厩（沿用 legacy bid=16；table_name='barn'，inner=1 城内）──
REPLACE INTO `cfg_buildings` (`bid`, `name`, `table_name`, `inner`) VALUES
(16, '马厩', 'barn', 1);

-- ── cfg_building_levels：马厩(16) 合成 10 级曲线（A 合成值，非原版）──
DELETE FROM `cfg_building_levels` WHERE `bid` = 16;
INSERT INTO `cfg_building_levels` (`bid`, `level`, `upgrade_wood`, `upgrade_rock`, `upgrade_iron`, `upgrade_food`, `upgrade_gold`, `upgrade_people`, `upgrade_time`, `using_people`) VALUES
(16,1,600,500,400,300,0,0,400,5),(16,2,1200,1000,800,600,0,50,800,10),(16,3,2400,2000,1600,1200,0,100,1600,15),(16,4,4800,4000,3200,2400,0,200,3200,20),(16,5,9600,8000,6400,4800,0,400,6400,25),
(16,6,19200,16000,12800,9600,0,800,12800,30),(16,7,38400,32000,25600,19200,0,1600,25600,35),(16,8,76800,64000,51200,38400,0,3200,51200,40),(16,9,153600,128000,102400,76800,0,6400,102400,45),(16,10,307200,256000,204800,153600,0,12800,204800,50);

-- ── 演示城 cid=5 补一座马厩（坐标 c4 = x2,y3）──
-- cid=5 现有建筑：a1..a4 / b1..b4 / c1..c3 / d1..d3（0004 + 0010 共 14 座）；c4 为空闲城内格。
INSERT INTO `buildings` (`city_id`, `building_id`, `xy`, `level`, `state`, `state_start_at`, `state_end_at`)
SELECT 5, 16, 'c4', 1, 0, 0, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM buildings WHERE city_id = 5 AND building_id = 16);
