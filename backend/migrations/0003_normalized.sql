-- 0003_normalized.sql
-- 规范化 schema（与 backend/internal 当前 SQL 查询对齐：users/cities/cfg_buildings…）。
-- 列名从后端 SELECT/INSERT/UPDATE 反推；全部 InnoDB + utf8mb4；时间一律 Unix 秒 BIGINT。
-- 所有语句幂等（CREATE TABLE IF NOT EXISTS），可重复执行。
-- 说明：0002_minimal.sql 是旧版表名（sys_user/cfg_building），与当前后端不匹配，仅作历史保留。

SET NAMES utf8mb4;

-- ── 账号 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `users` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `passport`      varchar(64) NOT NULL,
  `password_hash` varchar(60) NOT NULL DEFAULT '',
  `nickname`      varchar(64) NOT NULL DEFAULT '',
  `state`         int NOT NULL DEFAULT 0,
  `money`         bigint NOT NULL DEFAULT 0,
  `honour`        int NOT NULL DEFAULT 0,
  `nobility`      varchar(32) NOT NULL DEFAULT '',
  `created_at`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_passport` (`passport`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 城池 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cities` (
  `id`                  int NOT NULL AUTO_INCREMENT,
  `user_id`             int NOT NULL DEFAULT 0,
  `name`                varchar(64) NOT NULL DEFAULT '',
  `is_special`          tinyint NOT NULL DEFAULT 0,
  `type`                int NOT NULL DEFAULT 0,
  `general_hero_id`     int NOT NULL DEFAULT 0,
  `chief_hero_id`       int NOT NULL DEFAULT 0,
  `counsellor_hero_id`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `city_resources` (
  `city_id`    int NOT NULL,
  `wood`       bigint NOT NULL DEFAULT 0,
  `wood_max`   bigint NOT NULL DEFAULT 0,
  `rock`       bigint NOT NULL DEFAULT 0,
  `rock_max`   bigint NOT NULL DEFAULT 0,
  `iron`       bigint NOT NULL DEFAULT 0,
  `iron_max`   bigint NOT NULL DEFAULT 0,
  `food`       bigint NOT NULL DEFAULT 0,
  `food_max`   bigint NOT NULL DEFAULT 0,
  `gold`       bigint NOT NULL DEFAULT 0,
  `gold_max`   bigint NOT NULL DEFAULT 0,
  `people`     bigint NOT NULL DEFAULT 0,
  `people_max` bigint NOT NULL DEFAULT 0,
  `morale`     int NOT NULL DEFAULT 0,
  `tax`        int NOT NULL DEFAULT 0,
  `complaint`  int NOT NULL DEFAULT 0,
  `vacation`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `buildings` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `city_id`       int NOT NULL,
  `building_id`   int NOT NULL,
  `xy`            varchar(8) NOT NULL,
  `level`         int NOT NULL DEFAULT 0,
  `state`         int NOT NULL DEFAULT 0,
  `state_start_at` bigint NOT NULL DEFAULT 0,
  `state_end_at`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_city_xy` (`city_id`, `xy`),
  KEY `idx_city` (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 建筑配置 ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_buildings` (
  `bid`        int NOT NULL,
  `name`       varchar(64) NOT NULL DEFAULT '',
  `table_name` varchar(64) NOT NULL DEFAULT '',
  `inner`      tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (`bid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_building_levels` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `bid`           int NOT NULL,
  `level`         int NOT NULL DEFAULT 0,
  `upgrade_wood`  bigint NOT NULL DEFAULT 0,
  `upgrade_rock`  bigint NOT NULL DEFAULT 0,
  `upgrade_iron`  bigint NOT NULL DEFAULT 0,
  `upgrade_food`  bigint NOT NULL DEFAULT 0,
  `upgrade_gold`  bigint NOT NULL DEFAULT 0,
  `upgrade_people` bigint NOT NULL DEFAULT 0,
  `upgrade_time`  bigint NOT NULL DEFAULT 0,
  `using_people`  int NOT NULL DEFAULT 0,
  `description`   varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_bid_level` (`bid`, `level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 科技 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `technics` (
  `id`             int NOT NULL AUTO_INCREMENT,
  `user_id`        int NOT NULL,
  `city_id`        int NOT NULL DEFAULT 0,
  `technic_id`     int NOT NULL,
  `level`          int NOT NULL DEFAULT 0,
  `state`          int NOT NULL DEFAULT 0,
  `state_start_at` bigint NOT NULL DEFAULT 0,
  `state_end_at`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_tech` (`user_id`, `technic_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `city_technics` (
  `city_id`    int NOT NULL,
  `technic_id` int NOT NULL,
  `level`      int NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `technic_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_technics` (
  `tid`         int NOT NULL,
  `name`        varchar(64) NOT NULL DEFAULT '',
  `description` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`tid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_technic_levels` (
  `id`           int NOT NULL AUTO_INCREMENT,
  `tid`          int NOT NULL,
  `level`        int NOT NULL DEFAULT 0,
  `upgrade_wood` bigint NOT NULL DEFAULT 0,
  `upgrade_rock` bigint NOT NULL DEFAULT 0,
  `upgrade_iron` bigint NOT NULL DEFAULT 0,
  `upgrade_food` bigint NOT NULL DEFAULT 0,
  `upgrade_gold` bigint NOT NULL DEFAULT 0,
  `upgrade_time` bigint NOT NULL DEFAULT 0,
  `description`  varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_tid_level` (`tid`, `level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 武将 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `heroes` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `user_id`       int NOT NULL DEFAULT 0,
  `city_id`       int NOT NULL DEFAULT 0,
  `name`          varchar(64) NOT NULL DEFAULT '',
  `sex`           tinyint NOT NULL DEFAULT 0,
  `face`          int NOT NULL DEFAULT 0,
  `state`         int NOT NULL DEFAULT 0,
  `level`         int NOT NULL DEFAULT 0,
  `hero_type`     int NOT NULL DEFAULT 0,
  `command_base`  int NOT NULL DEFAULT 0,
  `affairs_base`  int NOT NULL DEFAULT 0,
  `bravery_base`  int NOT NULL DEFAULT 0,
  `wisdom_base`   int NOT NULL DEFAULT 0,
  `bravery_add`   int NOT NULL DEFAULT 0,
  `wisdom_add`    int NOT NULL DEFAULT 0,
  `affairs_add`   int NOT NULL DEFAULT 0,
  `attack_base`   int NOT NULL DEFAULT 0,
  `attack_add_on` int NOT NULL DEFAULT 0,
  `defence_base`  int NOT NULL DEFAULT 0,
  `defence_add_on` int NOT NULL DEFAULT 0,
  `exp`           bigint NOT NULL DEFAULT 0,
  `hero_health`   int NOT NULL DEFAULT 0,
  `loyalty`       int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city_user` (`city_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hero_exprs` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `user_id`    int NOT NULL,
  `city_id`    int NOT NULL,
  `hero_id`    int NOT NULL,
  `expr_type`  int NOT NULL DEFAULT 0,
  `hours`      int NOT NULL DEFAULT 0,
  `state`      int NOT NULL DEFAULT 0,
  `started_at` bigint NOT NULL DEFAULT 0,
  `end_at`     bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_hero` (`hero_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_hero_levels` (
  `level`       int NOT NULL,
  `total_exp`   bigint NOT NULL DEFAULT 0,
  `upgrade_exp` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 兵种 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_soldiers` (
  `sid`         int NOT NULL,
  `type`        int NOT NULL DEFAULT 0,
  `fromcity`    tinyint NOT NULL DEFAULT 0,
  `name`        varchar(64) NOT NULL DEFAULT '',
  `hp`          bigint NOT NULL DEFAULT 0,
  `ap`          bigint NOT NULL DEFAULT 0,
  `dp`          bigint NOT NULL DEFAULT 0,
  `range`       bigint NOT NULL DEFAULT 0,
  `speed`       bigint NOT NULL DEFAULT 0,
  `carry`       bigint NOT NULL DEFAULT 0,
  `time_need`   bigint NOT NULL DEFAULT 0,
  `wood_need`   bigint NOT NULL DEFAULT 0,
  `rock_need`   bigint NOT NULL DEFAULT 0,
  `iron_need`   bigint NOT NULL DEFAULT 0,
  `food_need`   bigint NOT NULL DEFAULT 0,
  `gold_need`   bigint NOT NULL DEFAULT 0,
  `people_need` bigint NOT NULL DEFAULT 0,
  `food_use`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_soldier_conditions` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `sid`       int NOT NULL,
  `pre_type`  int NOT NULL DEFAULT 0,
  `pre_id`    int NOT NULL DEFAULT 0,
  `pre_level` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_sid_cond` (`sid`, `pre_type`, `pre_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `city_soldiers` (
  `city_id`    int NOT NULL,
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `soldier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `draft_queue` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `city_id`    int NOT NULL,
  `xy`         varchar(8) NOT NULL DEFAULT '',
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  `state`      int NOT NULL DEFAULT 0,
  `need_time`  bigint NOT NULL DEFAULT 0,
  `accmark`    int NOT NULL DEFAULT 0,
  `queued_at`  bigint NOT NULL DEFAULT 0,
  `started_at` bigint NOT NULL DEFAULT 0,
  `end_at`     bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city_xy` (`city_id`, `xy`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 行军 / 野地 / 战报 ──────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `troops` (
  `id`          int NOT NULL AUTO_INCREMENT,
  `user_id`     int NOT NULL,
  `city_id`     int NOT NULL,
  `hero_id`     int NOT NULL DEFAULT 0,
  `target_type` int NOT NULL DEFAULT 0,
  `target_id`   int NOT NULL DEFAULT 0,
  `task`        int NOT NULL DEFAULT 0,
  `state`       int NOT NULL DEFAULT 0,
  `soldiers`    text,
  `start_at`    bigint NOT NULL DEFAULT 0,
  `arrive_at`   bigint NOT NULL DEFAULT 0,
  `back_at`     bigint NOT NULL DEFAULT 0,
  `created_at`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city` (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `fields` (
  `id`             int NOT NULL AUTO_INCREMENT,
  `name`           varchar(64) NOT NULL DEFAULT '',
  `level`          int NOT NULL DEFAULT 0,
  `owner_uid`      int NOT NULL DEFAULT 0,
  `guard_soldiers` text,
  `guard_power`    bigint NOT NULL DEFAULT 0,
  `loot_food`      bigint NOT NULL DEFAULT 0,
  `loot_wood`      bigint NOT NULL DEFAULT 0,
  `loot_rock`      bigint NOT NULL DEFAULT 0,
  `loot_iron`      bigint NOT NULL DEFAULT 0,
  `loot_gold`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `battle_reports` (
  `id`          int NOT NULL AUTO_INCREMENT,
  `user_id`     int NOT NULL,
  `city_id`     int NOT NULL,
  `hero_id`     int NOT NULL DEFAULT 0,
  `target_desc` varchar(128) NOT NULL DEFAULT '',
  `result`      int NOT NULL DEFAULT 0,
  `attack_loss` text,
  `defend_loss` text,
  `looted`      text,
  `detail`      text,
  `created_at`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city` (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
