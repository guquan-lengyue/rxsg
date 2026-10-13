-- 0023_m13f_xilian_skillbook.sql
-- R11-4：铁匠铺「极品属性洗练」与「技能书」页签所需表 / 配置。
--
-- 消费方 legacy：
--   洗练   server/game/EquipmentFunc.php loadXilianGoods(1821) / doXilian(1841)
--          —— 依赖既有 cfg_xilian / cfg_xilian_type（0009 已建）。
--          【数据缺口修正】0017 头注称 cfg_goods 12079-12081「已齐」，但全量迁移实际未写入这三行
--          （0009/0017 均无对应 REPLACE），导致 loadXilianGoods(1830) 取不到 cfg_goods 行 → 本文件补齐。
--          名称逐字取自 EquipmentFunc.php:1823 注释：12079 祝融之符 / 12080 极品属性清洗符 / 12081 极品属性洗练符；
--          group_id/position/value 沿用 0009 消耗品先例（1/gid/0），属合成种子（原 dump 丢失，0006 已声明）。
--   技能书 server/game/HeroSkillFunc.php loadUserValidSkillBook(5) / AddSkillBookShelf(21) /
--          loadUserSkillBook(63) / loadSkillBook_OpenTab(360)。
--
-- 表映射：sys_user_book → user_books（uid→user_id）、sys_user_bookShelfNum → user_book_shelf_num、
--   cfg_book → cfg_book（同名）、sys_goods → user_goods、cfg_goods → cfg_goods。
--
-- 【非原版精确还原 · 重要】原库 cfg_book（技能书配置）dump 已丢失，仓库内无任何权威数据源
--   （Explore 全仓：cfg_book 仅出现在 PHP 查询与后端降级注释里，无 .sql / 数据文件）。
--   故以下 cfg_book 为【合成数据】，规则如下（与原版查询自洽）：
--     · 结构依据 legacy 字段：id / skill / level / name / attr_type / attr / description；
--     · addSkillBook(GoodsFunc.php:3144) 的 gid 编码 gid=40000+id*100+level（level=gid%100）表明
--       level∈1..9、id 为技能书 id；setHeroBook(175) 以 `cfg_book.id` 与 sys_user_book.bid 比对，
--       同时多处 join 用 `bid=skill`（HeroSkillFunc.php:8/213/272）→ 判定 **id == skill**，主键为 (id, level)；
--       如此 loadUserSkillBook(66) 的 `bid=id` 与 loadSkillBook_OpenTab(364) 的 `bid=skill` 均可命中。
--     · 技能名取自客户端文案 tools/export/BloodWar/binaryData/4_utils.LocaleChinese.bin（逐字）：
--       运筹帷幄(4) / 骁勇善战(5) / 政绩通达(6) / 足智多谋(7) / 千里走单骑(14) / 鬼斧神工(15) / 城防加固(17)。
--     · attr_type / attr / description 无权威值 → attr_type 置 0、description 置空、
--       attr 取 level*10【合成值，非原版精确还原】。
--     · 技能 8-13（内政类，CityFunc/OfficeFunc 引用）因无权威名称，未纳入种子。
--
-- 幂等：CREATE TABLE IF NOT EXISTS + REPLACE INTO。

SET NAMES utf8mb4;

-- ── cfg_goods 补齐洗练三符（gid 12079/12080/12081；名称取自 EquipmentFunc.php:1823）──
REPLACE INTO `cfg_goods` (`gid`,`name`,`group_id`,`position`,`value`) VALUES
(12079,'祝融之符',1,12079,0),
(12080,'极品属性清洗符',1,12080,0),
(12081,'极品属性洗练符',1,12081,0);

-- ── cfg_book（← cfg_book；主键 (id, level)，id == skill）────────────────────
CREATE TABLE IF NOT EXISTS `cfg_book` (
  `id`          int NOT NULL,          -- 技能书 id（原版 == skill）
  `skill`       int NOT NULL,          -- 技能 id
  `level`       int NOT NULL,          -- 技能书等级 1-9（mergeBook:283 限制 <10）
  `name`        varchar(64)  NOT NULL DEFAULT '',
  `attr_type`   int          NOT NULL DEFAULT 0,   -- 无权威值 → 0（合成）
  `attr`        varchar(64)  NOT NULL DEFAULT '',  -- 属性串（合成：level*10）
  `description` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`,`level`),
  KEY `idx_skill_level` (`skill`,`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_book` (`id`,`skill`,`level`,`name`,`attr_type`,`attr`,`description`) VALUES
(4,4,1,'运筹帷幄',0,'10',''),(4,4,2,'运筹帷幄',0,'20',''),(4,4,3,'运筹帷幄',0,'30',''),
(4,4,4,'运筹帷幄',0,'40',''),(4,4,5,'运筹帷幄',0,'50',''),(4,4,6,'运筹帷幄',0,'60',''),
(4,4,7,'运筹帷幄',0,'70',''),(4,4,8,'运筹帷幄',0,'80',''),(4,4,9,'运筹帷幄',0,'90',''),
(5,5,1,'骁勇善战',0,'10',''),(5,5,2,'骁勇善战',0,'20',''),(5,5,3,'骁勇善战',0,'30',''),
(5,5,4,'骁勇善战',0,'40',''),(5,5,5,'骁勇善战',0,'50',''),(5,5,6,'骁勇善战',0,'60',''),
(5,5,7,'骁勇善战',0,'70',''),(5,5,8,'骁勇善战',0,'80',''),(5,5,9,'骁勇善战',0,'90',''),
(6,6,1,'政绩通达',0,'10',''),(6,6,2,'政绩通达',0,'20',''),(6,6,3,'政绩通达',0,'30',''),
(6,6,4,'政绩通达',0,'40',''),(6,6,5,'政绩通达',0,'50',''),(6,6,6,'政绩通达',0,'60',''),
(6,6,7,'政绩通达',0,'70',''),(6,6,8,'政绩通达',0,'80',''),(6,6,9,'政绩通达',0,'90',''),
(7,7,1,'足智多谋',0,'10',''),(7,7,2,'足智多谋',0,'20',''),(7,7,3,'足智多谋',0,'30',''),
(7,7,4,'足智多谋',0,'40',''),(7,7,5,'足智多谋',0,'50',''),(7,7,6,'足智多谋',0,'60',''),
(7,7,7,'足智多谋',0,'70',''),(7,7,8,'足智多谋',0,'80',''),(7,7,9,'足智多谋',0,'90',''),
(14,14,1,'千里走单骑',0,'10',''),(14,14,2,'千里走单骑',0,'20',''),(14,14,3,'千里走单骑',0,'30',''),
(14,14,4,'千里走单骑',0,'40',''),(14,14,5,'千里走单骑',0,'50',''),(14,14,6,'千里走单骑',0,'60',''),
(14,14,7,'千里走单骑',0,'70',''),(14,14,8,'千里走单骑',0,'80',''),(14,14,9,'千里走单骑',0,'90',''),
(15,15,1,'鬼斧神工',0,'10',''),(15,15,2,'鬼斧神工',0,'20',''),(15,15,3,'鬼斧神工',0,'30',''),
(15,15,4,'鬼斧神工',0,'40',''),(15,15,5,'鬼斧神工',0,'50',''),(15,15,6,'鬼斧神工',0,'60',''),
(15,15,7,'鬼斧神工',0,'70',''),(15,15,8,'鬼斧神工',0,'80',''),(15,15,9,'鬼斧神工',0,'90',''),
(17,17,1,'城防加固',0,'10',''),(17,17,2,'城防加固',0,'20',''),(17,17,3,'城防加固',0,'30',''),
(17,17,4,'城防加固',0,'40',''),(17,17,5,'城防加固',0,'50',''),(17,17,6,'城防加固',0,'60',''),
(17,17,7,'城防加固',0,'70',''),(17,17,8,'城防加固',0,'80',''),(17,17,9,'城防加固',0,'90','');

-- ── user_books（← sys_user_book；bid=cfg_book.id、level、hid=0 表示未穿戴）──
CREATE TABLE IF NOT EXISTS `user_books` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `bid`     int NOT NULL DEFAULT 0,
  `level`   int NOT NULL DEFAULT 1,
  `hid`     int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user_hid` (`user_id`,`hid`),
  KEY `idx_bid` (`bid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_book_shelf_num（← sys_user_bookShelfNum；默认 100，上限 500）──────
CREATE TABLE IF NOT EXISTS `user_book_shelf_num` (
  `user_id` int NOT NULL,
  `num`     int NOT NULL DEFAULT 100,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
