-- 0013_m9_activity.sql
-- M9 单机活动：抽奖（server/game/LotteryFunc.php）+ 单机 PK 征战（server/game/PKFunc.php 的 campaign 部分）。
-- 口径来源（文件:行号）：
--   log_lottery / mem_lottery_goods ← LotteryFunc.php:43/47/128/203/293/502/629（按 SQL 反推列名）
--   mem_state(state=150)           ← LotteryFunc.php:11/91 抽奖总开关（0=开放）
--   cfg_goods.level / cfg_armor.level ← LotteryFunc.php:410/415/420/437 的 `level` 抽奖档位（1~8）
--   cfg_pk_battle                  ← PKFunc.php:354/460/491（id,hero_level,user_level,battlename,description）
--   cfg_pk_level                   ← PKFunc.php:268/330/400（battleid,levelid,type,area,hid）
--   cfg_pk_hero                    ← PKFunc.php:401/947（hid,sex,energy(+add),command/affair/bravery/wisdom/speed 的 base+add,face,flag,name）
--   cfg_pk_reward                  ← PKFunc.php:270/280（battleid,type,rewardtype,rewardid,count）
--   cfg_pk_first                   ← PKFunc.php:216-223/430-441/502（battleid,type,rankid,uid,passtime,reward,time）
--   sys_pk_user                    ← PKFunc.php:258/454/538（uid,normal,special）
--
-- ⚠ 合成数据声明：老库 dump 已丢失，本文件末尾所有 cfg_pk_* / cfg_pk_hero / cfg_pk_level /
--   cfg_pk_reward / cfg_pk_first / 抽奖物品池 数据均为【合成演示数据】，仅用于跑通 1:1 复刻逻辑与集成测试，
--   数值/文案非原版精确复刻（已在汇报中标注）。代码中的常数（概率数组、RATE=50、价格 5、奖励 gid 公式
--   18000+battleId*10+flag 等）均逐字来自 PHP 源码，为权威值。
--
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE；种子 REPLACE INTO / ON DUPLICATE。
--
-- 裁剪（不建表，源码注释逐处声明）：
--   沙场（ShaChangFunc.php / ShaChang.php / CROSS_ShaChang_* 外部跨服网关）——纯跨服，无外部服务可 1:1；
--   竞技场（mem_assemble* / sys_user_assemble / cfg_assemble_* / mem_assemble_win_record / log_assemble_reward）——
--     玩家间排行 PvP，属用户明确排除范围；
--   sys_user_level（君主修为）——未在已完成模块建表，checkUserLevel(flag=1) 按"无表→0"降级；
--   sys_inform / sendSysInform 公告广播、sys_goods(→user_goods)、sys_city_hero(→heroes)、mem_hero_blood(→hero_blood)。

SET NAMES utf8mb4;

-- ── mem_state（← legacy mem_state；抽奖开关 state=150，新库缺失时兜底建表）──
CREATE TABLE IF NOT EXISTS `mem_state` (
  `state` int NOT NULL,
  `value` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO `mem_state` (`state`,`value`) VALUES (150,0)
  ON DUPLICATE KEY UPDATE `value`=VALUES(`value`);

-- ── log_lottery（← log_lottery：每次抽奖流水；getTodayCount/specialProp 依赖）──
CREATE TABLE IF NOT EXISTS `log_lottery` (
  `id`   int NOT NULL AUTO_INCREMENT,
  `uid`  int NOT NULL,
  `time` datetime NOT NULL,
  `gid`  int NOT NULL DEFAULT 0,   -- 中奖 id（type=0 为 cfg_goods.gid，type=1 为 cfg_armor.id）
  `type` int NOT NULL DEFAULT 0,   -- 0 道具 1 装备
  PRIMARY KEY (`id`),
  KEY `idx_uid_time` (`uid`, `time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── mem_lottery_goods（← mem_lottery_goods：玩家当前盘面；一个 uid 一行）──
CREATE TABLE IF NOT EXISTS `mem_lottery_goods` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `uid`           int NOT NULL,
  `records`       text,                                   -- "type,id,count,type,id,count..." 8 格内容
  `time`          date DEFAULT NULL,                      -- legacy current_date()
  `win`           varchar(64) NOT NULL DEFAULT '-1,0,0',  -- "win_type,win_id,win_count"
  `got`           int NOT NULL DEFAULT 0,                 -- 1=已领奖
  `restart_count` int NOT NULL DEFAULT 0,                 -- 重开次数（Round restart 清 0）
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_goods 补 level（← cfg_goods.level：抽奖档位 1~8；legacy `group` 对应新库 group_id）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_goods' AND COLUMN_NAME = 'level') = 0,
  'ALTER TABLE `cfg_goods` ADD COLUMN `level` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── cfg_armor 补 level（← cfg_armor.level：抽奖档位 1~8）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_armor' AND COLUMN_NAME = 'level') = 0,
  'ALTER TABLE `cfg_armor` ADD COLUMN `level` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ══════════════════════════════════════════════════════════════════════════
-- PK 征战表
-- ══════════════════════════════════════════════════════════════════════════

-- ── cfg_pk_battle（← cfg_pk_battle）──────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_pk_battle` (
  `id`          int NOT NULL,
  `hero_level`  int NOT NULL DEFAULT 0,   -- flag=0（普通）门槛：君主将等级
  `user_level`  int NOT NULL DEFAULT 0,   -- flag=1（精英）门槛：君主修为
  `battlename`  varchar(64) NOT NULL DEFAULT '',
  `description` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_pk_level（← cfg_pk_level：每战役每关每站位(area 1~3) 一个 NPC 将 hid）──
CREATE TABLE IF NOT EXISTS `cfg_pk_level` (
  `battleid` int NOT NULL,
  `levelid`  int NOT NULL,
  `type`     int NOT NULL DEFAULT 0,   -- 0 普通 1 精英
  `area`     int NOT NULL DEFAULT 0,   -- 1~3 站位
  `hid`      int NOT NULL DEFAULT 0,   -- 指向 cfg_pk_hero.hid
  PRIMARY KEY (`battleid`, `levelid`, `type`, `area`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_pk_hero（← cfg_pk_hero：NPC 将属性 base+add）─────────────────────
CREATE TABLE IF NOT EXISTS `cfg_pk_hero` (
  `hid`          int NOT NULL,
  `sex`          tinyint NOT NULL DEFAULT 1,
  `energy`       int NOT NULL DEFAULT 0,
  `energy_add`   int NOT NULL DEFAULT 0,
  `command_base` int NOT NULL DEFAULT 0,
  `command_add`  int NOT NULL DEFAULT 0,
  `affair_base`  int NOT NULL DEFAULT 0,
  `affair_add`   int NOT NULL DEFAULT 0,
  `bravery_base` int NOT NULL DEFAULT 0,
  `bravery_add`  int NOT NULL DEFAULT 0,
  `wisdom_base`  int NOT NULL DEFAULT 0,
  `wisdom_add`   int NOT NULL DEFAULT 0,
  `speed_base`   int NOT NULL DEFAULT 0,
  `speed_add`    int NOT NULL DEFAULT 0,
  `face`         int NOT NULL DEFAULT 0,
  `flag`         int NOT NULL DEFAULT 0,
  `name`         varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`hid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_pk_reward（← cfg_pk_reward：战役通关奖励）────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_pk_reward` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `battleid`   int NOT NULL DEFAULT 0,
  `type`       int NOT NULL DEFAULT 0,   -- 0 普通 1 精英（对应 battleFlag）
  `rewardtype` int NOT NULL DEFAULT 0,   -- 0 道具(cfg_goods) 1 装备(cfg_armor)
  `rewardid`   int NOT NULL DEFAULT 0,
  `count`      int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_battle` (`battleid`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_pk_first（← cfg_pk_first：首通前三名及榜首奖励）──────────────────
CREATE TABLE IF NOT EXISTS `cfg_pk_first` (
  `battleid` int NOT NULL,
  `type`     int NOT NULL DEFAULT 0,
  `rankid`   int NOT NULL DEFAULT 0,   -- 1~3
  `uid`      int NOT NULL DEFAULT 0,   -- 0=占用位空缺
  `passtime` bigint NOT NULL DEFAULT 0,
  `reward`   varchar(64) NOT NULL DEFAULT '',   -- "count,type,gid,cnt"（parseAndAddReward 格式）
  `time`     bigint NOT NULL DEFAULT 0,         -- >1 表示已领取榜首奖励
  PRIMARY KEY (`battleid`, `type`, `rankid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── sys_pk_user（← sys_pk_user：通关进度 normal/special = battleId*100+levelId）──
CREATE TABLE IF NOT EXISTS `sys_pk_user` (
  `uid`     int NOT NULL,
  `normal`  int NOT NULL DEFAULT 1,
  `special` int NOT NULL DEFAULT 1,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ══════════════════════════════════════════════════════════════════════════
-- 以下全部为【合成演示数据】
-- ══════════════════════════════════════════════════════════════════════════

-- 抽奖物品池：每档位(1~8) 1 个材料(group_id=4) + 1 个道具(group_id=1)，gid<10000。
-- getGoodsByType case0(group_id in(4,5)=材料) / case1(group_id in(0,1,2,3)=道具) 依赖 level。
REPLACE INTO `cfg_goods` (`gid`,`name`,`group_id`,`position`,`level`) VALUES
(9001,'抽奖材料一',4,9001,1),(9002,'抽奖材料二',4,9002,2),(9003,'抽奖材料三',4,9003,3),(9004,'抽奖材料四',4,9004,4),
(9005,'抽奖材料五',4,9005,5),(9006,'抽奖材料六',4,9006,6),(9007,'抽奖材料七',4,9007,7),(9008,'抽奖材料八',4,9008,8),
(9101,'抽奖道具一',1,9101,1),(9102,'抽奖道具二',1,9102,2),(9103,'抽奖道具三',1,9103,3),(9104,'抽奖道具四',1,9104,4),
(9105,'抽奖道具五',1,9105,5),(9106,'抽奖道具六',1,9106,6),(9107,'抽奖道具七',1,9107,7),(9108,'抽奖道具八',1,9108,8),
-- PK 通关奖励道具（getPkGid(1,0)=18010 / getPkGid(1,1)=18011）
(18010,'征战奖品·普通',1,18010,1),(18011,'征战奖品·精英',1,18011,1),
-- 军令 / 五铢钱（loadCampaignInitData / buyJunlingFunc 使用）
(19200,'军令',1,19200,1),(10195,'五铢钱',1,10195,1);

-- 抽奖装备池：档位 1~6（randType case2 装备；7/8 档强制为道具）
REPLACE INTO `cfg_armor` (`id`,`name`,`part`,`type`,`hero_level`,`value`,`ori_hp_max`,`attribute`,`tieid`,`level`) VALUES
(91101,'抽奖装备一',1,2,1,1,100,'1,3,10',0,1),
(91102,'抽奖装备二',1,2,1,1,100,'1,3,10',0,2),
(91103,'抽奖装备三',1,2,1,1,100,'1,3,10',0,3),
(91104,'抽奖装备四',1,2,1,1,100,'1,3,10',0,4),
(91105,'抽奖装备五',1,2,1,1,100,'1,3,10',0,5),
(91106,'抽奖装备六',1,2,1,1,100,'1,3,10',0,6);

-- PK 战役：1 个战役(id=1) + 2 关(levelid 1/2)，普通(type=0) 与精英(type=1) 各 2 关，每关 3 站位。
REPLACE INTO `cfg_pk_battle` (`id`,`hero_level`,`user_level`,`battlename`,`description`) VALUES
(1,1,0,'黄巾之乱','合成演示战役：剿灭黄巾贼。');

-- NPC 将：type0 用 hid 1001-1006，type1 用 hid 1011-1016（属性刻意偏弱，便于集成测试通关）。
REPLACE INTO `cfg_pk_hero`
(`hid`,`sex`,`energy`,`energy_add`,`command_base`,`command_add`,`affair_base`,`affair_add`,`bravery_base`,`bravery_add`,`wisdom_base`,`wisdom_add`,`speed_base`,`speed_add`,`face`,`flag`,`name`) VALUES
(1001,1,10,0,10,0,10,0,10,0,10,0,0,0,95,0,'黄巾贼首领'),
(1002,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'黄巾贼甲'),
(1003,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'黄巾贼乙'),
(1004,1,10,0,10,0,10,0,10,0,10,0,0,0,95,0,'黄巾贼先锋'),
(1005,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'黄巾贼丙'),
(1006,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'黄巾贼丁'),
(1011,1,10,0,10,0,10,0,10,0,10,0,0,0,95,0,'精英黄巾首领'),
(1012,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'精英黄巾甲'),
(1013,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'精英黄巾乙'),
(1014,1,10,0,10,0,10,0,10,0,10,0,0,0,95,0,'精英黄巾先锋'),
(1015,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'精英黄巾丙'),
(1016,1,10,0,10,0,10,0,10,0,10,0,0,0,90,0,'精英黄巾丁');

REPLACE INTO `cfg_pk_level` (`battleid`,`levelid`,`type`,`area`,`hid`) VALUES
(1,1,0,1,1001),(1,1,0,2,1002),(1,1,0,3,1003),
(1,2,0,1,1004),(1,2,0,2,1005),(1,2,0,3,1006),
(1,1,1,1,1011),(1,1,1,2,1012),(1,1,1,3,1013),
(1,2,1,1,1014),(1,2,1,2,1015),(1,2,1,3,1016);

REPLACE INTO `cfg_pk_reward` (`id`,`battleid`,`type`,`rewardtype`,`rewardid`,`count`) VALUES
(1,1,0,0,18010,2),
(2,1,0,1,91101,1),
(3,1,1,0,18011,2);

-- 首通前三名榜位（uid=0 空缺；reward 为合成榜首奖励）。
-- 每次 migrate 重置占用位，保证集成测试起始态确定（测试 Cleanup 亦会复位）。
REPLACE INTO `cfg_pk_first` (`battleid`,`type`,`rankid`,`uid`,`passtime`,`reward`,`time`) VALUES
(1,0,1,0,0,'1,0,18010,2',0),
(1,0,2,0,0,'1,0,18010,1',0),
(1,0,3,0,0,'1,0,18010,1',0),
(1,1,1,0,0,'1,0,18011,2',0),
(1,1,2,0,0,'1,0,18011,1',0),
(1,1,3,0,0,'1,0,18011,1',0);
