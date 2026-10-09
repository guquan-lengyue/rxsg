-- 0007_m3_hero.sql
-- M3 武将（HeroFunc.php upgradeHero/addHeroPoint/clearHeroPoint/beginExprHero/cancelHeroExpr/
--   fasterHeroExpr + HeroExpr.php 历练结算 + OfficeFunc.php 任命三职）所需表/列。
-- 幂等：information_schema 判断 + PREPARE 动态 ALTER；建表 CREATE TABLE IF NOT EXISTS；种子 REPLACE INTO。

SET NAMES utf8mb4;

-- ── users 补列（← sys_user：俸禄/官职消耗走 addMoney）────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'money') = 0,
  'ALTER TABLE `users` ADD COLUMN `money` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── heroes 补列（← sys_city_hero）────────────────────────────────────────
-- npc_id：名将卡 NPC id（getMaxHeroLevel 120/125 上限判定；新库无 cfg_npc_hero，恒按普通将 120 上限）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'npc_id') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `npc_id` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- command_add_on 已在 0005 添加；此处补 bravery/wisdom 的 add_on
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'bravery_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `bravery_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'wisdom_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `wisdom_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── hero_exprs 补列（← sys_hero_expr：carrymoney 随带元宝、accTimes 加速次数）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'hero_exprs' AND COLUMN_NAME = 'carrymoney') = 0,
  'ALTER TABLE `hero_exprs` ADD COLUMN `carrymoney` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'hero_exprs' AND COLUMN_NAME = 'acc_times') = 0,
  'ALTER TABLE `hero_exprs` ADD COLUMN `acc_times` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_resources 补列（← mem_city_resource：城守忠诚/俸禄）──────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'chief_loyalty') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `chief_loyalty` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'hero_fee') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `hero_fee` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── log_money（← log_money：addMoney 流水）───────────────────────────────
CREATE TABLE IF NOT EXISTS `log_money` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── hero_base_add（← sys_city_hero_base_add：升级/突破/加点的基础属性流水）──
CREATE TABLE IF NOT EXISTS `hero_base_add` (
  `id`                   int NOT NULL AUTO_INCREMENT,
  `user_id`              int NOT NULL,
  `hero_id`              int NOT NULL,
  `bravery_base_add_on`  int NOT NULL DEFAULT 0,
  `wisdom_base_add_on`   int NOT NULL DEFAULT 0,
  `affairs_base_add_on`  int NOT NULL DEFAULT 0,
  `command_base_add_on`  int NOT NULL DEFAULT 0,
  `type`                 int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_hero` (`hero_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_hero_expr_types（← 同名表；原 dump 丢失，数值为合成，已在对照表标注）──
-- hour_expr 仅 cancelHeroExpr 的 exp_add 预览查询使用（该值不落库、结算路径不读取）。
CREATE TABLE IF NOT EXISTS `cfg_hero_expr_types` (
  `type`       int NOT NULL,
  `name`       varchar(64) NOT NULL DEFAULT '',
  `min_hour`   int NOT NULL DEFAULT 0,
  `max_hour`   int NOT NULL DEFAULT 0,
  `hour_money` bigint NOT NULL DEFAULT 0,
  `hour_gold`  bigint NOT NULL DEFAULT 0,
  `hour_expr`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_hero_expr_types` (`type`, `name`, `min_hour`, `max_hour`, `hour_money`, `hour_gold`, `hour_expr`) VALUES
(1, '修身养性', 1, 24, 100, 1, 0),
(2, '闯荡江湖', 1, 24, 200, 2, 0);
