-- 0011_m7_battle.sql
-- M7 战斗（1:1 复刻 legacy OLdBattleCron.php 回合引擎 + getBattleResult 结算）。
-- 口径来源（文件:行号）：
--   mem_battle        ← cfg_js.php:535 replace into mem_battle(...) 列清单
--   sys_battle        ← cfg_js.php:472  insert into sys_battle(type,starttime,cid,attackuid,resistuid,attacktroop,result)
--   sys_battle_report ← OLdBattleCron.php:485 insert into sys_battle_report(battleid,round,report)
--   mem_battle_tactics← TroopFunc.php:672 replace into mem_battle_tactics(battleid,attack,stype,action,target,action2,target2)
--   bak_troops        ← cfg_js.php:529 insert into bak_troops(...) 列清单
--   mem_city_wounded  ← cfg_js.php:1383 addCityWounded
--   sys_city_defence  ← 0002_minimal.sql:156（已存在同名列 sys_city_defence；新库统一为 city_defences）
--   cfg_defence       ← OLdBattleCron.php:103 select * from cfg_defence（数据缺失，合成并声明）
--
-- 说明：legacy 用 $_SESSION['battle'][$id] 承载回合内态（fightsoldier/resistnpc/attacknpc/battle_end）。
-- 新栈无 PHP session，改为将 fightsoldier 以 JSON 持久化到 battles.engine_json，每次惰性结算 load→跑一回合→save。
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE；种子 REPLACE INTO。

SET NAMES utf8mb4;

-- 一次性清理：早期草稿曾用名 battle_round_reports，正式名 battle_rounds。
DROP TABLE IF EXISTS `battle_round_reports`;

-- ── battles（← mem_battle + sys_battle 合并：进行中的战斗快照）──────────────
CREATE TABLE IF NOT EXISTS `battles` (
  `id`             int NOT NULL AUTO_INCREMENT,
  `type`           int NOT NULL DEFAULT 0,       -- 0野地/抢占 1占领城池 2掠夺城池 (4战场：新库无战场，保留位)
  `state`          int NOT NULL DEFAULT 0,       -- 0进行中 1已结束（对齐 sys_battle.state）
  `result`         int NOT NULL DEFAULT 3,       -- 0胜 1败 2平 3未定（对齐 sys_battle.result）
  `starttime`      bigint NOT NULL DEFAULT 0,
  `cid`            int NOT NULL DEFAULT 0,       -- sys_battle.cid（进攻方出发城）
  `attackuid`      int NOT NULL DEFAULT 0,
  `resistuid`      int NOT NULL DEFAULT 0,
  `attacktroop`    int NOT NULL DEFAULT 0,       -- 进攻方 sys_troops.id
  `resisttroops`   text,                          -- sys_battle.resisttroops（防守方部队 id 串）
  `resistdefence`  text,                          -- 城防串 "N,did,oldcnt,cnt,..."
  -- mem_battle 列
  `round`          int NOT NULL DEFAULT 0,
  `nexttime`       bigint NOT NULL DEFAULT 0,
  `attackcid`      int NOT NULL DEFAULT 0,
  `attackhid`      int NOT NULL DEFAULT 0,
  `attacksoldiers` text,
  `attackpos`      text,
  `attackadd`      text,
  `resistcid`      int NOT NULL DEFAULT 0,
  `resisthid`      int NOT NULL DEFAULT 0,
  `resistsoldiers` text,
  `resistpos`      text,
  `resistadd`      text,
  `wallhp`         bigint NOT NULL DEFAULT 0,
  `walllevel`      int NOT NULL DEFAULT 0,
  `fieldrange`     int NOT NULL DEFAULT 0,
  `level`          int NOT NULL DEFAULT 0,
  `attackstartcid` int NOT NULL DEFAULT 0,
  `resiststartcid` int NOT NULL DEFAULT 0,
  -- 引擎态（替代 $_SESSION['battle'][id]['fightsoldier']）
  `engine_json`    mediumtext,
  PRIMARY KEY (`id`),
  KEY `idx_nexttime` (`nexttime`),
  KEY `idx_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── battle_rounds（← sys_battle_report：逐回合原始战报行）─────────────────
CREATE TABLE IF NOT EXISTS `battle_rounds` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `battleid` int NOT NULL,
  `round`    int NOT NULL DEFAULT 0,
  `report`   mediumtext,
  PRIMARY KEY (`id`),
  KEY `idx_battle` (`battleid`, `round`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── battle_tactics（← mem_battle_tactics：兵种战术）────────────────────────
CREATE TABLE IF NOT EXISTS `battle_tactics` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `battleid` int NOT NULL,
  `attack`   int NOT NULL DEFAULT 0,   -- 1进攻方 0防守方
  `stype`    int NOT NULL DEFAULT 0,   -- 兵种 type
  `action`   int NOT NULL DEFAULT 0,   -- 1前进 3后退
  `target`   int NOT NULL DEFAULT 0,   -- 目标（兵种 type 值；15=箭塔）
  `action2`  int NOT NULL DEFAULT 0,
  `target2`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_bt` (`battleid`, `attack`, `stype`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── bak_troops（← bak_troops：战斗期间的双方部队快照）──────────────────────
CREATE TABLE IF NOT EXISTS `bak_troops` (
  `id`         int NOT NULL,
  `uid`        int NOT NULL DEFAULT 0,
  `cid`        int NOT NULL DEFAULT 0,
  `hid`        int NOT NULL DEFAULT 0,
  `targetcid`  int NOT NULL DEFAULT 0,
  `task`       int NOT NULL DEFAULT 0,
  `state`      int NOT NULL DEFAULT 0,
  `starttime`  bigint NOT NULL DEFAULT 0,
  `pathtime`   bigint NOT NULL DEFAULT 0,
  `noback`     int NOT NULL DEFAULT 0,
  `soldiers`   text,
  `resource`   text,
  `battleid`   int NOT NULL DEFAULT 0,
  `people`     bigint NOT NULL DEFAULT 0,
  `fooduse`    bigint NOT NULL DEFAULT 0,
  `startcid`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_battle` (`battleid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_wounded（← mem_city_wounded：伤兵营）──────────────────────────────
CREATE TABLE IF NOT EXISTS `city_wounded` (
  `city_id`    int NOT NULL,
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `soldier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_defences（← sys_city_defence：城防器械存量）──────────────────────
CREATE TABLE IF NOT EXISTS `city_defences` (
  `city_id` int NOT NULL,
  `did`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_defence（← cfg_defence：5 类城防器械 hp/ap/dp/range）──────────────
-- 警告：原始 cfg_defence 数据在迁移中丢失，下列数值为合成值（与 M6 cfg_technic_levels 同样声明）。
CREATE TABLE IF NOT EXISTS `cfg_defence` (
  `did`     int NOT NULL,
  `name`    varchar(32) NOT NULL DEFAULT '',
  `hp`      bigint NOT NULL DEFAULT 0,
  `ap`      bigint NOT NULL DEFAULT 0,
  `dp`      bigint NOT NULL DEFAULT 0,
  `g_range` bigint NOT NULL DEFAULT 0,   -- legacy 列名 range（保留字，改 g_range）
  PRIMARY KEY (`did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_defence` (`did`,`name`,`hp`,`ap`,`dp`,`g_range`) VALUES
  (1, '陷阱', 80000,  300,  200, 30),
  (2, '拒马', 120000, 400,  350, 40),
  (3, '箭塔', 150000, 900,  500, 260),
  (4, '滚木', 200000, 1200, 700, 50),
  (5, '擂石', 260000, 1600, 900, 60);

-- ── users.prestige（← sys_user.prestige/warprestige：战报"君主声望"来源）───
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'prestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `prestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'warprestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `warprestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'war_attack_prestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `war_attack_prestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'war_defence_prestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `war_defence_prestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ── troops.arrive_at 已存在；补 troops.battleid（战斗挂靠）────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'troops' AND COLUMN_NAME = 'battleid') = 0,
  'ALTER TABLE `troops` ADD COLUMN `battleid` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ── heroes.speed_add_on（← sys_city_hero.speed_add_on：getUsersHeroBattleAdd 用）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'speed_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `speed_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
