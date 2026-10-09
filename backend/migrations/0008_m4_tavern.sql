-- 0008_m4_tavern.sql —— M4 酒馆招募（幂等，可重复执行）
-- 对应 legacy：sys_recruit_hero（招募池）、mem_hero_blood（武将体力）、
--   mem_city_schedule.last_reset_recruit（招贤榜刷新游标）、
--   mem_cfg_firstname/boyname/girlname（随机姓名表）。
-- bid 说明：legacy ID_BUILDING_HOTEL=10 与重写版「校场=10」冲突，客栈改用扩展槽位 12
--   （官署沿用 legacy 11，见 office.go）。

SET NAMES utf8mb4;

-- ── recruit_heroes（← sys_recruit_hero 招募池）──────────────────────────
CREATE TABLE IF NOT EXISTS `recruit_heroes` (
  `id`           int NOT NULL AUTO_INCREMENT,
  `name`         varchar(64) NOT NULL DEFAULT '',
  `sex`          tinyint NOT NULL DEFAULT 0,
  `face`         int NOT NULL DEFAULT 0,
  `city_id`      int NOT NULL DEFAULT 0,
  `level`        int NOT NULL DEFAULT 0,
  `exp`          bigint NOT NULL DEFAULT 0,
  `affairs_base` int NOT NULL DEFAULT 0,
  `bravery_base` int NOT NULL DEFAULT 0,
  `wisdom_base`  int NOT NULL DEFAULT 0,
  `command_base` int NOT NULL DEFAULT 0,
  `affairs_add`  int NOT NULL DEFAULT 0,
  `bravery_add`  int NOT NULL DEFAULT 0,
  `wisdom_add`   int NOT NULL DEFAULT 0,
  `command_add`  int NOT NULL DEFAULT 0,
  `loyalty`      int NOT NULL DEFAULT 0,
  `gold_need`    bigint NOT NULL DEFAULT 0,
  `gen_time`     bigint NOT NULL DEFAULT 0,
  `hero_type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_city` (`city_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── hero_blood（← mem_hero_blood：招募时初始化 force/energy 上限）───────
CREATE TABLE IF NOT EXISTS `hero_blood` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `hero_id`    int NOT NULL,
  `force`      int NOT NULL DEFAULT 0,
  `force_max`  int NOT NULL DEFAULT 0,
  `energy`     int NOT NULL DEFAULT 0,
  `energy_max` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_hero` (`hero_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_schedule 补 last_reset_recruit（← mem_city_schedule）───────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_schedule' AND COLUMN_NAME = 'last_reset_recruit') = 0,
  'ALTER TABLE `city_schedule` ADD COLUMN `last_reset_recruit` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ── cfg_names（← mem_cfg_firstname/boyname/girlname；原 data/name_*.txt 已丢失，
--    以下为【合成值】：常见姓氏与男性/女性名用字，仅保证随机姓名可生成）────
-- 主键 (kind,id)：三种 kind 各自独立编号（对齐 legacy 三张独立表）。
-- DROP 重建：历史版本误用单列 id 主键导致 REPLACE 互相覆盖，此处保证 schema 收敛。
DROP TABLE IF EXISTS `cfg_names`;
CREATE TABLE `cfg_names` (
  `id`   int NOT NULL,
  `kind` int NOT NULL, -- 1=姓 2=男名 3=女名
  `name` varchar(32) NOT NULL,
  PRIMARY KEY (`kind`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_names` (`id`, `kind`, `name`) VALUES
(1,1,'赵'),(2,1,'钱'),(3,1,'孙'),(4,1,'李'),(5,1,'周'),(6,1,'吴'),(7,1,'郑'),(8,1,'王'),
(9,1,'冯'),(10,1,'陈'),(11,1,'褚'),(12,1,'卫'),(13,1,'蒋'),(14,1,'沈'),(15,1,'韩'),(16,1,'杨'),
(17,1,'朱'),(18,1,'秦'),(19,1,'尤'),(20,1,'许'),(21,1,'何'),(22,1,'吕'),(23,1,'施'),(24,1,'张'),
(25,1,'孔'),(26,1,'曹'),(27,1,'严'),(28,1,'华'),(29,1,'金'),(30,1,'魏'),(31,1,'陶'),(32,1,'姜'),
(1,2,'飞'),(2,2,'勇'),(3,2,'刚'),(4,2,'强'),(5,2,'军'),(6,2,'超'),(7,2,'杰'),(8,2,'涛'),
(9,2,'明'),(10,2,'辉'),(11,2,'力'),(12,2,'成'),(13,2,'峰'),(14,2,'磊'),(15,2,'彬'),(16,2,'波'),
(17,2,'宁'),(18,2,'龙'),(19,2,'腾'),(20,2,'威'),(21,2,'翔'),(22,2,'凯'),(23,2,'健'),(24,2,'俊'),
(25,2,'亮'),(26,2,'鹏'),(27,2,'斌'),(28,2,'康'),(29,2,'皓'),(30,2,'宸'),
(1,3,'芳'),(2,3,'娜'),(3,3,'敏'),(4,3,'静'),(5,3,'丽'),(6,3,'秀'),(7,3,'娟'),(8,3,'英'),
(9,3,'华'),(10,3,'慧'),(11,3,'巧'),(12,3,'美'),(13,3,'玉'),(14,3,'萍'),(15,3,'红'),(16,3,'燕'),
(17,3,'玲'),(18,3,'霞'),(19,3,'兰'),(20,3,'雪'),(21,3,'琳'),(22,3,'晶'),(23,3,'倩'),(24,3,'瑶'),
(25,3,'洁'),(26,3,'薇'),(27,3,'璐'),(28,3,'岚'),(29,3,'婉'),(30,3,'婷');

-- ── cfg_buildings 扩展槽位：11 官署（legacy 同值）、12 客栈（legacy 10 与校场冲突）──
REPLACE INTO `cfg_buildings` (`bid`, `name`, `table_name`, `inner`) VALUES
(11, '官署', 'office', 1),
(12, '客栈', 'hotel', 1);
