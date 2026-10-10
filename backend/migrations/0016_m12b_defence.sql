-- 0016_m12b_defence.sql
-- R11-2 项③：城防器械信息（1:1 复刻 legacy server/game/DefenceFunc.php:40 doGetDefenceInfo）。
--
-- 现状（0011 已建）：city_defences（← sys_city_defence）、cfg_defence（← cfg_defence，含
--   did/name/hp/ap/dp/g_range，**合成值**）。doGetDefenceInfo 还读取 cfg_defence 的
--   description/speed/carry/*_need/area_need/time_need 与 cfg_defence_condition → 本迁移补齐。
--
-- ⚠ 数据来源说明：cfg_defence 原始 dump 已丢失（0011 头注已声明 hp/ap/dp/range 为合成值）。
--   本迁移仅补齐【列结构】以对齐 legacy 查询口径，新增列一律保持默认 0/空串，不填任何数值
--   （原配置数值无来源 → 不造假）。故运行时 reinforce_time = max(1, 0 × 速度) = 1，
--   can_reinforce 仅由配置表条件决定（当前无行 → 恒 true）。
--   cfg_defence_conditions 同理建【空表】：无行 = 无前置条件（对齐现库未配置 cfg_building_conditions）。
--
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE（兼容 multiStatements，
--       与 0005/0011/0015 同写法，不使用存储过程/DELIMITER）。

SET NAMES utf8mb4;

-- ── cfg_defence 补列（← legacy cfg_defence 同名列）────────────────────────
-- description 器械说明；speed/carry 行军速度/负重；*_need 建造资源；area_need 占地；time_need 单件建造时间。
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'description') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `description` varchar(255) NULL', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'speed') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `speed` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'carry') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `carry` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'wood_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `wood_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'rock_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `rock_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'iron_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `iron_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'food_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `food_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'gold_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `gold_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'area_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `area_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_defence' AND COLUMN_NAME = 'time_need') = 0,
  'ALTER TABLE `cfg_defence` ADD COLUMN `time_need` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cfg_defence_conditions（← cfg_defence_condition）──────────────────────
-- pre_type：0 建筑 / 1 科技；pre_id/pre_level 为前置 id 与所需等级。原版 dump 丢失 → 建空表
-- （无行 = 无前置条件，与现库未配置一致；对齐 0015 的 cfg_building_conditions 处理）。
CREATE TABLE IF NOT EXISTS `cfg_defence_conditions` (
  `did`       int NOT NULL,
  `pre_type`  int NOT NULL DEFAULT 0,
  `pre_id`    int NOT NULL DEFAULT 0,
  `pre_level` int NOT NULL DEFAULT 0,
  KEY `idx_did` (`did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
