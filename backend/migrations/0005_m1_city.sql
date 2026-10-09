-- 0005_m1_city.sql
-- M1 城市内政（CityFunc.php changeTax/levyResource/pacifyPeople/getCityProduct）所需表/列。
-- 幂等：information_schema 判断 + PREPARE 动态 ALTER（兼容 go-sql-driver multiStatements，不使用存储过程/DELIMITER）。

SET NAMES utf8mb4;

-- ── city_resources 补列（对应 legacy mem_city_resource）──────────────────
-- morale_stable：稳定民心，changeTax/pacify 时按 GREATEST(0,LEAST(100-tax-complaint,100)) 重算
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'morale_stable') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `morale_stable` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- people_stable：稳定人口，levy/pacify 后按 people_max*morale*0.01 重算（legacy 为浮点）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'people_stable') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `people_stable` double NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- food_army_use：当兵吃粮的军粮消耗（getCityProduct 读取，由 M7 战斗/征兵维护）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'food_army_use') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `food_army_use` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── heroes 补列（getCityProduct 城守加成公式使用）────────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'affairs_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `affairs_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'command_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `command_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_res_add（对应 legacy sys_city_res_add：产出比例/技能/道具加成）──
CREATE TABLE IF NOT EXISTS `city_res_add` (
  `city_id`          int NOT NULL,
  `food_rate`        int NOT NULL DEFAULT 80,
  `wood_rate`        int NOT NULL DEFAULT 80,
  `rock_rate`        int NOT NULL DEFAULT 80,
  `iron_rate`        int NOT NULL DEFAULT 80,
  `chief_add`        double NOT NULL DEFAULT 0,
  `resource_changing` tinyint NOT NULL DEFAULT 0,
  `skill_gold_add`   int NOT NULL DEFAULT 0,
  `skill_food_add`   int NOT NULL DEFAULT 0,
  `skill_wood_add`   int NOT NULL DEFAULT 0,
  `skill_rock_add`   int NOT NULL DEFAULT 0,
  `skill_iron_add`   int NOT NULL DEFAULT 0,
  `goods_food_add`   int NOT NULL DEFAULT 0,
  `goods_wood_add`   int NOT NULL DEFAULT 0,
  `goods_rock_add`   int NOT NULL DEFAULT 0,
  `goods_iron_add`   int NOT NULL DEFAULT 0,
  `field_food_add`   int NOT NULL DEFAULT 0,
  `field_wood_add`   int NOT NULL DEFAULT 0,
  `field_rock_add`   int NOT NULL DEFAULT 0,
  `field_iron_add`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_schedule（对应 legacy mem_city_schedule：冷却与天灾/天赐排程）──
CREATE TABLE IF NOT EXISTS `city_schedule` (
  `city_id`             int NOT NULL,
  `last_levy_resource`  bigint NOT NULL DEFAULT 0,
  `last_pacify_people`  bigint NOT NULL DEFAULT 0,
  `next_bad_event`      bigint NOT NULL DEFAULT 0,
  `next_good_event`     bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
