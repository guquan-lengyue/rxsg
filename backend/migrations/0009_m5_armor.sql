-- 0009_m5_armor.sql —— M5 装备批次 schema（幂等，可重复执行）。
-- 命名规范：cfg_*（配置）、user_/hero_ 前缀（持久态）、log_*（流水）。
-- 对应 legacy：sys_user_armor / sys_hero_armor / sys_hero_attribute / cfg_armor /
--   cfg_strong_probability / cfg_xilian / cfg_xilian_type / cfg_armor_level_attr /
--   cfg_armor_hole_rule / cfg_attribute / cfg_tie / cfg_tie_attribute /
--   cfg_tie_deify_attribute / cfg_armor_attribute / sys_user_tie_deify_attribute /
--   sys_armor_special / sys_armor_addon / log_armor_strong / log_armor_combine /
--   log_selled_armor / log_armor。
-- 【合成值声明】原库 cfg_* 装备配置 dump 已丢失（Explore 全仓确认无数据源），
--   以下 cfg_armor / cfg_strong_probability / cfg_xilian* / cfg_armor_level_attr /
--   cfg_armor_hole_rule / cfg_attribute / cfg_tie* / cfg_armor_attribute 均为合成种子，
--   仅保证流程与公式结构 1:1；数值非原版精确复刻（已在复刻对照表 M5 章节声明）。
--   代码中的公式常数（levelvalue 表、rateArr 熔炼概率、强化材料 gid、保底 maxtimes 表等）
--   均逐字来自 PHP 源码，为权威值。

-- ── heroes 补属性重算列（← sys_city_hero.*_add_on）──────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'speed_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `speed_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'force_max_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `force_max_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'heroes' AND COLUMN_NAME = 'energy_max_add_on') = 0,
  'ALTER TABLE `heroes` ADD COLUMN `energy_max_add_on` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cfg_goods 补 attr 列（← cfg_goods.attr，镶嵌宝珠属性串 "type,val,..."）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_goods' AND COLUMN_NAME = 'attr') = 0,
  'ALTER TABLE `cfg_goods` ADD COLUMN `attr` varchar(128) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cfg_armor（← cfg_armor；合成种子，覆盖 M5 测试链）────────────────────
-- part：1武器 2头盔 3铠甲 4战靴 5坐骑(12=legacy 坐骑位) …；spart=floor 校验用 part*10+pos。
-- attribute 串格式："N,type,val[,type,val...]"（首元素为属性对个数，HeroFunc.php:1461-1464 校验）。
CREATE TABLE IF NOT EXISTS `cfg_armor` (
  `id`          int NOT NULL,
  `name`        varchar(64) NOT NULL DEFAULT '',
  `part`        int NOT NULL DEFAULT 0,   -- 1武器 2头盔 3铠甲 4战靴 10宝物 12坐骑
  `type`        int NOT NULL DEFAULT 1,   -- 1灰 2白 3绿 4蓝 5紫 6橙 7红
  `hero_level`  int NOT NULL DEFAULT 1,   -- 穿戴等级门槛
  `value`       int NOT NULL DEFAULT 1,   -- 出售基准/打孔扣金基准
  `ori_hp_max`  int NOT NULL DEFAULT 100, -- 初始耐久上限
  `attribute`   varchar(255) NOT NULL DEFAULT '',
  `tieid`       int NOT NULL DEFAULT 0,   -- 套装 id（0=散件）
  `description` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_armor` (`id`,`name`,`part`,`type`,`hero_level`,`value`,`ori_hp_max`,`attribute`,`tieid`) VALUES
(10001,'铁剑',1,2,1,1,100,'1,3,10',0),
(10002,'青锋剑',1,3,10,2,120,'2,3,15,1,5',0),
(10003,'灰盔',2,1,1,1,90,'1,9,8',0),
(10004,'皮甲',3,2,1,1,110,'1,9,12',0),
(10005,'蓝纹甲',3,4,20,3,150,'2,9,20,11,3',0),
(10006,'战靴',4,2,1,1,100,'1,11,6',0),
(12003,'龙渊套·剑',1,5,40,5,200,'2,3,30,8,10',12003),
(12006,'龙渊套·甲',3,5,40,5,200,'2,9,30,8,5',12003),
(12010,'洛神玉佩',10,4,1,2,100,'1,1,10',0),
(12016,'末日之刃',1,6,60,8,300,'2,3,50,8,20',0),
(12018,'热血金枪',1,5,50,6,250,'1,11,10',0),
(15000,'君主冠',2,5,1,10,400,'2,1,40,2,40',15000),
(53016,'冰封马',12,4,1,2,180,'1,11,8',0),
(53040,'龙渊马',12,5,40,6,260,'2,11,12,1,10',0),
(53052,'白虎马',12,5,40,6,260,'2,11,12,3,10',0),
(12011,'赤龙马',12,4,1,2,180,'1,11,8',0),
(12015,'的卢马',12,4,1,2,180,'1,11,8',0);

-- ── user_armors（← sys_user_armor）───────────────────────────────────────
CREATE TABLE IF NOT EXISTS `user_armors` (
  `sid`            int NOT NULL AUTO_INCREMENT,
  `user_id`        int NOT NULL,
  `armorid`        int NOT NULL,
  `hp`             int NOT NULL DEFAULT 0,   -- 实际值 = hp_max×10 起步（legacy ×10 存储）
  `hp_max`         int NOT NULL DEFAULT 0,
  `ori_hp_max`     int NOT NULL DEFAULT 0,
  `hid`            int NOT NULL DEFAULT 0,   -- 0=背包
  `strong_level`   int NOT NULL DEFAULT 0,
  `strong_value`   int NOT NULL DEFAULT 0,
  `strong_times`   int NOT NULL DEFAULT 0,   -- 失败次数（保底/活动重算用）
  `combine_level`  int NOT NULL DEFAULT 0,   -- 熔炼等级 0-7
  `embed_holes`    varchar(64) NOT NULL DEFAULT '',
  `embed_pearls`   varchar(64) NOT NULL DEFAULT '',
  `best_quality`   varchar(64) NOT NULL DEFAULT '',
  `deified`        int NOT NULL DEFAULT 0,
  `active_special` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`sid`),
  KEY `idx_user_hid` (`user_id`, `hid`),
  KEY `idx_hid` (`hid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── hero_armors（← sys_hero_armor，uniq(hid,spart)）──────────────────────
CREATE TABLE IF NOT EXISTS `hero_armors` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `hid`     int NOT NULL,
  `spart`   int NOT NULL,
  `sid`     int NOT NULL,
  `armorid` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_hid_spart` (`hid`, `spart`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── hero_attributes（← sys_hero_attribute，uniq(hid,attid)）──────────────
CREATE TABLE IF NOT EXISTS `hero_attributes` (
  `id`     int NOT NULL AUTO_INCREMENT,
  `attid`  int NOT NULL,
  `hid`    int NOT NULL,
  `value`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_hid_attid` (`hid`, `attid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_attribute（← cfg_attribute：attid→type 映射；合成种子）────────────
CREATE TABLE IF NOT EXISTS `cfg_attribute` (
  `attid` int NOT NULL,
  `name`  varchar(32) NOT NULL DEFAULT '',
  `type`  int NOT NULL DEFAULT 0, -- 1统 2政 3武 4智 5力 6精 8攻 9防 11速
  PRIMARY KEY (`attid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_attribute` (`attid`,`name`,`type`) VALUES
(1,'统率',1),(2,'内政',2),(3,'勇武',3),(4,'智谋',4),(5,'体力',5),(6,'精力',6),
(8,'攻击',8),(9,'防御',9),(11,'速度',11),
(10001,'统率百分比',101),(10002,'内政百分比',102),(10003,'勇武百分比',103),
(10004,'智谋百分比',104),(10005,'体力百分比',105),(10006,'精力百分比',106),
(10008,'攻击百分比',108),(10009,'防御百分比',109);

-- ── cfg_strong_probability（← cfg_strong_probability；合成种子）───────────
-- 字段权威语义（EquipmentFunc.php:312-410）：
--   suc_value 基础成功率（百分比口径：rand(1,10000)<=suc_value*100，EquipmentFunc.php:327-329；
--     拆解宝珠返还 floor(100/suc_value) 亦要求百分比量纲）
--   strong_value 该级强化属性值（regenerate 实际用硬编码 levelvalue 表覆盖，此列仅入库对齐）
--   zero_value/degrade_value/intact_value 失败三分支概率（pValue=rand(1,100) 落区间）
--   xilian_rate 出极品属性概率（getBestQuality: mt_rand(1,100)<=rate）
CREATE TABLE IF NOT EXISTS `cfg_strong_probability` (
  `level`          int NOT NULL,
  `suc_value`      int NOT NULL DEFAULT 0,
  `strong_value`   int NOT NULL DEFAULT 0,
  `zero_value`     int NOT NULL DEFAULT 0,
  `degrade_value`  int NOT NULL DEFAULT 0,
  `intact_value`   int NOT NULL DEFAULT 0,
  `xilian_rate`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_strong_probability` (`level`,`suc_value`,`strong_value`,`zero_value`,`degrade_value`,`intact_value`,`xilian_rate`) VALUES
(1,100,2,34,33,33,0),
(2,90,2,34,33,33,0),
(3,80,2,34,33,33,0),
(4,70,2,34,33,33,0),
(5,60,2,34,33,33,0),
(6,50,2,34,33,33,0),
(7,40,3,34,33,33,5),
(8,30,3,34,33,33,5),
(9,25,3,34,33,33,5),
(10,20,6,34,33,33,10),
(11,15,3,34,33,33,10),
(12,12,3,34,33,33,10),
(13,10,5,34,33,33,15),
(14,8,5,34,33,33,15),
(15,5,7,34,33,33,15);

-- ── cfg_xilian / cfg_xilian_type（← 同名；极品属性，合成种子）─────────────
-- property 串："属性索引,概率,属性索引,概率,..."（getProperty:492 加权随机，i 从 1 步长 2；
-- 索引对应 cfg_property：0生命 1攻击 2防御 3射程 4速度 5负重）。
CREATE TABLE IF NOT EXISTS `cfg_xilian` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `name`     varchar(64) NOT NULL DEFAULT '',
  `property` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_xilian` (`id`,`name`,`property`) VALUES
(1,'锋锐','1,50,4,50'),
(2,'坚固','2,60,0,40'),
(3,'迅捷','4,70,1,30'),
(4,'厚重','0,50,2,50'),
(5,'神佑','1,40,2,30,0,30'),
(6,'无双','4,40,1,40,3,20');

CREATE TABLE IF NOT EXISTS `cfg_xilian_type` (
  `type`   int NOT NULL,   -- cfg_armor.type（装备颜色）
  `minAdd` int NOT NULL DEFAULT 1,
  `maxAdd` int NOT NULL DEFAULT 10,
  PRIMARY KEY (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_xilian_type` (`type`,`minAdd`,`maxAdd`) VALUES
(1,1,5),(2,1,5),(3,2,8),(4,3,10),(5,5,15),(6,8,20),(7,10,30);

-- ── cfg_armor_level_attr（← 同名，熔炼等级加成；合成种子）─────────────────
-- attr 串格式同装备属性："N,type,val,..."。
CREATE TABLE IF NOT EXISTS `cfg_armor_level_attr` (
  `level` int NOT NULL,
  `attr`  varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_armor_level_attr` (`level`,`attr`) VALUES
(1,'1,1,2'),(2,'1,1,4'),(3,'2,1,6,3,4'),(4,'2,1,8,3,6'),
(5,'2,1,10,3,8'),(6,'3,1,12,3,10,8,5'),(7,'3,1,15,3,12,8,8');

-- ── cfg_armor_hole_rule（← 同名，打孔规则；合成种子）──────────────────────
-- rule 元素：0=初级打孔器(1) -1=高级(2) -2=特级(3) N/A=不开(4) 数字=聚魂珠等级(5+?)。
-- parseHoleRule（EquipmentFunc.php:670-690）逐字符映射。
CREATE TABLE IF NOT EXISTS `cfg_armor_hole_rule` (
  `type` int NOT NULL,
  `rule` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_armor_hole_rule` (`type`,`rule`) VALUES
(1,'0,0,N/A,N/A,N/A'),(2,'0,0,-1,N/A,N/A'),(3,'0,0,-1,-1,N/A'),
(4,'0,-1,-1,-1,N/A'),(5,'-1,-1,-1,-1,4'),(6,'-1,-1,-1,-2,4'),(7,'-2,-2,-2,-2,4');

-- ── cfg_tie / cfg_tie_attribute / cfg_tie_deify_attribute（套装；合成种子）──
CREATE TABLE IF NOT EXISTS `cfg_tie` (
  `tieid` int NOT NULL,
  `name`  varchar(64) NOT NULL DEFAULT '',
  `count` int NOT NULL DEFAULT 2, -- 集齐件数
  PRIMARY KEY (`tieid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_tie` (`tieid`,`name`,`count`) VALUES
(12003,'龙渊套',2),(15000,'君主套',1);

CREATE TABLE IF NOT EXISTS `cfg_tie_attribute` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `tieid`   int NOT NULL,
  `attid`   int NOT NULL,
  `value`   int NOT NULL DEFAULT 0,
  `precond` int NOT NULL DEFAULT 1, -- 穿戴件数门槛
  PRIMARY KEY (`id`),
  KEY `idx_tie` (`tieid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_tie_attribute` (`id`,`tieid`,`attid`,`value`,`precond`) VALUES
(1,12003,1,10,1),(2,12003,3,20,2),(3,15000,1,30,1);

CREATE TABLE IF NOT EXISTS `cfg_tie_deify_attribute` (
  `id`    int NOT NULL AUTO_INCREMENT,
  `tieid` int NOT NULL,
  `attid` int NOT NULL,
  `low`   int NOT NULL DEFAULT 0,
  `high`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_tie` (`tieid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_tie_deify_attribute` (`id`,`tieid`,`attid`,`low`,`high`) VALUES
(1,12003,3,10,30),(2,12003,8,5,20),(3,15000,1,10,40);

-- ── cfg_armor_attribute（← 同名，装备新属性；合成种子，空表即可跑通链路）──
CREATE TABLE IF NOT EXISTS `cfg_armor_attribute` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `armorid` int NOT NULL,
  `attid`   int NOT NULL,
  `value`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_armor` (`armorid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_tie_deify_attribute（← sys_user_tie_deify_attribute，神化落库）────
CREATE TABLE IF NOT EXISTS `user_tie_deify_attribute` (
  `id`     int NOT NULL AUTO_INCREMENT,
  `sid`    int NOT NULL,
  `attid`  int NOT NULL,
  `value`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── armor_special / armor_addon（← sys_armor_special / sys_armor_addon，
--    末日之刃特效；特效装备无权威数值 → 空表，链路按"无特效"降级）────────
CREATE TABLE IF NOT EXISTS `armor_special` (
  `id`   int NOT NULL AUTO_INCREMENT,
  `sid`  int NOT NULL,
  `type` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `armor_addon` (
  `id`    int NOT NULL AUTO_INCREMENT,
  `sid`   int NOT NULL,
  `attid` int NOT NULL,
  `value` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── 流水表 ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `log_armor_strong` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `user_id`    int NOT NULL,
  `sid`        int NOT NULL,
  `armorid`    int NOT NULL,
  `is_zuoji`   int NOT NULL DEFAULT 0,
  `startlevel` int NOT NULL DEFAULT 0,
  `endlevel`   int NOT NULL DEFAULT 0,
  `usegoods`   varchar(64) NOT NULL DEFAULT '',
  `success`    int NOT NULL DEFAULT 0,
  `time`       bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_sid_level` (`sid`, `user_id`, `startlevel`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `log_armor_combine` (
  `id`             int NOT NULL AUTO_INCREMENT,
  `sid`            int NOT NULL,
  `user_id`        int NOT NULL,
  `armorid`        int NOT NULL,
  `hp`             int NOT NULL DEFAULT 0,
  `hp_max`         int NOT NULL DEFAULT 0,
  `hid`            int NOT NULL DEFAULT 0,
  `strong_level`   int NOT NULL DEFAULT 0,
  `strong_value`   int NOT NULL DEFAULT 0,
  `embed_pearls`   varchar(64) NOT NULL DEFAULT '',
  `embed_holes`    varchar(64) NOT NULL DEFAULT '',
  `deified`        int NOT NULL DEFAULT 0,
  `active_special` int NOT NULL DEFAULT 0,
  `strong_times`   int NOT NULL DEFAULT 0,
  `combine_level`  int NOT NULL DEFAULT 0,
  `usegoods`       int NOT NULL DEFAULT 0,
  `success`        int NOT NULL DEFAULT 0,
  `mainsid`        int NOT NULL DEFAULT 0,
  `time`           bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_sid` (`sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `log_selled_armor` (
  `id`           int NOT NULL AUTO_INCREMENT,
  `sid`          int NOT NULL,
  `user_id`      int NOT NULL,
  `armorid`      int NOT NULL,
  `hp`           int NOT NULL DEFAULT 0,
  `hp_max`       int NOT NULL DEFAULT 0,
  `hid`          int NOT NULL DEFAULT 0,
  `strong_level` int NOT NULL DEFAULT 0,
  `strong_value` int NOT NULL DEFAULT 0,
  `embed_pearls` varchar(64) NOT NULL DEFAULT '',
  `embed_holes`  varchar(64) NOT NULL DEFAULT '',
  `deified`      int NOT NULL DEFAULT 0,
  `time`         bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `log_armor` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `armorid` int NOT NULL,
  `count`   int NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_goods 补 M5 材料 gid（名称沿用 lang/源码注释；attr 宝珠由 migrate 程序化种子）──
REPLACE INTO `cfg_goods` (`gid`,`name`,`group_id`,`position`,`value`) VALUES
(202,'打孔器(测试)',1,202,0),(203,'天工符',1,203,0),(204,'乾坤宝珠',1,204,0),(205,'强化宝珠',4,205,0),
(206,'初级打孔器',1,206,0),(207,'高级打孔器',1,207,0),
(212,'伯乐符',1,212,0),(213,'师皇针',1,213,0),(214,'灵通甘草',1,214,0),
(11170,'高级强化宝珠',4,11170,0),(11171,'高级灵通甘草',1,11171,0),(11172,'宝珠保护符',1,11172,0),
(10776,'熔炼石I',1,10776,0),(10777,'熔炼石II',1,10777,0),(10778,'熔炼保护符',1,10778,0),
(10400,'装备碎片',1,10400,0),(10522,'神兵鉴符',1,10522,0),
(12156,'五彩化石粉',1,12156,0),(12157,'升阶保护符',1,12157,0),(12158,'升阶符',1,12158,0),
(12159,'升阶材料I',1,12159,0),(12160,'升阶材料II',1,12160,0),(12161,'升阶材料III',1,12161,0),
(10667,'化石粉II',1,10667,0),(10615,'强化礼盒',1,10615,0),(10616,'强化礼盒',1,10616,0),
(10617,'强化礼盒',1,10617,0),(10618,'强化礼盒',1,10618,0),
(112,'坐骑礼盒',1,112,0),(10307,'彩蛋',1,10307,0),
(12178,'战神马装I',1,12178,0),(12179,'战神马装II',1,12179,0),(12180,'战神马装III',1,12180,0),
(12181,'战神马装IV',1,12181,0),(12182,'战神马装V',1,12182,0),
(160040,'青铜虎符I',1,160040,0),(160041,'青铜虎符II',1,160041,0),(160042,'青铜虎符III',1,160042,0),
(19057,'马鞭',1,19057,0),(19058,'汗血马鞭',1,19058,0);

-- ── cfg_goods 镶嵌宝珠/坐骑装备种子（attr 列：镶嵌加成 "type,val,..."；合成种子）──
-- gid 区间语义（EquipmentFunc.php:742/942）：300-379=1-10 级宝珠（gid%10+1=等级）、
--   17500-17539=11-15 级（gid%5+11）、10830-10839=聚魂珠（只能 pos4）、400-582/1400-1500=坐骑装备。
REPLACE INTO `cfg_goods` (`gid`,`name`,`group_id`,`position`,`value`,`attr`) VALUES
(300,'1级攻击宝珠',4,300,0,'8,3'),
(301,'2级攻击宝珠',4,301,0,'8,5'),
(302,'3级防御宝珠',4,302,0,'9,8'),
(309,'10级宝珠',4,309,0,'3,20'),
(10831,'2级聚魂珠',4,10831,0,'1,10'),
(17500,'11级攻击宝珠',4,17500,0,'8,30'),
(401,'1级马装',4,401,0,'11,2'),
(409,'冰封马专属装',4,409,0,'11,4');

-- ── things / log_things（← sys_things / log_things；utils.php addThings:1030，拆解碎片入账）──
CREATE TABLE IF NOT EXISTS `things` (
  `user_id` int NOT NULL,
  `tid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`user_id`, `tid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `log_things` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `tid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
