-- 0011_m7_battle.sql
-- M7 回合制战斗引擎（OLdBattleCron.php 1:1 复刻）所需表/列。
-- 幂等：CREATE TABLE IF NOT EXISTS + REPLACE INTO 种子；补列用 information_schema + PREPARE 动态 ALTER。
--
-- 表映射（原版 → 新库）：
--   mem_battle + sys_battle  → battles（合并；fightsoldier 承载原 $_SESSION[battle][id][fightsoldier]）
--   sys_battle_report        → battle_rounds
--   mem_battle_tactics       → battle_tactics
--   cfg_defence              → cfg_defence（数值为【合成值】：原 dump 丢失，见种子注释）
--   bak_troops               → bak_troops
--   mem_city_wounded         → city_wounded
--   sys_city_defence         → city_defences

SET NAMES utf8mb4;

-- ── battles（← mem_battle + sys_battle 合并）────────────────────────────
CREATE TABLE IF NOT EXISTS `battles` (
  `id`             int NOT NULL AUTO_INCREMENT,          -- mem_battle.id = sys_battle.id
  `type`           int NOT NULL DEFAULT 0,               -- 0野地 1攻城(占领) 2掠夺 3防守 (>=5 战场战不实现)
  `state`          int NOT NULL DEFAULT 0,               -- sys_battle.state: 0战斗中 1已结束
  `result`         int NOT NULL DEFAULT 3,               -- sys_battle.result: 0胜 1败 2平 3进行中
  `starttime`      bigint NOT NULL DEFAULT 0,
  `cid`            int NOT NULL DEFAULT 0,               -- sys_battle.cid = targetcid
  `attackuid`      int NOT NULL DEFAULT 0,
  `resistuid`      int NOT NULL DEFAULT 0,               -- 野地 NPC = 0（原版 <1000；新库玩家自增从 1 起，见 battle 包注释）
  `attacktroop`    int NOT NULL DEFAULT 0,               -- sys_battle.attacktroop = 攻方 troops.id
  `nexttime`       bigint NOT NULL DEFAULT 0,            -- 下一回合时间戳（+25s/回合）
  `round`          int NOT NULL DEFAULT 1,
  `attackcid`      int NOT NULL DEFAULT 0,
  `attackhid`      int NOT NULL DEFAULT 0,
  `attacksoldiers` longtext,                             -- 串: N,sid,count,sid,count,...
  `attackpos`      longtext,                             -- 串: N,sid,range,...
  `attackadd`      longtext,                             -- info 串: sid,sidtype,shoot,att,def,blood,speed,ap,dp,hp,uid;...
  `resistcid`      int NOT NULL DEFAULT 0,
  `resisthid`      int NOT NULL DEFAULT 0,
  `resistsoldiers` longtext,
  `resistpos`      longtext,
  `resistadd`      longtext,
  `resistdefence`  longtext,                             -- 初始 N,did,cnt,cnt（array2troop def=1）；回写 N,did,range,cnt
  `wallhp`         bigint NOT NULL DEFAULT 0,
  `walllevel`      int NOT NULL DEFAULT 0,
  `fieldrange`     bigint NOT NULL DEFAULT 0,            -- 战场距离（getBattleRange）
  `level`          int NOT NULL DEFAULT 0,               -- 野地等级（mem_battle.level）
  `attackstartcid` int NOT NULL DEFAULT 0,
  `resiststartcid` int NOT NULL DEFAULT 0,
  `fightsoldier`   longtext,                             -- 逐单位状态 JSON（← session；battle 结束清空）
  PRIMARY KEY (`id`),
  KEY `idx_due` (`state`, `nexttime`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── battle_rounds（← sys_battle_report）────────────────────────────────
CREATE TABLE IF NOT EXISTS `battle_rounds` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `battleid` int NOT NULL,
  `round`    int NOT NULL DEFAULT 0,
  `report`   longtext,
  PRIMARY KEY (`id`),
  KEY `idx_battle` (`battleid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── battle_tactics（← mem_battle_tactics）──────────────────────────────
CREATE TABLE IF NOT EXISTS `battle_tactics` (
  `battleid` int NOT NULL,
  `attack`   int NOT NULL,        -- 1攻方 0防守方（battleAdd 的 $state 怪癖见 battle 包注释）
  `stype`    int NOT NULL,
  `action`   int NOT NULL DEFAULT 0,
  `target`   int NOT NULL DEFAULT 0,
  `action2`  int NOT NULL DEFAULT 0,
  `target2`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`battleid`, `attack`, `stype`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_defence（← cfg_defence；原 dump 丢失，数值为【合成值】）──────────
-- 引擎仅使用 range(→gongjifanwei)/hp/ap/dp：陷阱/拒马/滚木/擂石为触发型（ap=0 自爆），
-- 箭塔 did=3 是唯一可反击城防，ap 参与反击伤害。range 取 100（近战位）；箭塔合成 1200（对齐弓箭兵射程档）。
CREATE TABLE IF NOT EXISTS `cfg_defence` (
  `did`   int NOT NULL,
  `name`  varchar(32) NOT NULL DEFAULT '',
  `hp`    bigint NOT NULL DEFAULT 0,
  `ap`    bigint NOT NULL DEFAULT 0,
  `dp`    bigint NOT NULL DEFAULT 0,
  `range` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
REPLACE INTO `cfg_defence` (`did`, `name`, `hp`, `ap`, `dp`, `range`) VALUES
(1, '陷阱', 1,   0,   0, 100),
(2, '拒马', 500, 0,   0, 100),
(3, '箭塔', 1000, 100, 50, 1200),
(4, '滚木', 300, 0,   0, 100),
(5, '擂石', 400, 0,   0, 100);

-- ── bak_troops（← bak_troops：开战时快照双方 troops 行）─────────────────
CREATE TABLE IF NOT EXISTS `bak_troops` (
  `rid`           int NOT NULL AUTO_INCREMENT,
  `id`            int NOT NULL,            -- troops.id
  `uid`           int NOT NULL DEFAULT 0,
  `cid`           int NOT NULL DEFAULT 0,
  `hid`           int NOT NULL DEFAULT 0,
  `targetcid`     int NOT NULL DEFAULT 0,
  `task`          int NOT NULL DEFAULT 0,
  `state`         int NOT NULL DEFAULT 0,
  `starttime`     bigint NOT NULL DEFAULT 0,
  `pathtime`      bigint NOT NULL DEFAULT 0,
  `noback`        int NOT NULL DEFAULT 0,
  `soldiers`      longtext,
  `resource`      varchar(128) NOT NULL DEFAULT '0',
  `battleid`      int NOT NULL DEFAULT 0,
  `people`        bigint NOT NULL DEFAULT 0,
  `fooduse`       bigint NOT NULL DEFAULT 0,
  `battlefieldid` int NOT NULL DEFAULT 0,  -- 战场战不实现，恒 0
  `startcid`      int NOT NULL DEFAULT 0,
  PRIMARY KEY (`rid`),
  KEY `idx_battle` (`battleid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_wounded（← mem_city_wounded：addCityWounded）───────────────────
CREATE TABLE IF NOT EXISTS `city_wounded` (
  `city_id`    int NOT NULL,
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `soldier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_defences（← sys_city_defence：城防器械存量）────────────────────
CREATE TABLE IF NOT EXISTS `city_defences` (
  `city_id` int NOT NULL,
  `did`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `did`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── heroes 补列（← sys_city_hero.speed_add_on：getUsersHeroBattleAdd 马速）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'speed_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `speed_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── users 补列（← sys_user.prestige/warprestige：addUserPrestige）────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'prestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `prestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'warprestige') = 0,
  'ALTER TABLE `users` ADD COLUMN `warprestige` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── troops 补列（← sys_troops.battleid/resource：战斗挂接与掠夺携带）─────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'troops' AND COLUMN_NAME = 'battleid') = 0,
  'ALTER TABLE `troops` ADD COLUMN `battleid` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'troops' AND COLUMN_NAME = 'resource') = 0,
  'ALTER TABLE `troops` ADD COLUMN `resource` varchar(128) NOT NULL DEFAULT ''0''', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;
