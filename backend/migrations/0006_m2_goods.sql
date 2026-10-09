-- 0006_m2_goods.sql
-- M2 道具系统（GoodsFunc.php useGoods / utils.php addGoods/reduceGoods / loadUserGoods）所需表与列。
-- 幂等：information_schema 判断 + PREPARE 动态 ALTER；建表 CREATE TABLE IF NOT EXISTS；种子 REPLACE INTO。

SET NAMES utf8mb4;

-- users 补 lastcid（legacy sys_user.lastcid：useGoods 内部 addCityResources 目标城）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'lastcid') = 0,
  'ALTER TABLE `users` ADD COLUMN `lastcid` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- city_resources 补 gold_rate（useShuiLiBian:2062 `set m.gold_rate=125`；getCityProduct 黄金产出比例）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'gold_rate') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `gold_rate` int NOT NULL DEFAULT 100', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cfg_goods（← legacy cfg_goods）───────────────────────────────────────
-- 注：legacy cfg_goods dump 已丢失，name 取 lang.php/代码注释；value（出售单价×500 金，sellGoods 用）无权威数值，置 0。
CREATE TABLE IF NOT EXISTS `cfg_goods` (
  `gid`      int NOT NULL,
  `name`     varchar(64) NOT NULL DEFAULT '',
  `group_id` int NOT NULL DEFAULT 0,
  `position` int NOT NULL DEFAULT 0,
  `value`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`gid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_goods（← sys_goods 背包）────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `user_goods` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `gid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_gid` (`user_id`, `gid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_goods（← log_goods 流水；reduceGoods 武魂/成就统计依赖，保留结构）──
CREATE TABLE IF NOT EXISTS `log_goods` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `gid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_buffers（← mem_user_buffer：道具 buff，uniq uid+buftype）────────
CREATE TABLE IF NOT EXISTS `user_buffers` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `user_id`  int NOT NULL,
  `buftype`  int NOT NULL,
  `bufparam` int NOT NULL DEFAULT 0,
  `endtime`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_buf` (`user_id`, `buftype`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_buffers（← mem_city_buffer：城池皮肤时效 buff，bufparam='705'）──
CREATE TABLE IF NOT EXISTS `city_buffers` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `city_id`  int NOT NULL,
  `buftype`  int NOT NULL,
  `bufparam` varchar(32) NOT NULL DEFAULT '',
  `endtime`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city` (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- city_buffers 补唯一键（useAdvancedConstructionPlan/changeCityMap 的 on-duplicate 依赖）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_buffers' AND INDEX_NAME = 'uniq_city_buf') = 0,
  'ALTER TABLE `city_buffers` ADD UNIQUE KEY `uniq_city_buf` (`city_id`, `buftype`)', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── user_action_log（← sys_user_action_log：免战牌连用计数，uniq uid+action）──
CREATE TABLE IF NOT EXISTS `user_action_log` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `action`  varchar(32) NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `mark`    varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_action` (`user_id`, `action`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_schedule（← mem_user_schedule：today_war_count 供请战书使用）──
CREATE TABLE IF NOT EXISTS `user_schedule` (
  `user_id`          int NOT NULL,
  `today_war_count`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- city_schedule 补 last_anming（useAnMingGaoShi:2542 安民告示 72h 冷却）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_schedule' AND COLUMN_NAME = 'last_anming') = 0,
  'ALTER TABLE `city_schedule` ADD COLUMN `last_anming` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── 种子：M2 分发到的 gid ────────────────────────────────────────────────
REPLACE INTO `cfg_goods` (`gid`, `name`, `group_id`, `position`) VALUES
(1,'传音符',1,1),(2,'神农锄',1,2),(3,'鲁班斧',1,3),(4,'开山锤',1,4),(5,'玄铁炉',1,5),
(6,'陷阵战鼓',1,6),(7,'八阵图',1,7),(8,'墨家残卷',1,8),(9,'墨家图纸',1,9),(10,'墨家典籍',1,10),
(12,'免战牌',1,12),(13,'锦囊',1,13),(15,'盟主令',1,15),(16,'青铜礼盒',1,16),(17,'白银礼盒',1,17),(18,'黄金礼盒',1,18),
(22,'洗髓丹',1,22),(23,'招贤榜',1,23),(25,'青囊书',1,25),
(44,'高级神农锄',1,44),(45,'高级鲁班斧',1,45),(46,'高级开山锤',1,46),(47,'高级玄铁炉',1,47),
(48,'高级陷阵战鼓',1,48),(49,'高级八阵图',1,49),(50,'古朴木盒',1,50),(51,'清仓令',1,51),(52,'墨家秘笈',1,52),
(54,'税吏鞭',1,54),(55,'高级税吏鞭',1,55),(56,'徭役令',1,56),(57,'典民令',1,57),(58,'安民告示',1,58),
(59,'军旗',1,59),(60,'考工记·上',1,60),(61,'考工记·中',1,61),(62,'考工记·下',1,62),(63,'韩信三篇',1,63),(64,'备城门',1,64),
(85,'金砖',2,85),(86,'金条',2,86),(87,'辎重包(粮)',2,87),(88,'辎重包(木)',2,88),(89,'辎重包(石)',2,89),(90,'辎重包(铁)',2,90),
(91,'辎重箱(粮)',2,91),(92,'辎重箱(木)',2,92),(93,'辎重箱(石)',2,93),(94,'辎重箱(铁)',2,94),
(117,'高级推恩令',1,117),(119,'宝藏盒',1,119),(120,'商队契约',1,120),(124,'推恩令',1,124),
(133,'军令状',1,133),(134,'赦免文书',1,134),(138,'请战书',1,138),(139,'太平要术',1,139),
(142,'巡查令',1,142),(154,'求贤诏',1,154),(158,'誓师文书',1,158),
(164,'沙场令',1,164),(165,'高级青囊书',1,165),(166,'高级徭役令',1,166),
(10013,'资源礼包',2,10013),(10321,'募兵令',1,10321),
(10932,'城池皮肤I',3,10932),(10933,'城池皮肤II',3,10933),(10934,'城池皮肤III',3,10934),
(10935,'城池皮肤IV',3,10935),(10936,'城池皮肤V',3,10936),(10937,'城池皮肤VI',3,10937),
(10957,'成长卷I',1,10957),(10958,'成长卷II',1,10958),(10959,'成长卷III',1,10959),
(10996,'王者之城',3,10996),(11021,'城池皮肤VII',3,11021),(11022,'城池皮肤VIII',3,11022),(11078,'城池皮肤IX',3,11078),
(14,'盟主密诏',1,14),(19,'青铜钥匙',1,19),(20,'白银钥匙',1,20),(21,'黄金钥匙',1,21),
(41,'珠宝盒I',1,41),(42,'珠宝盒II',1,42),(43,'珠宝盒III',1,43),(83,'玫瑰花',1,83),(140,'高级安民告示',1,140),
(95,'装备箱I',1,95),(96,'装备箱II',1,96),(97,'装备箱III',1,97),
(101,'武器箱',1,101),(102,'头盔箱',1,102),(103,'铠甲箱',1,103),(104,'战靴箱',1,104),(105,'坐骑箱',1,105),
(106,'兵符箱',1,106),(107,'宝物箱',1,107),(108,'战旗箱',1,108),(109,'护腕箱',1,109),(110,'腰带箱',1,110),(111,'披风箱',1,111),(112,'面具箱',1,112),
(145,'武器架I',1,145),(146,'武器架II',1,146),(147,'神秘传音符',1,147),(150,'答题礼包',1,150),(153,'宝石箱',1,153),(200,'宝珠盒',1,200),
(250,'名将卡',1,250),(251,'军令I',1,251),(252,'军令II',1,252),(253,'军令III',1,253),(254,'军令IV',1,254),
(1015,'武魂碎片礼盒',1,1015),(1016,'宝珠盒',1,1016),(10017,'钥匙链',1,10017),
(10001,'新手礼包',1,10001),(10002,'升级礼包',1,10002),(10003,'白银礼包I',1,10003),(10004,'白银礼包II',1,10004),
(10005,'黄金礼包I',1,10005),(10006,'黄金礼包II',1,10006),(10007,'黄金礼包III',1,10007),(10008,'超值建设礼包',1,10008),
(10009,'超值城主礼包',1,10009),(10010,'功勋礼包',1,10010),(10011,'生产礼包',1,10011),(10012,'高级生产礼包',1,10012),
(10014,'过期道具I',1,10014),(10015,'过期道具II',1,10015),(10016,'活动道具',1,10016),(10072,'活动道具',1,10072),
(10018,'活动道具',1,10018),(10019,'活动道具',1,10019),(10020,'活动道具',1,10020),(10021,'活动道具',1,10021),
(10022,'活动道具',1,10022),(10023,'活动道具',1,10023),(10024,'活动道具',1,10024),
(10083,'上级青囊书',1,10083),(10084,'强陷阵战鼓',1,10084),(10085,'强八卦阵图',1,10085),
(10158,'高级建筑图纸',1,10158),(10215,'热血勇士召唤令',1,10215),(11227,'统领召唤令',1,11227),(11228,'侍卫召唤令',1,11228),(12051,'热血家将',1,12051),
(10254,'过期道具',1,10254),(10270,'过期道具',1,10270),(10279,'过期道具',1,10279),(10282,'过期道具',1,10282),
(10303,'彩蛋盒I',1,10303),(10304,'彩蛋盒II',1,10304),(10308,'收藏道具I',1,10308),(10314,'万圣节面具',1,10314),(10417,'收藏道具II',1,10417),
(10333,'诏安令',1,10333),(10428,'玄冰匣',1,10428),(10487,'竹编箱子',1,10487),(10931,'王者兵符',1,10931),(10816,'王者之城图纸',1,10816),
(161501,'献帝诏书',1,161501),(19061,'南蛮战书',1,19061),(19062,'山越战书',1,19062),(19063,'匈奴战书',1,19063),
(8887,'龙渊装备箱I',1,8887),(8888,'龙渊装备箱II',1,8888),(8889,'龙渊装备箱III',1,8889),(8890,'龙渊装备箱IV',1,8890),
(8891,'龙渊铁装备箱I',1,8891),(8892,'龙渊铁装备箱II',1,8892),(8893,'联盟战箱I',1,8893),(8894,'联盟战箱II',1,8894),
(50101,'官府礼包I',1,50101),(50102,'官府礼包II',1,50102),(50103,'官府礼包III',1,50103),(50104,'官府礼包IV',1,50104),
(50105,'官府礼包V',1,50105),(50106,'官府礼包VI',1,50106),(50107,'官府礼包VII',1,50107),(50108,'官府礼包VIII',1,50108),
(50109,'官府礼包IX',1,50109),(50110,'官府礼包X',1,50110),
(1000001,'高级官府礼包I',1,1000001),(1000002,'高级官府礼包II',1,1000002),(1000003,'高级官府礼包III',1,1000003),
(1000004,'高级官府礼包IV',1,1000004),(1000005,'高级官府礼包V',1,1000005),(1000006,'高级官府礼包VI',1,1000006),
(1000007,'高级官府礼包VII',1,1000007),(1000008,'高级官府礼包VIII',1,1000008),(1000009,'高级官府礼包IX',1,1000009),(1000010,'高级官府礼包X',1,1000010);
