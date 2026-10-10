-- 0017_m13_office_barn.sql
-- R11-3：官署（OfficeFunc.php）+ 马厩（BarnFunc.php）1:1 复刻所需 schema 补齐（幂等）。
--
-- 官署（OfficeFunc.php）复用既有表：buildings(building_id=11)、heroes.state∈{1,7,8}、
--   cities.chief_hero_id/general_hero_id/counsellor_hero_id、users.nobility —— 无新增表列。
--
-- 马厩（BarnFunc.php）：
--   loadBarnGoods(5)  读 cfg_goods（gid 212/213/214/11171/12079-12081/12157-12161）——已齐。
--   loadZuojiArmor(19) 按 cfg_goods.zuoji_type 过滤坐骑装备槽位 —— **缺列**，本迁移补。
--   doUnlade(30)      只读 user_armors.embed_pearls + addGoods —— 已齐。
--   doUpgradeArmor(ArmorFunc.php:1190) 读 cfg_armor.tieid + user_armors + user_goods —— 已齐。
--
-- ⚠ 数据来源说明：原 cfg_goods dump 已丢失（0009 头注已声明合成种子）。zuoji_type 为
--   legacy 列语义【槽位号 = embed 位置 pos + 1】（见 BarnFunc.php:19-26 + isFitPos:64-78）。
--   本迁移仅对既有坐骑装备行按 isFitPos 反推的槽位号回填，不新增任何商品行（不造假）。
--
-- 幂等：补列 information_schema 判断 + PREPARE（兼容 multiStatements，与 0009/0016 同写法）。

SET NAMES utf8mb4;

-- ── cfg_goods 补列 zuoji_type（← legacy cfg_goods.zuoji_type，坐骑装备槽位号）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cfg_goods' AND COLUMN_NAME = 'zuoji_type') = 0,
  'ALTER TABLE `cfg_goods` ADD COLUMN `zuoji_type` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── 既有坐骑装备行回填槽位号（isFitPos 反推：pos→slot=pos+1）──────────────────
-- 12178-12182 战神马装：isFitPos 落 12178≤gid≤12182 → pos=gid-12178 → slot=gid-12178+1。
UPDATE `cfg_goods` SET `zuoji_type` = 1 WHERE `gid` = 12178;
UPDATE `cfg_goods` SET `zuoji_type` = 2 WHERE `gid` = 12179;
UPDATE `cfg_goods` SET `zuoji_type` = 3 WHERE `gid` = 12180;
UPDATE `cfg_goods` SET `zuoji_type` = 4 WHERE `gid` = 12181;
UPDATE `cfg_goods` SET `zuoji_type` = 5 WHERE `gid` = 12182;
-- 401/409（1 级马装/冰封马专属装）：isFitPos 落 else 分支 → pos=(gid-400)/20=0 → slot=1。
UPDATE `cfg_goods` SET `zuoji_type` = 1 WHERE `gid` IN (401, 409);
