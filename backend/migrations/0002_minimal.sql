-- 0002_minimal.sql
-- 手写最小表集：仅覆盖「登录 → 进入城市 → 拉取城市信息」切片所需表。
-- 列名从 backend 的 SELECT/INSERT 与 legacy server/game 代码反推（无 mysqldump 可用时的兜底基线）。
-- 说明：真实运营库请用 0001_baseline.sql（mysqldump --no-data bloodwar）。
-- 所有语句幂等，可重复执行。

SET NAMES utf8;

-- ── 账号 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `test_passport` (
  `passport` varchar(64) NOT NULL,
  `password` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`passport`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_user` (
  `uid`      int NOT NULL AUTO_INCREMENT,
  `passtype` varchar(32) NOT NULL DEFAULT '',
  `passport` varchar(64) NOT NULL DEFAULT '',
  `name`     varchar(64) NOT NULL DEFAULT '',
  `group`    int NOT NULL DEFAULT 0,
  `state`    tinyint NOT NULL DEFAULT 3,
  `money`    bigint NOT NULL DEFAULT 0,
  `regtime`  int NOT NULL DEFAULT 0,
  `domainid` int NOT NULL DEFAULT 0,
  `honour`   int NOT NULL DEFAULT 0,
  `nobility` int NOT NULL DEFAULT 0,
  `lastcid`  int NOT NULL DEFAULT 0,
  `officepos` int NOT NULL DEFAULT 0,
  `union_id` int NOT NULL DEFAULT 0,
  `rank`     int NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`),
  KEY `idx_passport` (`passport`, `passtype`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 会话 / 在线 ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `sys_sessions` (
  `uid` int NOT NULL,
  `sid` bigint NOT NULL DEFAULT 0,
  `ip`  bigint unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_online` (
  `uid`          int NOT NULL,
  `lastupdate`   int NOT NULL DEFAULT 0,
  `onlineupdate` int NOT NULL DEFAULT 0,
  `onlinetime`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_user_comming` (
  `uid`         int NOT NULL,
  `site_id`     int NOT NULL DEFAULT 0,
  `page_id`     int NOT NULL DEFAULT 0,
  `sub_page_id` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 公告 / 全局状态 ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `sys_announce` (
  `id`      int NOT NULL,
  `content` text,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `mem_state` (
  `state` int NOT NULL,
  `value` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 城池 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `sys_city` (
  `cid`        int NOT NULL AUTO_INCREMENT,
  `uid`        int NOT NULL DEFAULT 0,
  `name`       varchar(64) NOT NULL DEFAULT '',
  `is_special` tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`),
  KEY `idx_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `mem_city_resource` (
  `cid`             int NOT NULL,
  `wood`            bigint NOT NULL DEFAULT 0,
  `wood_add`        bigint NOT NULL DEFAULT 0,
  `wood_max`        bigint NOT NULL DEFAULT 0,
  `rock`            bigint NOT NULL DEFAULT 0,
  `rock_add`        bigint NOT NULL DEFAULT 0,
  `rock_max`        bigint NOT NULL DEFAULT 0,
  `iron`            bigint NOT NULL DEFAULT 0,
  `iron_add`        bigint NOT NULL DEFAULT 0,
  `iron_max`        bigint NOT NULL DEFAULT 0,
  `food`            bigint NOT NULL DEFAULT 0,
  `food_add`        bigint NOT NULL DEFAULT 0,
  `food_max`        bigint NOT NULL DEFAULT 0,
  `food_army_use`   bigint NOT NULL DEFAULT 0,
  `gold`            bigint NOT NULL DEFAULT 0,
  `gold_rate`       int NOT NULL DEFAULT 0,
  `gold_max`        bigint NOT NULL DEFAULT 0,
  `people`          bigint NOT NULL DEFAULT 0,
  `people_max`      bigint NOT NULL DEFAULT 0,
  `people_stable`   bigint NOT NULL DEFAULT 0,
  `people_working`  bigint NOT NULL DEFAULT 0,
  `people_building` bigint NOT NULL DEFAULT 0,
  `morale`          int NOT NULL DEFAULT 0,
  `tax`             int NOT NULL DEFAULT 0,
  `complaint`       int NOT NULL DEFAULT 0,
  `hero_fee`        bigint NOT NULL DEFAULT 0,
  `vacation`        int NOT NULL DEFAULT 0,
  `forbidden`       int NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_building` (
  `cid`              int NOT NULL,
  `bid`              int NOT NULL,
  `x`                int NOT NULL DEFAULT 0,
  `y`                int NOT NULL DEFAULT 0,
  `level`            int NOT NULL DEFAULT 0,
  `state`            int NOT NULL DEFAULT 0,
  `state_starttime`  int NOT NULL DEFAULT 0,
  `state_endtime`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`, `bid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `cfg_building` (
  `bid`         int NOT NULL,
  `name`        varchar(64) NOT NULL DEFAULT '',
  `description` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`bid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `cfg_building_level` (
  `bid`          int NOT NULL,
  `level`        int NOT NULL DEFAULT 0,
  `description`  varchar(255) NOT NULL DEFAULT '',
  `using_people` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`bid`, `level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_city_technic` (
  `cid`   int NOT NULL,
  `tid`   int NOT NULL,
  `level` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`, `tid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_city_soldier` (
  `cid`   int NOT NULL,
  `sid`   int NOT NULL,
  `count` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`, `sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_city_defence` (
  `cid`   int NOT NULL,
  `did`   int NOT NULL,
  `count` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`cid`, `did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_alarm` (
  `uid`  int NOT NULL,
  `task` int NOT NULL DEFAULT 0,
  `mail` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 武将 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `sys_city_hero` (
  `hid`          int NOT NULL AUTO_INCREMENT,
  `uid`          int NOT NULL DEFAULT 0,
  `cid`          int NOT NULL DEFAULT 0,
  `name`         varchar(64) NOT NULL DEFAULT '',
  `sex`          tinyint NOT NULL DEFAULT 0,
  `face`         int NOT NULL DEFAULT 0,
  `state`        int NOT NULL DEFAULT 0,
  `level`        int NOT NULL DEFAULT 0,
  `herotype`     int NOT NULL DEFAULT 0,
  `command_base` int NOT NULL DEFAULT 0,
  `affairs_base` int NOT NULL DEFAULT 0,
  `bravery_base` int NOT NULL DEFAULT 0,
  `wisdom_base`  int NOT NULL DEFAULT 0,
  `loyalty`      int NOT NULL DEFAULT 0,
  PRIMARY KEY (`hid`),
  KEY `idx_cid_uid` (`cid`, `uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `mem_hero_blood` (
  `hid`        int NOT NULL,
  `force`      int NOT NULL DEFAULT 0,
  `force_max`  int NOT NULL DEFAULT 0,
  `energy`     int NOT NULL DEFAULT 0,
  `energy_max` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`hid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `sys_troops` (
  `hid`       int NOT NULL,
  `targetcid` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`hid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 世界 / 配置 ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `mem_world` (
  `wid`      int NOT NULL,
  `province` varchar(32) NOT NULL DEFAULT '',
  `jun`      varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`wid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE IF NOT EXISTS `cfg_soldier_special_city` (
  `cid`  int NOT NULL,
  `sid`  int NOT NULL DEFAULT 0,
  `type` varchar(8) NOT NULL DEFAULT '',
  PRIMARY KEY (`cid`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 日志 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `log_login` (
  `id`   int NOT NULL AUTO_INCREMENT,
  `uid`  int NOT NULL DEFAULT 0,
  `ip`   bigint unsigned NOT NULL DEFAULT 0,
  `time` int NOT NULL DEFAULT 0,
  `sip`  varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 活动（切片暂未读写，占位以对齐计划表清单）────────────────────────────
CREATE TABLE IF NOT EXISTS `sys_activity` (
  `uid`  int NOT NULL,
  `type` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- ── 最小种子数据 ────────────────────────────────────────────────────────
INSERT INTO `mem_state` (`state`, `value`) VALUES
  (2, 1),      -- 服务器状态：1=正常
  (3, 1),      -- 客户端版本号
  (4, 5000),   -- 最大在线人数
  (100, 100000), -- 最大角色数
  (150, 0)     -- 抽奖开关
ON DUPLICATE KEY UPDATE `value` = VALUES(`value`);

INSERT INTO `sys_announce` (`id`, `content`) VALUES (1, '欢迎来到血战')
ON DUPLICATE KEY UPDATE `content` = VALUES(`content`);