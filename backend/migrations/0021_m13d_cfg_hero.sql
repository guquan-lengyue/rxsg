-- 0021_m13d_cfg_hero.sql
-- R5/M13d：补建 cfg_hero（商城「属性提示」所需武将静态配置）。
--
-- 背景：legacy server/game/ShopFunc.php 消费 cfg_hero：
--   getGoodsHeroAttr:25  select * from cfg_hero where id=$hid（空行 → buyGoods.no_tip「没有属性提示」）
--   addBattleHero:48     名将卡兑换（商品 group∈{6,7}），读取
--                        name/npcid/sex/face/level/affairs_base/bravery_base/wisdom_base/loyalty/command_base，
--                        再 insert into sys_city_hero。
--   BattleFunc.php:636   名将卡列表（cfg_shop group∈{6,7} and battlegoodstype=2）。
-- 重写版此前完全没有该表 → economy.GetGoodsHeroAttr 被硬编码恒抛「没有属性提示」，
--   并非 legacy 的「数据驱动：有行则返回」行为。
--
-- 【重要·无法确认项】原版 cfg_hero 的完整列集因权威 dump 丢失不可知
--   （本地无 cfg_hero 的 .sql / 数据文件，仅存在 cfg_hero_levels / cfg_hero_expr_types 等派生表）。
--   故此处列集为「代码实际消费字段的最小忠实超集」——严格按上述 PHP 实际读取的字段推导。
--   列命名与重写版 heroes 表（0003/0007：npc_id）保持一致，即 legacy 的 npcid → npc_id。
--
-- 【重要·无法确认项】种子武将为【合成演示数据，非原版数值】：
--   口径对齐 0004_seed_normalized.sql 的演示武将（id=1 赵云 / id=2 诸葛亮：sex/face/level/四维/loyalty）。
--   另补 2 名演示将（张飞/关羽）供「属性提示」演示，数值同样为合成、非原版精确还原。
--
-- 注意：本迁移【不新增】cfg_shops 的 group∈{6,7} 名将卡商品（属数据决策，需另行确认），
--   故名将卡商城入口在本版仍不存在；本表当前仅服务商城「属性提示」。
--
-- 幂等：CREATE TABLE IF NOT EXISTS + REPLACE INTO。

SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_hero` (
  `id`           int NOT NULL,
  `name`         varchar(64) NOT NULL DEFAULT '',
  `npc_id`       int NOT NULL DEFAULT 0,   -- 名将卡 NPC id（addBattleHero 写入 sys_city_hero.npcid / heroes.npc_id）
  `sex`          tinyint NOT NULL DEFAULT 0,
  `face`         int NOT NULL DEFAULT 0,   -- 头像 id
  `level`        int NOT NULL DEFAULT 0,
  `command_base` int NOT NULL DEFAULT 0,   -- 统帅
  `affairs_base` int NOT NULL DEFAULT 0,   -- 内政
  `bravery_base` int NOT NULL DEFAULT 0,   -- 勇武
  `wisdom_base`  int NOT NULL DEFAULT 0,   -- 智谋
  `loyalty`      int NOT NULL DEFAULT 0,   -- 忠诚
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

REPLACE INTO `cfg_hero`
  (`id`,`name`,`npc_id`,`sex`,`face`,`level`,`command_base`,`affairs_base`,`bravery_base`,`wisdom_base`,`loyalty`) VALUES
(1, '赵云',   0, 1, 95, 10, 80, 40, 90,  60, 100),
(2, '诸葛亮', 0, 1, 90, 10, 60, 95, 30, 100, 100),
(3, '张飞',   0, 1, 70, 10, 70, 30, 95,  40, 100),
(4, '关羽',   0, 1, 80, 10, 85, 50, 92,  55, 100);
