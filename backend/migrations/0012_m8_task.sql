-- 0012_m8_task.sql
-- M8 任务/成就（1:1 复刻 legacy TaskFunc.php / AchivementFunc.php / utils.php completeTask 族）。
-- 口径来源（文件:行号）：
--   cfg_task_group     ← TaskFunc.php:802/772 getTaskTypeGroupList 读取 cfg_task_group(id,type,name,description,priority)
--   cfg_task           ← utils.php:694/1532、TaskFunc.php:737/899 读取 cfg_task(id,`group`,pretid,name,todo,`default`,inform)
--   cfg_task_goal      ← TaskFunc.php:621/709/1111 读取 cfg_task_goal(id,tid,type,count,sort,content,reduce,strong_level)
--   cfg_task_reward    ← TaskFunc.php:1162/2087 读取 cfg_task_reward(id,tid,type,count,sort)
--   user_tasks         ← sys_user_task(uid,tid,state)（唯一键 (uid,tid)，on duplicate key update state）
--   user_goals         ← sys_user_goal(uid,gid,currentcount)（replace into / on duplicate key）
--   alarms.task        ← sys_alarm(uid,task) 任务红点（TaskFunc.php:644/993）；新库已有 alarms(user_id,report)，
--                        此处按"最小改动"给 alarms 补一列 task，不另建表。
--   cfg_achivement_group / cfg_achivement / cfg_achivement_goal / cfg_achivement_goal_mapping / sys_user_achivement
--                      ← AchivementFunc.php 全文 + utils.php finishAchivement:1777
--   users.achivement_point/achivement_count/officepos ← sys_user 同名（finishAchivement/giveResource）
--   user_systask_num / user_schedule.last_reset_sys_task ← mem_user_systask_num / mem_user_schedule（系统随机任务）
--
-- ⚠ 合成数据声明：真实 dump 已丢失，本文件内全部 cfg_task* / cfg_achivement* 数据均为【合成演示数据】，
--   仅用于跑通 1:1 复刻逻辑与集成测试，数值/文案非原版精确复刻。
--
-- 幂等：CREATE TABLE IF NOT EXISTS；补列 information_schema 判断 + PREPARE；种子 REPLACE INTO。
--
-- 裁剪（不建表，源码注释逐处声明）：sys_union* / rank_user* / sys_user_battle_state / log_battle* /
--   log_everyday_task / sys_user_reward_task / sys_pub_reward_task / sys_hero_task / sys_lionize /
--   sys_attack_position / luoyang_progress / huangjin_progress / dongzhuo_progress / temp_act_event /
--   log_act / log_soldier_convert / mem_city_captive / cfg_act / mem_state / sys_user_taskstate /
--   log_task / log_task_epic / log_login / sys_things(→things) 等（详见 task 包源码注释）。

SET NAMES utf8mb4;

-- ── cfg_task_groups（← cfg_task_group）────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_task_groups` (
  `id`          int NOT NULL,
  `type`        int NOT NULL DEFAULT 0,   -- 0成长 1日常 2成长 3.. 5..6.. 7系统随机（legacy getTaskTypeGroupList）
  `name`        varchar(64) NOT NULL DEFAULT '',
  `description` varchar(255) NOT NULL DEFAULT '',
  `priority`    int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_tasks（← cfg_task）────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_tasks` (
  `id`            int NOT NULL,
  `group`         int NOT NULL DEFAULT 0,   -- 所属任务组（legacy cfg_task.group，保留列名 group）
  `pretid`        int NOT NULL DEFAULT 0,   -- 前置任务 id（完成本任务后勾出 pretid=本任务 的后续）
  `name`          varchar(64) NOT NULL DEFAULT '',
  `todo`          varchar(255) NOT NULL DEFAULT '',
  `default_state` int NOT NULL DEFAULT 0,   -- legacy cfg_task.`default`（列名 default 为保留字，改名 default_state）：100=不限次数
  `inform`        int NOT NULL DEFAULT 0,   -- 1=完成时全服公告（广播裁剪）
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_task_goals（← cfg_task_goal）──────────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_task_goals` (
  `id`           int NOT NULL,
  `tid`          int NOT NULL DEFAULT 0,
  `type`         int NOT NULL DEFAULT 0,
  `count`        bigint NOT NULL DEFAULT 0,
  `sort`         int NOT NULL DEFAULT 0,   -- 1资源 2宝物 3军队 4城防 5任务物品 6装备 8名城 9名将 50/80累计 201武将等级 ...
  `content`      varchar(255) NOT NULL DEFAULT '',
  `reduce`       int NOT NULL DEFAULT 0,   -- 1=领取奖励时按本目标扣除相应资源/道具（TaskFunc.php:2148）
  `strong_level` int NOT NULL DEFAULT 0,   -- sort6/201/102 的强化等级/战场等级门槛
  PRIMARY KEY (`id`),
  KEY `idx_tid` (`tid`),
  KEY `idx_sort_type` (`sort`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── cfg_task_rewards（← cfg_task_reward）──────────────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_task_rewards` (
  `id`    int NOT NULL,
  `tid`   int NOT NULL DEFAULT 0,
  `type`  int NOT NULL DEFAULT 0,   -- sort1资源类型/sort2道具gid(0=礼金)/sort3兵种/sort4城防/sort5物品/sort6装备
  `count` bigint NOT NULL DEFAULT 0,
  `sort`  int NOT NULL DEFAULT 0,   -- 1资源 2道具 3兵力 4城防 5物品 6装备 10任务 11任务组
  PRIMARY KEY (`id`),
  KEY `idx_tid` (`tid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_tasks（← sys_user_task）──────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `user_tasks` (
  `uid`   int NOT NULL,
  `tid`   int NOT NULL,
  `state` int NOT NULL DEFAULT 0,   -- 0进行中 1已完成/已领取
  PRIMARY KEY (`uid`, `tid`),
  KEY `idx_uid_state` (`uid`, `state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_goals（← sys_user_goal）──────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `user_goals` (
  `uid`          int NOT NULL,
  `gid`          int NOT NULL,
  `currentcount` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`, `gid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── user_systask_num（← mem_user_systask_num：系统随机任务每日领取计数）──
CREATE TABLE IF NOT EXISTS `user_systask_num` (
  `user_id` int NOT NULL,
  `count`   int NOT NULL DEFAULT 0,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ── alarms.task（← sys_alarm.task：任务红点）──────────────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'alarms' AND COLUMN_NAME = 'task') = 0,
  'ALTER TABLE `alarms` ADD COLUMN `task` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── user_schedule.last_reset_sys_task（← mem_user_schedule.last_reset_sys_task：系统任务每日刷新）──
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_schedule' AND COLUMN_NAME = 'last_reset_sys_task') = 0,
  'ALTER TABLE `user_schedule` ADD COLUMN `last_reset_sys_task` bigint NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── users 补列（← sys_user：成就点数/成就数/官职）───────────────────────
SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'achivement_point') = 0,
  'ALTER TABLE `users` ADD COLUMN `achivement_point` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'achivement_count') = 0,
  'ALTER TABLE `users` ADD COLUMN `achivement_count` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

SET @sql = IF((SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'officepos') = 0,
  'ALTER TABLE `users` ADD COLUMN `officepos` int NOT NULL DEFAULT 0', 'DO 0');
PREPARE st FROM @sql; EXECUTE st; DEALLOCATE PREPARE st;

-- ── 成就：组 / 成就 / 成就目标 / 映射 / 用户成就 ─────────────────────────
CREATE TABLE IF NOT EXISTS `cfg_achivement_groups` (
  `id`   int NOT NULL,
  `name` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_achivements` (
  `id`                int NOT NULL,
  `group`             int NOT NULL DEFAULT 0,   -- 成就组 id
  `sub_group`         int NOT NULL DEFAULT 0,
  `name`              varchar(64) NOT NULL DEFAULT '',
  `content`           varchar(255) NOT NULL DEFAULT '',
  `todo`              varchar(255) NOT NULL DEFAULT '',
  `image`             varchar(128) NOT NULL DEFAULT '',
  `type`              int NOT NULL DEFAULT 0,   -- 0普通 2数值型(sql_current_value) 3目标型(cfg_achivement_goal)
  `point`             int NOT NULL DEFAULT 0,
  `target_value`      bigint NOT NULL DEFAULT 0,
  `sql_current_value` varchar(255) NOT NULL DEFAULT '',   -- 含 %d/%s 占位，uid 代入
  `sql_check_goal`    varchar(255) NOT NULL DEFAULT '',
  `open_time`         int NOT NULL DEFAULT 0,   -- 0常开 1脱离新手保护 2黄巾史诗完成（mem_state，裁剪）
  `state`             int NOT NULL DEFAULT 1,   -- 0关闭 1开放
  `union_inform`      int NOT NULL DEFAULT 0,   -- 联盟公告（裁剪）
  `system_inform`     int NOT NULL DEFAULT 0,   -- 全服公告（裁剪）
  PRIMARY KEY (`id`),
  KEY `idx_group` (`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_achivement_goals` (
  `id`             int NOT NULL,
  `content`        varchar(255) NOT NULL DEFAULT '',
  `sql_check_goal` varchar(255) NOT NULL DEFAULT '',   -- 含 %d/%s 占位，uid 代入
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `cfg_achivement_goal_mappings` (
  `achivement_id`      int NOT NULL,
  `achivement_goal_id` int NOT NULL,
  PRIMARY KEY (`achivement_id`, `achivement_goal_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_achivements` (
  `id`            int NOT NULL AUTO_INCREMENT,
  `uid`           int NOT NULL,
  `achivement_id` int NOT NULL,
  `time`          bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_uid_ach` (`uid`, `achivement_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ══════════════════════════════════════════════════════════════════════════
-- 以下全部为【合成演示数据】（真实配置在迁移中丢失）
-- ══════════════════════════════════════════════════════════════════════════

-- 任务组：1=成长任务(type=2，供 getTaskList/getAllTaskByType(type=2) 测试)；2=名将任务(type=2，供 dropTask 测试)
REPLACE INTO `cfg_task_groups` (`id`,`type`,`name`,`description`,`priority`) VALUES
(1, 2, '成长任务', '完成成长任务可领取奖励。', 100),
(2, 2, '名将任务', '名将专属任务（合成演示）。', 200);

-- 任务：1001/1002/1003 属组 1；20000/30000 属组 2（dropTask 尾号非 1 分支删除）；80001 属系统随机任务组
REPLACE INTO `cfg_tasks` (`id`,`group`,`pretid`,`name`,`todo`,`default_state`,`inform`) VALUES
(1001, 1, 0, '初露锋芒', '掠夺资源，囤积国力。', 0, 0),
(1002, 1, 0, '兵强马壮', '招募并驻守军队。', 0, 0),
(1003, 1, 0, '上缴贡品', '向朝廷上缴黄金。', 0, 0),
(20000, 2, 0, '名将任务·甲', '合成演示任务。', 0, 0),
(30000, 2, 0, '名将任务·乙', '合成演示任务。', 0, 0),
(80001, 80001, 0, '随机任务·甲', '系统随机任务（合成演示）。', 100, 0);

-- 目标：覆盖 sort 1(资源) / 2(宝物) / 3(军队) / 5(任务物品) / 15(建筑) 等多分支
REPLACE INTO `cfg_task_goals` (`id`,`tid`,`type`,`count`,`sort`,`content`,`reduce`,`strong_level`) VALUES
(10011, 1001, 1, 5000, 1, '本城黄金达到 5000', 0, 0),
(10012, 1001, 1, 2,    2, '拥有传音符 2 个', 0, 0),
(10021, 1002, 1, 10,   3, '拥有 1 级兵种 10 个', 0, 0),
(10022, 1002, 1, 3,    5, '拥有任务物品 1 共 3 个', 0, 0),
(10031, 1003, 1, 100,  1, '上缴黄金 100', 1, 0),
(20001, 20000, 1, 1,   1, '合成演示目标。', 0, 0),
(30001, 30000, 1, 1,   1, '合成演示目标。', 0, 0);

-- 奖励：覆盖 sort 1资源/2道具(0=礼金)/3兵力/4城防
REPLACE INTO `cfg_task_rewards` (`id`,`tid`,`type`,`count`,`sort`) VALUES
(101, 1001, 2, 1000, 1),
(102, 1001, 0, 50,   2),
(103, 1001, 1, 5,    2),
(104, 1002, 1, 20,   3),
(105, 1002, 1, 5,    4),
(106, 1002, 1, 8000, 1),
(107, 1003, 0, 10,   2);

-- 成就组
REPLACE INTO `cfg_achivement_groups` (`id`,`name`) VALUES
(1, '发展成就'),
(2, '军事成就');

-- 成就：type 0(普通) / 2(数值型) / 3(目标型)，open_time 0常开 / 1脱离新手保护
REPLACE INTO `cfg_achivements`
(`id`,`group`,`sub_group`,`name`,`content`,`todo`,`image`,`type`,`point`,`target_value`,`sql_current_value`,`sql_check_goal`,`open_time`,`state`,`union_inform`,`system_inform`) VALUES
(1001, 1, 0, '初来乍到', '君主脱离新手保护。', '脱离新手保护', '', 0, 10, 0,  '', '', 0, 1, 0, 0),
(1002, 1, 0, '富甲一方', '拥有 10000 黄金。', '攒够黄金', '', 2, 20, 10000, 'select coalesce(sum(gold),0) from city_resources c join users u on u.lastcid=c.city_id where u.id=%d', '', 0, 1, 0, 0),
(1003, 2, 0, '武力超群', '拥有 1 名等级 10 以上武将。', '培养武将', '', 2, 30, 10, 'select coalesce(max(level),0) from heroes where user_id=%d', '', 1, 1, 0, 0),
(1004, 2, 0, '三军用命', '达成全部军事目标。', '完成军事目标', '', 3, 40, 0, '', '', 0, 1, 0, 0);

REPLACE INTO `cfg_achivement_goals` (`id`,`content`,`sql_check_goal`) VALUES
(1, '拥有 1 座城池', 'select 1 from cities where user_id=%d limit 1'),
(2, '拥有 1 名武将', 'select 1 from heroes where user_id=%d limit 1');

REPLACE INTO `cfg_achivement_goal_mappings` (`achivement_id`,`achivement_goal_id`) VALUES
(1004, 1),
(1004, 2);
