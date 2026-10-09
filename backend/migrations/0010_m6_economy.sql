-- 0010_m6_economy.sql
-- M6 经济系统（MarketFunc.php / StoreFunc.php / WorkShop.php / ShopFunc.php /
-- GoodsFunc.sellGoods / CityFunc.setCityProductRate / utils.SetCityBaseProduce+UpdateUsersCityResource /
-- ReportCron.HandleTrade+HandleAutoTrans）所需表与列。
-- 幂等：建表 CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE；种子 REPLACE INTO。
--
-- 数据来源声明（与 cfg_technic_levels 先例一致）：
--   * legacy sys_city_trade / sys_city_merchant / mem_city_autotrans / sys_user_workshop /
--     log_merchant / log_shop / log_shop_buy_cnt / sys_report / sys_alarm / sys_ticket(_content) /
--     log_gift / log_workshop_fresh / log_user_action / log_action_count 的 dump 已丢失，
--     表结构按代码中的 SQL 语句逐列还原。
--   * cfg_shop / cfg_goods_copper 的原始商品配置 dump 已丢失 → 合成种子（见文末），仅供接口贯通与测试。
--   * city_res_add 的 *_store 四列 legacy 默认值无权威来源 → 合成 25/25/25/25（和=100）。
--   * cfg_building_levels 的市场(新bid13←legacy13)/工匠作坊(新bid14←legacy15) 成本曲线无来源 → 合成。

SET NAMES utf8mb4;

-- ── users 补列 ───────────────────────────────────────────────────────────
-- gift（礼金，sys_user.gift：buyFromMerchant/sellToMerchant/buyGoods/buyWorkShopGood 扣减）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'gift') = 0,
  'ALTER TABLE `users` ADD COLUMN `gift` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- last_pay（buyGoods 记录最后支付方式）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'last_pay') = 0,
  'ALTER TABLE `users` ADD COLUMN `last_pay` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- union_id（sellToUser/buyFromUser/getUserBuyList 联盟限制；M8 社交不实现，恒 0）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'union_id') = 0,
  'ALTER TABLE `users` ADD COLUMN `union_id` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_res_add 补列（sys_city_res_add 的仓库存放比例）──────────────────
-- 注：legacy 表 dump 丢失，默认值合成 25（四项和=100，SetCityBaseProduce 仓库容量公式消费方）。
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_res_add' AND COLUMN_NAME = 'food_store') = 0,
  'ALTER TABLE `city_res_add` ADD COLUMN `food_store` int NOT NULL DEFAULT 25, ADD COLUMN `wood_store` int NOT NULL DEFAULT 25, ADD COLUMN `rock_store` int NOT NULL DEFAULT 25, ADD COLUMN `iron_store` int NOT NULL DEFAULT 25', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_resources 补列（mem_city_resource 的产出速率与在业劳力）──────────
-- food_add 等：UpdateUsersCityResource 每小时产量（每 225/255 秒 tick 累加一次）。
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'food_add') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `food_add` bigint NOT NULL DEFAULT 0, ADD COLUMN `wood_add` bigint NOT NULL DEFAULT 0, ADD COLUMN `rock_add` bigint NOT NULL DEFAULT 0, ADD COLUMN `iron_add` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- people_working（在业劳力，UpdateUsersCityResource:2364）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'people_working') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `people_working` double NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- changing（资源上限重算标记，SetCityBaseProduce/UpdateUsersCityResource 置 1）
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'city_resources' AND COLUMN_NAME = 'changing') = 0,
  'ALTER TABLE `city_resources` ADD COLUMN `changing` tinyint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── city_trades（← sys_city_trade；mem_city_trade 镜像仅为 cron 缓存，合并省略）──
CREATE TABLE IF NOT EXISTS `city_trades` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `cid`       int NOT NULL,
  `buycid`    int NOT NULL DEFAULT 0,
  `state`     int NOT NULL DEFAULT 0,          -- 0 挂单 / 1 已成交在途 / 2 自动运输
  `restype`   int NOT NULL DEFAULT 0,          -- 0粮 1木 2石 3铁 4金
  `count`     bigint NOT NULL DEFAULT 0,
  `price`     double NOT NULL DEFAULT 0,
  `gold`      bigint NOT NULL DEFAULT 0,
  `distance`  double NOT NULL DEFAULT 0,
  `unionid`   int NOT NULL DEFAULT 0,
  `limittime` bigint NOT NULL DEFAULT 0,
  `endtime`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_cid` (`cid`),
  KEY `idx_buycid` (`buycid`),
  KEY `idx_state_endtime` (`state`, `endtime`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_merchants（← sys_city_merchant：每日官市库存）───────────────────
CREATE TABLE IF NOT EXISTS `city_merchants` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `city_id`   int NOT NULL,
  `food`      bigint NOT NULL DEFAULT 100,
  `wood`      bigint NOT NULL DEFAULT 100,
  `rock`      bigint NOT NULL DEFAULT 100,
  `iron`      bigint NOT NULL DEFAULT 100,
  `gold`      bigint NOT NULL DEFAULT 10000,
  `trade_day` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_city_day` (`city_id`, `trade_day`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── city_autotrans（← mem_city_autotrans：自动运输）──────────────────────
CREATE TABLE IF NOT EXISTS `city_autotrans` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `user_id`    int NOT NULL,
  `fromcid`    int NOT NULL,
  `tocid`      int NOT NULL,
  `state`      int NOT NULL DEFAULT 0,
  `trans_type` int NOT NULL DEFAULT 0,          -- 0 单次 / 1 每日循环
  `start_time` bigint NOT NULL DEFAULT 0,
  `distance`   double NOT NULL DEFAULT 0,
  `cost_time`  bigint NOT NULL DEFAULT 0,
  `res_type`   int NOT NULL DEFAULT 0,
  `count`      bigint NOT NULL DEFAULT 0,
  `end_time`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`),
  KEY `idx_endtime` (`end_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_workshops（← sys_user_workshop：工匠作坊货架）───────────────────
CREATE TABLE IF NOT EXISTS `user_workshops` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `count`   int NOT NULL DEFAULT 0,             -- 今日刷新次数（≥50 拒）
  `gidstr`  text,                               -- "数量,gid,价格,gid,价格,..."
  `time`    bigint NOT NULL DEFAULT 0,          -- 上次刷新时间（+7200 冷却）
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_shops（← cfg_shop；原 dump 丢失 → 合成种子）──────────────────────
CREATE TABLE IF NOT EXISTS `cfg_shops` (
  `id`         int NOT NULL,
  `gid`        int NOT NULL DEFAULT 0,
  `price`      bigint NOT NULL DEFAULT 0,
  `pack`       int NOT NULL DEFAULT 1,
  `group`      int NOT NULL DEFAULT 0,
  `position`   int NOT NULL DEFAULT 0,
  `onsale`     int NOT NULL DEFAULT 1,
  `starttime`  bigint NOT NULL DEFAULT 0,
  `endtime`    bigint NOT NULL DEFAULT 4102444800,
  `commend`    int NOT NULL DEFAULT 0,
  `rebate`     int NOT NULL DEFAULT 0,
  `totalCount` int NOT NULL DEFAULT 2000000000,
  `userbuycnt` int NOT NULL DEFAULT 0,
  `daybuycnt`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_goods_copper（← cfg_goods_copper 五铢钱/积分商城；dump 丢失 → 合成）─
CREATE TABLE IF NOT EXISTS `cfg_goods_copper` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `gid`     int NOT NULL,
  `price`   bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,             -- 0 道具(cfg_goods) / 1 物品(cfg_things，新库无该配置表→不种子)
  `onsale`  int NOT NULL DEFAULT 1,
  `commend` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_gid_type` (`gid`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_shops（← log_shop：商城购买流水）─────────────────────────────────
CREATE TABLE IF NOT EXISTS `log_shops` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `shopid`  int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  `price`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `paytype` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user_shop` (`user_id`, `shopid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_shop_buy_cnts（← log_shop_buy_cnt：限购计数，uniq uid+sid）───────
CREATE TABLE IF NOT EXISTS `log_shop_buy_cnts` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `sid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_sid` (`user_id`, `sid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_merchants（← log_merchant：官市买卖流水）─────────────────────────
CREATE TABLE IF NOT EXISTS `log_merchants` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `time`    bigint NOT NULL DEFAULT 0,
  `wood`    bigint NOT NULL DEFAULT 0,
  `food`    bigint NOT NULL DEFAULT 0,
  `iron`    bigint NOT NULL DEFAULT 0,
  `rock`    bigint NOT NULL DEFAULT 0,
  `gold`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_gifts（← log_gift：addGift 与 addGoods(gid=0) 两种列形共用，gid 默认 0）─
CREATE TABLE IF NOT EXISTS `log_gifts` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `gid`     int NOT NULL DEFAULT 0,
  `count`   bigint NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  `type`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_workshop_freshes（← log_workshop_fresh）──────────────────────────
CREATE TABLE IF NOT EXISTS `log_workshop_freshes` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `gidstr`  text,
  `cost`    int NOT NULL DEFAULT 0,
  `time`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── log_user_actions / log_action_counts（← log_user_action / log_action_count）─
CREATE TABLE IF NOT EXISTS `log_user_actions` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `aid`     int NOT NULL,
  `time`    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `log_action_counts` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `aid`     int NOT NULL,
  `count`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user_aid` (`user_id`, `aid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── reports / alarms（← sys_report / sys_alarm：HandleTrade 战报）────────
CREATE TABLE IF NOT EXISTS `reports` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `user_id`   int NOT NULL,
  `origincid` int NOT NULL DEFAULT 0,
  `origincity` varchar(64) NOT NULL DEFAULT '',
  `happencid` int NOT NULL DEFAULT 0,
  `happencity` varchar(64) NOT NULL DEFAULT '',
  `title`     int NOT NULL DEFAULT 0,
  `type`      int NOT NULL DEFAULT 0,
  `time`      bigint NOT NULL DEFAULT 0,
  `read`      int NOT NULL DEFAULT 0,
  `battleid`  int NOT NULL DEFAULT 0,
  `content`   text,
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `alarms` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `report`  int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── tickets / ticket_contents（← sys_ticket / sys_ticket_content：礼券兑换）─
CREATE TABLE IF NOT EXISTS `tickets` (
  `id`        int NOT NULL AUTO_INCREMENT,
  `code`      varchar(32) NOT NULL,
  `user_id`   int NOT NULL DEFAULT 0,
  `binduid`   int NOT NULL DEFAULT 0,
  `contentid` int NOT NULL DEFAULT 0,
  `time`      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `ticket_contents` (
  `id`      int NOT NULL AUTO_INCREMENT,
  `content` text,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_buildings 补市场/工匠作坊（重写版 bid：13市场←legacy13、14工匠作坊←legacy15）─
REPLACE INTO `cfg_buildings` (`bid`, `name`, `table_name`, `inner`) VALUES
(13, '市场', 'market', 1),
(14, '工匠作坊', 'workshop', 1);

-- ── cfg_building_levels 市场(13)/作坊(14)（成本曲线无权威来源 → 合成 10 级）─
DELETE FROM `cfg_building_levels` WHERE `bid` IN (13, 14);
INSERT INTO `cfg_building_levels` (`bid`, `level`, `upgrade_wood`, `upgrade_rock`, `upgrade_iron`, `upgrade_food`, `upgrade_gold`, `upgrade_people`, `upgrade_time`, `using_people`) VALUES
(13,1,500,400,300,200,0,0,300,5),(13,2,1000,800,600,400,0,50,600,10),(13,3,2000,1600,1200,800,0,100,1200,15),(13,4,4000,3200,2400,1600,0,200,2400,20),(13,5,8000,6400,4800,3200,0,400,4800,25),
(13,6,16000,12800,9600,6400,0,800,9600,30),(13,7,32000,25600,19200,12800,0,1600,19200,35),(13,8,64000,51200,38400,25600,0,3200,38400,40),(13,9,128000,102400,76800,51200,0,6400,76800,45),(13,10,256000,204800,153600,102400,0,12800,153600,50),
(14,1,600,500,400,300,0,0,400,5),(14,2,1200,1000,800,600,0,50,800,10),(14,3,2400,2000,1600,1200,0,100,1600,15),(14,4,4800,4000,3200,2400,0,200,3200,20),(14,5,9600,8000,6400,4800,0,400,6400,25),
(14,6,19200,16000,12800,9600,0,800,12800,30),(14,7,38400,32000,25600,19200,0,1600,25600,35),(14,8,76800,64000,51200,38400,0,3200,51200,40),(14,9,153600,128000,102400,76800,0,6400,102400,45),(14,10,307200,256000,204800,153600,0,12800,204800,50);

-- ── cfg_goods 补 M6 用到的 gid（152 铜钱 / 10960 五铢钱 / 888888 积分 / 300-375 宝珠）─
REPLACE INTO `cfg_goods` (`gid`, `name`, `group_id`, `position`, `value`) VALUES
(152, '铜钱', 1, 152, 0),
(10960, '五铢钱', 1, 10960, 0),
(888888, '积分', 1, 888888, 0);

-- 宝珠 gid = 300 + 10*attid + (level-1)，attid∈{0,1,2,3,4,6,7}，level∈{1..5}
REPLACE INTO `cfg_goods` (`gid`, `name`, `group_id`, `position`, `value`) VALUES
(300,'宝石I',4,300,0),(301,'宝石II',4,301,0),(302,'宝石III',4,302,0),(303,'宝石IV',4,303,0),(304,'宝石V',4,304,0),
(310,'攻击宝石I',4,310,0),(311,'攻击宝石II',4,311,0),(312,'攻击宝石III',4,312,0),(313,'攻击宝石IV',4,313,0),(314,'攻击宝石V',4,314,0),
(320,'防御宝石I',4,320,0),(321,'防御宝石II',4,321,0),(322,'防御宝石III',4,322,0),(323,'防御宝石IV',4,323,0),(324,'防御宝石V',4,324,0),
(330,'体力宝石I',4,330,0),(331,'体力宝石II',4,331,0),(332,'体力宝石III',4,332,0),(333,'体力宝石IV',4,333,0),(334,'体力宝石V',4,334,0),
(340,'智力宝石I',4,340,0),(341,'智力宝石II',4,341,0),(342,'智力宝石III',4,342,0),(343,'智力宝石IV',4,343,0),(344,'智力宝石V',4,344,0),
(360,'敏捷宝石I',4,360,0),(361,'敏捷宝石II',4,361,0),(362,'敏捷宝石III',4,362,0),(363,'敏捷宝石IV',4,363,0),(364,'敏捷宝石V',4,364,0),
(370,'幸运宝石I',4,370,0),(371,'幸运宝石II',4,371,0),(372,'幸运宝石III',4,372,0),(373,'幸运宝石IV',4,373,0),(374,'幸运宝石V',4,374,0);

-- ── cfg_shops 合成种子（覆盖常规/commend/限量/ rebate/121 聚贤包 分支）────
REPLACE INTO `cfg_shops` (`id`, `gid`, `price`, `pack`, `group`, `position`, `onsale`, `starttime`, `endtime`, `commend`, `rebate`, `totalCount`, `userbuycnt`, `daybuycnt`) VALUES
(1, 1, 1, 1, 1, 1, 1, 0, 4102444800, 0, 0, 2000000000, 0, 0),
(2, 12, 5, 1, 1, 2, 1, 0, 4102444800, 0, 0, 2000000000, 0, 0),
(3, 23, 10, 1, 1, 3, 1, 0, 4102444800, 1, 0, 2000000000, 0, 0),
(4, 120, 20, 1, 1, 4, 1, 0, 4102444800, 0, 0, 2000000000, 0, 0),
(5, 138, 30, 1, 1, 5, 1, 0, 4102444800, 2, 0, 2000000000, 0, 0),
(121, 139, 50, 1, 1, 6, 1, 0, 4102444800, 0, 0, 2000000000, 0, 0),
(201, 16, 100, 1, 1, 7, 1, 0, 4102444800, 0, 0, 10, 3, 2),
(202, 11, 2, 1, 1, 8, 1, 0, 4102444800, 0, 1, 2000000000, 0, 0);

-- ── cfg_goods_copper 合成种子 ────────────────────────────────────────────
REPLACE INTO `cfg_goods_copper` (`gid`, `price`, `type`, `onsale`, `commend`) VALUES
(1, 100, 0, 1, 0),
(12, 500, 0, 1, 1),
(152, 10, 0, 1, 0);

-- ── 演示数据：测试账号 uid=1 / 城 cid=5 的市场与作坊 ─────────────────────
-- cid=5 补市场(bid13 lv5)与工匠作坊(bid14 lv3)建筑（xy 沿用城内空闲位）。
INSERT INTO `buildings` (`city_id`, `building_id`, `xy`, `level`, `state`, `state_start_at`, `state_end_at`)
SELECT 5, 13, 'a4', 5, 0, 0, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM buildings WHERE city_id = 5 AND building_id = 13);
INSERT INTO `buildings` (`city_id`, `building_id`, `xy`, `level`, `state`, `state_start_at`, `state_end_at`)
SELECT 5, 14, 'b4', 3, 0, 0, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM buildings WHERE city_id = 5 AND building_id = 14);

-- cid=5 的 city_res_add 行（若缺）与存放比例
INSERT IGNORE INTO `city_res_add` (`city_id`) VALUES (5);
UPDATE `city_res_add` SET `food_store`=25, `wood_store`=25, `rock_store`=25, `iron_store`=25 WHERE `city_id`=5 AND `food_store`=0 AND `wood_store`=0 AND `rock_store`=0 AND `iron_store`=0;

-- uid=1 补 gift（测试官市礼金支付）
UPDATE `users` SET `gift`=1000 WHERE `id`=1 AND `gift`=0;

-- 测试礼券（exchangeLiquan 贯通：LQA 前缀限 1 次；content="3,0,12,2,0,152,100,2,300,1" 即 3 项：免战牌×2、铜钱×100、宝石I×1）
REPLACE INTO `ticket_contents` (`id`, `content`) VALUES (1, '3,0,12,2,0,152,100,0,300,1');
REPLACE INTO `tickets` (`code`, `user_id`, `binduid`, `contentid`, `time`) VALUES ('LQATEST0001', 0, 0, 1, 0);
