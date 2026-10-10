-- 0014_m11_world.sql
-- M11 世界地图（1:1 复刻 legacy server/game/WorldFunc.php 全部函数）所需表/列。
-- 口径来源（文件:行号）：
--   mem_world.type/ownercid/state/level  ← WorldFunc.php:25/206/271/343/351（getBlockData/getWorldFieldInfo/createCityFromLand）
--   cities.province/state                ← createCityFromLand:343 replace into sys_city(cid,uid,name,type,state,province)
--   users.union_pos                      ← markCity:870（union_pos 权限判定）
--   city_schedule.govern_count/last_govern_time/last_be_govern_time ← getGovernInfo:529、governOthers:667/674/772-778
--   user_favourites                      ← sys_favourites（WorldFunc.php:451/455/470/476/477/483/488/496/501）
--   user_inwars                          ← mem_user_inwar（startWar:213/226、getWorldCityInfo:99/124/155）
--   user_trickwars                       ← mem_user_trickwar（getWorldCityInfo:93/118/149 只读；计策模块未实现，恒空）
--   user_states                          ← sys_user_state（governOthers:610）
--   unions / union_marks / union_relations ← sys_union / sys_union_mark / sys_union_relation
--                                          （getWorldCityInfo:60/74/865、markCity:865/887/892/900、clearMark:921/929/931/935）
--   log_city_soldiers                    ← log_city_soldier（createCityFromLand:384、addCitySoldier utils.php:286）
--   cfg_nobility                         ← cfg_nobility（createCityFromLand:322/327）
--   cfg_world_type                       ← cfg_world_type（addFavourites:468、sendReport utils.php:140/154）
--   cfg_office_pos                       ← cfg_office_pos（getGovernInfo:533、governOthers:629）
--   cfg_name                             ← cfg_name（governOthers:627）
--   cfg_special_act                      ← cfg_special_act（getActionField:943）
--   city_lamsters / city_captives        ← mem_city_lamster / mem_city_captive（createCityFromLand:340/341 清理；
--                                          0012 已将 mem_city_captive 列为 M8 裁剪，此处为其补最小空表以保留 delete 语句）
--
-- ⚠ 合成数据声明：世界地格（mem_world）dump 已丢失，文件末尾所有 mem_world 行均为【合成演示数据】
--   （以演示城 cid=5（wid=5）为中心，覆盖 block 0/1）；cfg_nobility / cfg_world_type / cfg_office_pos /
--   cfg_name / cfg_special_act 的原始配置 dump 亦丢失，均为【合成演示数据】。数值/文案非原版精确复刻，
--   仅用于跑通 1:1 复刻逻辑与集成测试。代码中的常数（GRID_DISTANCE、NPC_UID_END=897、block*100、
--   cid2wid/wid2cid 公式等）逐字来自 PHP 源码，为权威值。
--
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE；种子 REPLACE INTO。
--
-- 裁剪（不建表，源码注释逐处声明）：
--   unionAssit(WorldFunc.php:988)：依赖 sys_union/sys_union_member（M8 整体排除联盟）→ 裁剪；
--   getLuoyangCityInfo(192) / getWorldCityInfo:176-184 洛阳分支 / getMapCity:794 type=4 分支：
--     M7/M8 已排除洛阳（log_luoyang_belong 不建表）→ 按无洛阳处理；
--   sys_mail_content / sys_mail_box（markCity:912/913 盟主发信）：新库无邮件表，按不发信处理；
--   sendSysInform 全服公告（checkIsInSili:444）：跨服/公告裁剪 → no-op；
--   mem_user_buffer 之外的其它跨服/战场/排行/反沉迷表：与前批次一致裁剪。

SET NAMES utf8mb4;

-- ── mem_world（← legacy mem_world；0002_minimal.sql 不在 migrate 链中，真实库无此表）──
-- 先建全表（wid/type/ownercid/state/level/province/jun），再对已存在的旧表补缺失列，保证幂等。
CREATE TABLE IF NOT EXISTS `mem_world` (
  `wid`      int NOT NULL,
  `type`     int NOT NULL DEFAULT 0,
  `ownercid` int NOT NULL DEFAULT 0,
  `state`    int NOT NULL DEFAULT 0,
  `level`    int NOT NULL DEFAULT 0,
  `province` varchar(32) NOT NULL DEFAULT '',
  `jun`      varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`wid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'mem_world' AND COLUMN_NAME = 'type') = 0,
  'ALTER TABLE `mem_world` ADD COLUMN `type` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'mem_world' AND COLUMN_NAME = 'ownercid') = 0,
  'ALTER TABLE `mem_world` ADD COLUMN `ownercid` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'mem_world' AND COLUMN_NAME = 'state') = 0,
  'ALTER TABLE `mem_world` ADD COLUMN `state` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'mem_world' AND COLUMN_NAME = 'level') = 0,
  'ALTER TABLE `mem_world` ADD COLUMN `level` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cities 补列（← sys_city.province/state）───────────────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cities' AND COLUMN_NAME = 'province') = 0,
  'ALTER TABLE `cities` ADD COLUMN `province` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cities' AND COLUMN_NAME = 'state') = 0,
  'ALTER TABLE `cities` ADD COLUMN `state` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── users 补列（← sys_user.union_pos：markCity 联盟官职判定）───────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'union_pos') = 0,
  'ALTER TABLE `users` ADD COLUMN `union_pos` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_schedule 补列（← mem_city_schedule 政令计数/时间）─────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_schedule' AND COLUMN_NAME = 'govern_count') = 0,
  'ALTER TABLE `city_schedule` ADD COLUMN `govern_count` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_schedule' AND COLUMN_NAME = 'last_govern_time') = 0,
  'ALTER TABLE `city_schedule` ADD COLUMN `last_govern_time` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_schedule' AND COLUMN_NAME = 'last_be_govern_time') = 0,
  'ALTER TABLE `city_schedule` ADD COLUMN `last_be_govern_time` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── user_favourites（← sys_favourites）────────────────────────────────────
CREATE TABLE IF NOT EXISTS `user_favourites` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `uid`      int NOT NULL,
  `cid`      int NOT NULL,
  `name`     varchar(64) NOT NULL DEFAULT '',
  `comments` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_inwars（← mem_user_inwar：宣战关系）──────────────────────────────
CREATE TABLE IF NOT EXISTS `user_inwars` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `uid`       int NOT NULL,
  `targetuid` int NOT NULL,
  `state`     int NOT NULL DEFAULT 0,
  `endtime`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_uid_target` (`uid`, `targetuid`),
  KEY `idx_endtime` (`endtime`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_trickwars（← mem_user_trickwar：getWorldCityInfo 只读；计策模块未实现，恒空）──
CREATE TABLE IF NOT EXISTS `user_trickwars` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `uid`       int NOT NULL,
  `targetuid` int NOT NULL,
  `endtime`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_uid_target` (`uid`, `targetuid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_states（← sys_user_state：封禁/休假到期）──────────────────────────
CREATE TABLE IF NOT EXISTS `user_states` (
  `uid`      int NOT NULL,
  `forbiend` bigint NOT NULL DEFAULT 0,
  `vacend`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── unions（← sys_union：仅 id/name/prestige，供 getWorldCityInfo join 与 markCity 存在性判定）──
CREATE TABLE IF NOT EXISTS `unions` (
  `id`       int NOT NULL,
  `name`     varchar(64) NOT NULL DEFAULT '',
  `prestige` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── union_marks（← sys_union_mark：联盟城池标记，getBlockData/markCity/clearMark）──
CREATE TABLE IF NOT EXISTS `union_marks` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `unionid` int NOT NULL,
  `cid`     int NOT NULL,
  `endtime` bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  `wid`     int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_unionid` (`unionid`),
  KEY `idx_cid` (`cid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── union_relations（← sys_union_relation：联盟外交关系）───────────────────
CREATE TABLE IF NOT EXISTS `union_relations` (
  `unionid` int NOT NULL,
  `target`  int NOT NULL,
  `type`    int NOT NULL DEFAULT 0,   -- 0友好 1中立 2敌对
  `time`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`unionid`, `target`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_city_soldiers（← log_city_soldier：addCitySoldier type=9、createCityFromLand type=10）──
CREATE TABLE IF NOT EXISTS `log_city_soldiers` (
  `id`    int NOT NULL AUTO_INCREMENT,
  `cid`   int NOT NULL,
  `sid`   int NOT NULL,
  `uid`   int NOT NULL DEFAULT 0,
  `count` bigint NOT NULL DEFAULT 0,
  `type`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_city_soldier_log` (`cid`, `sid`, `uid`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_lamsters / city_captives（← mem_city_lamster / mem_city_captive：createCityFromLand 清理）──
CREATE TABLE IF NOT EXISTS `city_lamsters` (
  `city_id`    int NOT NULL,
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `soldier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `city_captives` (
  `city_id`    int NOT NULL,
  `soldier_id` int NOT NULL,
  `count`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`city_id`, `soldier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 配置表（← legacy 同名；数据丢失 → 合成种子见文末）─────────────────────
CREATE TABLE IF NOT EXISTS `cfg_nobility` (
  `id`         int NOT NULL,
  `name`       varchar(32) NOT NULL DEFAULT '',
  `city_count` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_world_type` (
  `type` int NOT NULL,
  `name` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_office_pos` (
  `id`   int NOT NULL,
  `name` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_name` (
  `name`  varchar(64) NOT NULL,
  `value` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_special_act` (
  `cid`       int NOT NULL,
  `count`     int NOT NULL DEFAULT 0,
  `starttime` varchar(16) NOT NULL DEFAULT '',
  PRIMARY KEY (`cid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ══════════════════════════════════════════════════════════════════════════
-- 以下全部为【合成演示数据】
-- ══════════════════════════════════════════════════════════════════════════

-- 世界地格：以演示城 cid=5（wid=cid2wid(5)=5）为中心，覆盖 block 0(0-99)/block 1(100-199)。
-- type：0无主地/城池占位 1平地(可筑城/附属) 2山寨野地 3其它野地；state=1 表示战乱；
-- ownercid>0 表示该格归属某城池（附属平地）。
REPLACE INTO `mem_world` (`wid`,`type`,`ownercid`,`state`,`level`,`province`,`jun`) VALUES
(5,   0, 5,    0, 3, 1, 1),   -- 演示城 cid=5 所在地格
(4,   1, 5,    0, 0, 1, 1),   -- cid=5 的附属平地（可筑城）
(6,   1, 5,    0, 0, 1, 1),
(15,  1, 5,    0, 0, 1, 1),
(7,   3, 0,    0, 0, 1, 1),
(8,   0, 0,    0, 0, 1, 2),   -- 无主平地（jun=2）
(9,   0, 0,    0, 0, 1, 2),
(16,  3, 0,    1, 0, 1, 1),   -- 战乱野地（state=1）
(25,  2, 0,    0, 2, 1, 1),   -- 2 级山寨野地
(35,  2, 0,    0, 1, 1, 1),   -- 1 级山寨野地
(105, 1, 0,    0, 0, 1, 2),   -- block 1
(120, 2, 0,    0, 4, 1, 2);

-- 野地/地形类型名（addFavourites、sendReport 野地名回退）
REPLACE INTO `cfg_world_type` (`type`,`name`) VALUES
(1,'平原'),(2,'森林'),(3,'山地'),(4,'湖泊'),(5,'沙漠');

-- 爵位（id 0=无爵；city_count 为可统治城池上限）——合成递增曲线
REPLACE INTO `cfg_nobility` (`id`,`name`,`city_count`) VALUES
(0,'平民',2),(1,'公士',3),(2,'上造',4),(3,'簪枭',5),(4,'不更',6),
(5,'大夫',7),(6,'官大夫',8),(7,'公大夫',9),(8,'公乘',10),(9,'五大夫',11),
(10,'左庶长',12),(11,'右庶长',13),(12,'左更',14),(13,'中更',15),(14,'右更',16),
(15,'少上造',17),(16,'大上造',18),(17,'驷车庶长',19),(18,'大庶长',20),(19,'关内侯',21),
(20,'诸侯',30);

-- 官职（officepos；getGovernInfo/GetMaxCountByOfficePos）
REPLACE INTO `cfg_office_pos` (`id`,`name`) VALUES
(0,'布衣'),(1,'亭长'),(2,'县令'),(3,'太守'),(4,'刺史'),(5,'州牧'),(6,'都尉'),(7,'校尉'),
(8,'中郎将'),(9,'偏将军'),(10,'裨将军'),(11,'牙门将'),(12,'杂号将军'),(13,'四镇将军'),
(14,'四征将军'),(15,'大将军');

-- 名城等级名（governOthers:626-627：cityTypeNameField = "big_city_"+cityType → PHP 整数化后为 cityType 本身，此处按 '1'~'4' 存值）
REPLACE INTO `cfg_name` (`name`,`value`) VALUES
('1','县'),('2','郡'),('3','州'),('4','都');

-- 特殊活动（getActionField；starttime='HH:MM'，当日此刻后生效）。
-- 注意 getActionField:964 `intval($cid)<1000 → wid=-1`，故活动 cid 必须 >=1000 才返回真实 wid。
-- 先清空以保证幂等（历史版本曾用 cid=5，REPLACE 不会删除旧主键行）。
DELETE FROM `cfg_special_act`;
REPLACE INTO `cfg_special_act` (`cid`,`count`,`starttime`) VALUES
(1000, 1, '00:00');
