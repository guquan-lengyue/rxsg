-- 0015_m12_building.sql
-- R11-1 建造新建筑 / 拆除 / 彻底拆除 / 取消拆除 / 资源地转换
-- （1:1 复刻 legacy server/game/BuildingFunc.php；口径来源见各表注释）。
--
-- 表/列来源（legacy 文件:行号）：
--   building_upgrading       ← mem_building_upgrading（BuildingFunc.php:561/605/782；GoodsFunc.php:1329/1396）
--   building_destroying      ← mem_building_destroying（BuildingFunc.php:721/743；GoodsFunc.php:3210）
--   cfg_building_conditions  ← cfg_building_condition（BuildingFunc.php:178/497 前置条件查询）
--     原版 dump 丢失，建空表以保证查询可用（无行 = 无前置条件，与现库未配置一致）。
--   city_resources.changing  ← mem_city_resource.changing（startChangeBuilding:785 置 1，产出重算标记）
--
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE（兼容 multiStatements，
--       与 0005/0014 同写法，不使用存储过程/DELIMITER）。

SET NAMES utf8mb4;

-- ── building_upgrading（← mem_building_upgrading：id = buildings.id，同一建筑唯一）──
CREATE TABLE IF NOT EXISTS `building_upgrading` (
  `id`            int NOT NULL,
  `cid`           int NOT NULL,
  `xy`            varchar(8) NOT NULL DEFAULT '',
  `bid`           int NOT NULL DEFAULT 0,
  `level`         int NOT NULL DEFAULT 0,
  `state_endtime` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_cid` (`cid`),
  KEY `idx_endtime` (`state_endtime`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── building_destroying（← mem_building_destroying）────────────────────────────
CREATE TABLE IF NOT EXISTS `building_destroying` (
  `id`            int NOT NULL,
  `cid`           int NOT NULL,
  `xy`            varchar(8) NOT NULL DEFAULT '',
  `bid`           int NOT NULL DEFAULT 0,
  `level`         int NOT NULL DEFAULT 0,
  `state_endtime` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_cid` (`cid`),
  KEY `idx_endtime` (`state_endtime`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_building_conditions（← cfg_building_condition）─────────────────────────
-- pre_type：0 建筑 / 1 科技 / 2 物品；pre_id/pre_level 为前置 id 与所需等级/数量。
CREATE TABLE IF NOT EXISTS `cfg_building_conditions` (
  `bid`       int NOT NULL,
  `levelid`   int NOT NULL,
  `pre_type`  int NOT NULL DEFAULT 0,
  `pre_id`    int NOT NULL DEFAULT 0,
  `pre_level` int NOT NULL DEFAULT 0,
  KEY `idx_bid_level` (`bid`, `levelid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_resources.changing（← mem_city_resource.changing）─────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'changing') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `changing` tinyint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;
