package task

// service.go 1:1 复刻 legacy server/game/TaskFunc.php + utils.php 的 completeTask 族。
// 入口函数对照（文件:行号）：
//   utils.php:703 completeTask / :707 completeTaskWithTaskid / :715 completeTaskGoalBySortandType
//   TaskFunc.php:28 checkGoalComplete（核心判定，逐 sort/type 分支）
//   TaskFunc.php:613 checkTaskComplete（含 alarms 红点）/ :694 checkTaskCount
//   TaskFunc.php:652 dropTask / :734 dropSysTask
//   TaskFunc.php:753 getTaskTypeGroupList / :878 checkDailyTask
//   TaskFunc.php:889 getAllTaskByType / :963 getTaskList
//   TaskFunc.php:1098 getTaskDetail / :1738 getReward（见 reward.go）
//
// 原版怪癖 1:1 保留：
//   - checkGoalComplete:30 若 user_goal 有记录（uid 非空）且 sort∉{50,80} → 直接判完成；
//   - checkTaskComplete:632 / checkTaskCount:712 用 foreach 结束后泄漏的 $goal（最后一个 goal）取 tid
//     作为"是否含 sort=100 战场目标"的判据；
//   - getReward:1772 的 `($tid>800||$tid<400)` 恒真（原版逻辑或，几乎等价 true）；
//   - getReward:1807 sort=1&type∈{51,52} 的装备部位/颜色校验只在 $armor 为空时执行（原版 bug）：
//     装备不存在时 null!=count 恒真 → 判未完成；存在时反而完全不校验。
//
// 裁剪（未实现分支保留判定结构，按"原版返回 false/0"语义返回，并注释）：
//   联盟（type 11/21、union_id 均无列）、排行（53/54/55/56）、战场（sort 100/110/120、*_battle_*、log_battle*、
//   log_everyday_task）、洛阳/黄巾/董卓（mem_state、luoyang_/huangjin_/dongzhuo_progress、101762-101766、102012 之外的
//   事件）、委托悬赏任务（sys_pub_reward_task）、武将任务（sys_hero_task/sys_lionize/sys_attack_position）、
//   反沉迷/公告（sys_inform）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// 文案格式化小工具（对齐 legacy sprintf 单占位语义）。
func sprintf1(format string, count int64) string { return fmt.Sprintf(format, count) }
func sprintfS(format string, s string) string    { return fmt.Sprintf(format, s) }
func itoa(n int64) string                        { return strconv.FormatInt(n, 10) }

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// WithUserLock 暴露用户级锁（对齐 legacy lockUser/unlockUser 文件锁）。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	return s.lk.WithUserLock(ctx, uid, key, fn)
}

// errLegacy 构造与 legacy throw new Exception 等价的错误响应（HTTP 400，code=task_error）。
func errLegacy(msg string) error {
	return httpx.BadRequest("task_error", msg)
}

// lang 文案（server/game/lang.php 逐字）。
const (
	msgAlreadyGot      = "你已经领取过该任务的奖励。"    // getReward.already_got
	msgTaskNotFinished = "任务尚未完成,不能领取奖励"    // getReward.task_not_finished
	msgNoCfgTask       = "没有该任务。"           // getReward.no_cfg_task
	msgGlobalTaskEnd   = "该任务已经结束。"         // getReward.global_task_end
	msgNotEnoughRemain = "任务剩余次数不足，批量领取失败。" // getReward.not_enough_remain_task
	msgServerBusy      = "服务器忙，请稍后再进行操作。"   // pacifyPeople.server_busy
	msgNotTaskOfUser   = "你没有这个任务！"         // sysTask.not_task_of_user
	msgNotSysTask      = "这个任务不是随机任务！"      // sysTask.not_systask
	msgRewardTaskNoPer = "你已经领取过该任务的奖励。"
)

// ── 小工具 ────────────────────────────────────────────────────────────────

// cellInt 读取单列整型；无行返回 0（对齐 legacy empty(sql_fetch_one_cell)→0/null 语义）。
func (s *Service) cellInt(ctx context.Context, q string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// lastCityID 对齐 `select lastcid from sys_user where uid=uid`。
func (s *Service) lastCityID(ctx context.Context, uid int) (int, error) {
	v, err := s.cellInt(ctx, "select lastcid from users where id=?", uid)
	return int(v), err
}

// nobilityOf 读取 users.nobility（新库为 varchar，按原版数值语义解析；空串/非法 → 0）。
func (s *Service) nobilityOf(ctx context.Context, uid int) (int, error) {
	str, err := s.db.FetchCellString(ctx, "select nobility from users where id=?", uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(str))
	return n, nil
}

// getBufferNobility 对齐 utils.php:1534 getBufferNobility（推恩令 buftype=16/18）。
// 新库 user_buffers 与 legacy mem_user_buffer 同构，可 1:1 实现。
func (s *Service) getBufferNobility(ctx context.Context, uid int, realNobility int) (int, error) {
	bufparam, err := s.cellInt(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		return 0, err
	}
	if bufparam != 0 {
		nobility := realNobility + int(bufparam)
		if bufparam == 5 && nobility > 19 {
			nobility = 19
		}
		if bufparam == 2 && nobility > 18 {
			nobility = 18
		}
		if nobility > realNobility {
			return nobility, nil
		}
	}
	return realNobility, nil
}

// cityRes 对齐 `select c.* from mem_city_resource c,sys_user u where c.cid=u.lastcid and u.uid=uid`。
// 无行返回 nil（对应 PHP $cityres=false，取列得 null）。
func (s *Service) cityRes(ctx context.Context, uid int) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx,
		"select c.* from city_resources c join users u on u.lastcid=c.city_id where u.id=?", uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// goalsWithUserGoals 对齐 TaskFunc.php:621/700/790/1111/1790 的 left join 行集：
//
//	select g.*,u.uid,u.currentcount from cfg_task_goals g
//	left join user_goals u on u.gid=g.id and u.uid=? where g.tid=?
func (s *Service) goalsWithUserGoals(ctx context.Context, uid, tid int) ([]map[string]any, error) {
	return s.db.FetchRows(ctx,
		"select g.*,u.uid,u.currentcount from cfg_task_goals g left join user_goals u on u.gid=g.id and u.uid=? where g.tid=?",
		uid, tid)
}

// goalsForTasks 取一批 tid 的 goal（左右 join），按 tid 分组，供 checkTaskComplete/checkTaskCount 使用。
// 为对齐 legacy 每任务一条 SQL，这里逐 tid 查询。
func (s *Service) taskGoalRows(ctx context.Context, uid, tid int) ([]map[string]any, error) {
	return s.goalsWithUserGoals(ctx, uid, tid)
}

// ── checkGoalComplete（TaskFunc.php:28）───────────────────────────────────

// CheckGoalComplete 判断单个目标是否达成。goal 为 cfg_task_goals left join user_goals 的行。
func (s *Service) CheckGoalComplete(ctx context.Context, uid int, goal map[string]any) (bool, error) {
	sort := model.Int(goal, "sort")
	typ := model.Int(goal, "type")
	count := model.Int64(goal, "count")
	hasGoal := model.Int(goal, "uid") != 0 // 对应 !empty($goal['uid'])

	// 原版怪癖：非累计任务（sort∉{50,80}）只要 user_goal 有记录即视为完成。
	if hasGoal && sort != 50 && sort != 80 {
		return true, nil
	}
	// 战场任务：sys_user_goal 无记录即未完成。
	if sort == 100 || sort == 110 || sort == 120 {
		return false, nil
	}

	switch sort {
	case 1:
		return s.checkGoalSort1(ctx, uid, goal, typ, count)
	case 2: // 宝物（sys_goods）
		v, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, typ)
		return v >= count, err
	case 3: // 军队：lastcid 城该兵种数量
		v, err := s.cellInt(ctx,
			"select `count` from city_soldiers s join users u on u.lastcid=s.city_id where u.id=? and s.soldier_id=?", uid, typ)
		return v >= count, err
	case 4: // 城防
		v, err := s.cellInt(ctx,
			"select `count` from city_defences d join users u on u.lastcid=d.city_id where u.id=? and d.did=?", uid, typ)
		return v >= count, err
	case 5: // 任务物品
		v, err := s.cellInt(ctx, "select `count` from things where user_id=? and tid=?", uid, typ)
		return v >= count, err
	case 6: // 装备（按强化等级）
		stronglv := model.Int(goal, "strong_level")
		armorid := typ
		if armorid > 0 {
			v, err := s.cellInt(ctx,
				"select count(1) from user_armors where user_id=? and hid=0 and armorid=? and strong_level>=?", uid, armorid, stronglv)
			return v >= count, err
		}
		v, err := s.cellInt(ctx,
			"select count(1) from user_armors where user_id=? and hid=0 and strong_level>=?", uid, stronglv)
		return v >= count, err
	case 8: // 名城：拥有 type 类城池数量
		v, err := s.cellInt(ctx, "select count(*) from cities where user_id=? and type=?", uid, typ)
		return v >= count, err
	case 9: // 名将：拥有 npcid=type 的武将（新库无 npcid 列，映射 heroes.hero_type）
		v, err := s.cellInt(ctx, "select count(*) from heroes where user_id=? and hero_type=?", uid, typ)
		return v >= count, err
	case 101: // 活动临时事件（temp_act_event 无表）——裁剪
		return false, nil
	case 102: // 当天战场胜利（log_battle_honour 无表）——裁剪
		return false, nil
	case 103: // 活动将领（heroes.hero_type）
		v, err := s.cellInt(ctx, "select count(*) from heroes where user_id=? and hero_type=?", uid, typ)
		return v >= count, err
	case 104: // 战场杀将活动（cfg_act/log_act 无表）——裁剪
		return false, nil
	case 105: // 原版即空分支 → 落到函数末尾 return false
		return false, nil
	case 50, 80: // 累计/单次：currentcount >= count
		return model.Int64(goal, "currentcount") >= count, nil
	case 10, 11, 12: // 名将专属任务-掠夺/侦查（sys_attack_position 裁剪）
		return false, nil
	case 13: // 建筑等级：lastcid 城 bid=type 的最高等级
		cid, err := s.lastCityID(ctx, uid)
		if err != nil {
			return false, err
		}
		v, err := s.cellInt(ctx,
			"select coalesce(max(level),0) from buildings where city_id=? and building_id=?", cid, typ)
		return v >= count, err
	case 14: // 兵种转化（log_soldier_convert 无表）——裁剪
		return false, nil
	case 202: // 俘虏营（mem_city_captive 无表）——裁剪
		return false, nil
	case 201: // 武将等级
		if typ == 155 { // 火鸡猎人：按数量
			v, err := s.cellInt(ctx, "select count(*) from heroes where hero_type=? and state=0 and user_id=?", typ, uid)
			return v >= count, err
		}
		stronglv := model.Int64(goal, "strong_level")
		lv, err := s.cellInt(ctx, "select coalesce(max(level),0) from heroes where hero_type=? and state=0 and user_id=?", typ, uid)
		if err != nil {
			return false, err
		}
		if lv > 0 {
			return lv >= stronglv, nil
		}
		return false, nil
	case 203: // 备战洛阳（checkBeiZhanLuoYang 裁剪）
		return false, nil
	case 204: // 在线时间（log_login 无表）——裁剪
		return false, nil
	case 205: // 建筑个数
		cid, err := s.lastCityID(ctx, uid)
		if err != nil {
			return false, err
		}
		v, err := s.cellInt(ctx, "select count(id) from buildings where city_id=? and building_id=? and state=0", cid, typ)
		if err == nil && v >= count {
			return true, nil
		}
		return false, err
	case 206: // 指定兵种数量
		cid, err := s.lastCityID(ctx, uid)
		if err != nil {
			return false, err
		}
		v, err := s.cellInt(ctx, "select `count` from city_soldiers where city_id=? and soldier_id=?", cid, typ)
		return v >= count, err
	default:
		// 任务 id 特判分支（TaskFunc.php:555-588）
		if tid := model.Int(goal, "tid"); tid != 0 {
			switch tid {
			case 101762: // 软合服开服活动（log_goods 统计，边界不明）——裁剪
				return false, nil
			case 101763, 101764, 101765, 101766: // 联盟城池数——裁剪
				return false, nil
			case 102012: // 火鸡猎人 + 道具 19999
				hasHero, err := s.db.Exists(ctx, "select 1 from heroes where user_id=? and hero_type=147", uid)
				if err != nil || !hasHero {
					return false, err
				}
				v, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=19999", uid)
				return v > 0, err
			}
		}
		return false, nil
	}
}

// checkGoalSort1 处理 sort==1 的资源/属性类分支（TaskFunc.php:40-437）。
func (s *Service) checkGoalSort1(ctx context.Context, uid int, goal map[string]any, typ int, count int64) (bool, error) {
	switch {
	case typ == 9: // 声望（sys_user.prestige）
		v, err := s.cellInt(ctx, "select prestige from users where id=?", uid)
		return v >= count, err
	case typ == 53 || typ == 54 || typ == 55 || typ == 56: // 排行（rank_* 无表）——裁剪
		return false, nil
	case typ == 11: // 联盟人数——裁剪（无 union_id）
		return false, nil
	case typ == 17: // 官职
		v, err := s.cellInt(ctx, "select officepos from users where id=?", uid)
		return v >= count, err
	case typ == 18: // 爵位（含推恩）
		real, err := s.nobilityOf(ctx, uid)
		if err != nil {
			return false, err
		}
		// 新库 users.nobility 为 varchar，按原版数值语义转换（nobilityOf 解析）。
		nb, err := s.getBufferNobility(ctx, uid, real)
		if err != nil {
			return false, err
		}
		return int64(nb) >= count, nil
	case typ == 19 || typ == 22 || typ == 122: // 元宝
		v, err := s.cellInt(ctx, "select money from users where id=?", uid)
		return v >= count, err
	case typ == 20: // 名城数量（type>0 且 <5）
		v, err := s.cellInt(ctx, "select count(*) from cities where user_id=? and type>0 and type<5", uid)
		return v >= count, err
	case typ == 21: // 联盟城池数——裁剪
		return false, nil
	case typ >= 12 && typ <= 15: // 四种基础产量：lastcid 城对应建筑 sum(level*(level+1)*50)
		bid := map[int]int{12: game.BidFarmland, 13: game.BidWood, 14: game.BidRock, 15: game.BidIron}[typ]
		cid, err := s.lastCityID(ctx, uid)
		if err != nil {
			return false, err
		}
		v, err := s.cellInt(ctx,
			"select coalesce(sum(level*(level+1)*50),0) from buildings where city_id=? and building_id=?", cid, bid)
		return v >= count, err
	case typ >= 31 && typ <= 46: // 战场类（log_battle_honour / bak_sys_user_battle_state）——裁剪
		return false, nil
	case typ == 50: // 一定等级将领（heroes.state=0 且 herotype!=1000）
		v, err := s.cellInt(ctx,
			"select count(1) from heroes where state=0 and user_id=? and level>=? and hero_type!=1000", uid, count)
		return v >= 1, err
	case typ == 51: // 装备颜色（user_armors + cfg_armor.type）
		v, err := s.cellInt(ctx,
			"select count(1) from user_armors a join cfg_armor b on a.armorid=b.id where a.hid=0 and a.user_id=? and b.type=?", uid, count)
		return v >= 1, err
	case typ == 52: // 装备部位（cfg_armor.part）
		v, err := s.cellInt(ctx,
			"select count(1) from user_armors a join cfg_armor b on a.armorid=b.id where a.hid=0 and a.user_id=? and b.part=?", uid, count)
		return v >= 1, err
	case typ >= 60 && typ <= 64: // 七擒孟获跨服战场（bak_sys_user_battle_state）——裁剪
		return false, nil
	default:
		// 城市资源/属性（mem_city_resource）
		res, err := s.cityRes(ctx, uid)
		if err != nil {
			return false, err
		}
		if res == nil {
			return false, nil
		}
		switch typ {
		case 1:
			return model.Int64(res, "gold") >= count, nil
		case 2:
			return model.Int64(res, "food") >= count, nil
		case 3:
			return model.Int64(res, "wood") >= count, nil
		case 4:
			return model.Int64(res, "rock") >= count, nil
		case 5:
			return model.Int64(res, "iron") >= count, nil
		case 6:
			return model.Int64(res, "people") >= count, nil
		case 7:
			return int64(model.Int(res, "morale")) >= count, nil
		case 8:
			return int64(model.Int(res, "complaint")) >= count, nil
		case 10: // 人口上限
			return model.Int64(res, "people_max") >= count, nil
		case 16: // 黄金产量 = people*tax*0.01
			prod := float64(model.Int64(res, "people")) * float64(model.Int(res, "tax")) * 0.01
			return prod >= float64(count), nil
		}
		return false, nil
	}
}

// ── checkTaskComplete（TaskFunc.php:613）───────────────────────────────────

// CheckTaskComplete 遍历任务列表，逐个判定目标是否全部完成并把 state 写回 task；返回 firstSet
// （true=本批没有任何任务触发红点）。tasklist 中每个元素会被就地写入 "state"。
func (s *Service) CheckTaskComplete(ctx context.Context, uid int, tasklist []map[string]any) (bool, error) {
	firstSet := true
	for _, task := range tasklist {
		tid := model.Int(task, "id")
		goals, err := s.taskGoalRows(ctx, uid, tid)
		if err != nil {
			return false, err
		}
		complete := true
		var lastGoal map[string]any
		for _, goal := range goals {
			lastGoal = goal
			ok, err := s.CheckGoalComplete(ctx, uid, goal)
			if err != nil {
				return false, err
			}
			if !ok {
				complete = false
				break
			}
		}
		// 原版：用泄漏的最后一个 goal 的 tid 判断本任务是否含 sort=100 战场目标。
		if lastGoal != nil {
			ltid := model.Int(lastGoal, "tid")
			has100, err := s.db.Exists(ctx, "select 1 from cfg_task_goals where tid=? and sort=100", ltid)
			if err != nil {
				return false, err
			}
			if has100 {
				if model.Int(lastGoal, "uid") == 0 {
					complete = false
				} else {
					complete = true
				}
			}
		}
		task["state"] = complete
		if complete && firstSet {
			if _, err := s.db.Exec(ctx,
				"insert into alarms (user_id, task) values (?,1) on duplicate key update task=1", uid); err != nil {
				return false, err
			}
			firstSet = false
		}
	}
	return firstSet, nil
}

// CheckTaskCount（TaskFunc.php:694）统计列表内已完成任务数。
func (s *Service) CheckTaskCount(ctx context.Context, uid int, tasklist []map[string]any) (int, error) {
	count := 0
	for _, task := range tasklist {
		tid := model.Int(task, "id")
		goals, err := s.taskGoalRows(ctx, uid, tid)
		if err != nil {
			return 0, err
		}
		complete := true
		var lastGoal map[string]any
		for _, goal := range goals {
			lastGoal = goal
			ok, err := s.CheckGoalComplete(ctx, uid, goal)
			if err != nil {
				return 0, err
			}
			if !ok {
				complete = false
				break
			}
		}
		if lastGoal != nil {
			ltid := model.Int(lastGoal, "tid")
			has100, err := s.db.Exists(ctx, "select 1 from cfg_task_goals where tid=? and sort=100", ltid)
			if err != nil {
				return 0, err
			}
			if has100 {
				if model.Int(lastGoal, "uid") == 0 {
					complete = false
				} else {
					complete = true
				}
			}
		}
		if complete {
			count++
		}
	}
	return count, nil
}

// ── completeTask 族（utils.php:703-726）────────────────────────────────────

// CompleteTask 对应 completeTask：replace into user_goals(uid,gid)。
func (s *Service) CompleteTask(ctx context.Context, uid, goalid int) error {
	_, err := s.db.Exec(ctx, "replace into user_goals (`uid`,`gid`) values (?,?)", uid, goalid)
	return err
}

// CompleteTaskWithTaskid 对应 completeTaskWithTaskid：把某任务下所有 goal 全部标记完成。
func (s *Service) CompleteTaskWithTaskid(ctx context.Context, uid, taskid int) error {
	goals, err := s.db.FetchRows(ctx, "select id from cfg_task_goals where tid=?", taskid)
	if err != nil {
		return err
	}
	for _, g := range goals {
		if err := s.CompleteTask(ctx, uid, model.Int(g, "id")); err != nil {
			return err
		}
	}
	return nil
}

// CompleteTaskGoalBySortandType 对应 completeTaskGoalBySortandType：仅当该任务处于进行中(state=0)才完成。
func (s *Service) CompleteTaskGoalBySortandType(ctx context.Context, uid, sort, typ int) error {
	goals, err := s.db.FetchRows(ctx, "select id from cfg_task_goals where sort=? and type=?", sort, typ)
	if err != nil {
		return err
	}
	for _, g := range goals {
		id := model.Int(g, "id")
		taskid, err := s.cellInt(ctx, "select tid from cfg_task_goals where id=?", id)
		if err != nil {
			return err
		}
		if taskid == 0 {
			continue
		}
		ok, err := s.db.Exists(ctx, "select 1 from user_tasks where uid=? and tid=? and state=0 limit 1", uid, taskid)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := s.CompleteTask(ctx, uid, id); err != nil {
			return err
		}
	}
	return nil
}

// ── dropTask / dropSysTask（TaskFunc.php:652/734）─────────────────────────

// DropTask 对应 dropTask：按任务组丢弃任务。返回 getAllTaskByType(2) 的结构。
// 裁剪：taskgroup>=200000 的委托任务分支、taskgroup%10==1 的武将从属分支中的 sys_lionize/deleteHero 等。
func (s *Service) DropTask(ctx context.Context, uid, taskgroup int) ([]any, error) {
	if taskgroup >= 200000 {
		// 裁剪：委托/悬赏任务（sys_user_reward_task / sys_pub_reward_task 无表）。
		return s.GetAllTaskByType(ctx, uid, 4)
	}
	if taskgroup%10 == 1 {
		// 名将专属任务回收（武将从属相关）。
		npcid := int(float64(taskgroup-20001) / 10)
		taskmax := (40000+npcid)*10 + 9
		taskmin := (40000+npcid)*10 + 1
		// 裁剪：sys_lionize.state==2 → deleteHero($uid,$npcid) 及 log_lionize、sys_hero_task、sys_lionize 删除。
		if _, err := s.db.Exec(ctx, "delete from user_goals where uid=? and gid>=? and gid<=?", uid, taskmin, taskmax); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "delete from user_tasks where uid=? and tid>=? and tid<=?", uid, taskmin, taskmax); err != nil {
			return nil, err
		}
	} else {
		npcid := int(float64(taskgroup-20000) / 10)
		taskid1 := 20000 + npcid
		taskid2 := 30000 + npcid
		// delete a from sys_user_goal a,cfg_task_goal b where a.uid and a.gid=b.id and (b.tid=t1 or b.tid=t2)
		if _, err := s.db.Exec(ctx,
			"delete ug from user_goals ug join cfg_task_goals b on ug.gid=b.id where ug.uid=? and (b.tid=? or b.tid=?)",
			uid, taskid1, taskid2); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "delete from user_tasks where uid=? and (tid=? or tid=?)", uid, taskid1, taskid2); err != nil {
			return nil, err
		}
	}
	return s.GetAllTaskByType(ctx, uid, 2)
}

// DropSysTask 对应 dropSysTask：把随机系统任务置为已完成(state=1)。
func (s *Service) DropSysTask(ctx context.Context, uid, tid int) ([]any, error) {
	row, err := s.db.FetchOne(ctx, "select * from cfg_tasks where id=?", tid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy(msgNotTaskOfUser)
	}
	if err != nil {
		return nil, err
	}
	grp := model.Int(row, "group")
	if grp < 80000 || grp > 99999 {
		return nil, errLegacy(msgNotSysTask)
	}
	ok, err := s.db.Exists(ctx, "select 1 from user_tasks where uid=? and tid=?", uid, tid)
	if err != nil {
		return nil, err
	}
	if ok {
		if _, err := s.db.Exec(ctx, "update user_tasks set state=1 where uid=? and tid=?", uid, tid); err != nil {
			return nil, err
		}
	}
	return s.GetAllTaskByType(ctx, uid, 7)
}

// ── getTaskTypeGroupList（TaskFunc.php:753）───────────────────────────────

// GetTaskTypeGroupList 对应 getTaskTypeGroupList。
// 裁剪：type7 的系统任务刷新依赖 mem_user_schedule/mem_state，已按 user_schedule 保留"每日重置"部分；
// type4 委托任务、sys_hero_task、isBeiZhanLuoYangVisible 均裁剪。
func (s *Service) GetTaskTypeGroupList(ctx context.Context, uid, typ int) ([]any, error) {
	if typ < 0 || typ > 7 {
		typ = 0
	}
	ret := []any{}
	switch {
	case typ == 7:
		// 每日免费重置：last_reset_sys_task 与当前日期不同则重置计数。
		lastDate, err := s.db.FetchCellString(ctx,
			"select substr(from_unixtime(coalesce(last_reset_sys_task,0)),1,10) from user_schedule where user_id=?", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		curDate, err := s.db.FetchCellString(ctx, "select substr(now(),1,10)")
		if err != nil {
			return nil, err
		}
		if lastDate != curDate {
			if _, err := s.db.Exec(ctx, "insert into user_schedule (user_id,last_reset_sys_task) values (?,unix_timestamp()) on duplicate key update last_reset_sys_task=unix_timestamp()", uid); err != nil {
				return nil, err
			}
			if _, err := s.db.Exec(ctx, "insert into user_systask_num (user_id,count) values (?,0) on duplicate key update count=0", uid); err != nil {
				return nil, err
			}
		}
		// 裁剪：`update sys_user_task a,sys_user_taskstate b` 定时任务超时刷新（无 sys_user_taskstate 表）。
		task, err := s.db.FetchRows(ctx,
			"select distinct g.* from user_tasks u, cfg_tasks t, cfg_task_groups g where u.tid=t.id and u.uid=? and u.state=0 and g.id=t.`group` and g.type=? order by g.id", uid, typ)
		if err != nil {
			return nil, err
		}
		for _, group := range task {
			list, err := s.db.FetchRows(ctx,
				"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=? and u.state=0", uid, model.Int(group, "id"))
			if err != nil {
				return nil, err
			}
			cnt, err := s.CheckTaskCount(ctx, uid, list)
			if err != nil {
				return nil, err
			}
			group["count"] = cnt
		}
		ret = append(ret, task)
		n, err := s.cellInt(ctx, "select `count` from user_systask_num where user_id=?", uid)
		if err != nil {
			return nil, err
		}
		ret = append(ret, int(n))
		return ret, nil

	case typ <= 3 || typ >= 5:
		if typ == 1 { // 日常任务：确保 801-805 存在
			if err := s.checkDailyTask(ctx, uid); err != nil {
				return nil, err
			}
		}
		// 裁剪：isBeiZhanLuoYangVisible 恒 false，恒排除 112001 组。
		groupStr, err := s.fetchGroupConcat(ctx,
			"select group_concat(distinct(t.`group`)) from user_tasks u, cfg_tasks t where u.tid=t.id and u.uid=? and u.state=0 and t.`group`<>112001", uid)
		if err != nil {
			return nil, err
		}
		var task []map[string]any
		if groupStr == "" {
			task = []map[string]any{}
		} else {
			task, err = s.db.FetchRows(ctx,
				"select * from cfg_task_groups where id in ("+groupStr+") and type=? order by priority,id", typ)
			if err != nil {
				return nil, err
			}
		}
		// 裁剪：sys_hero_task 武将任务组。
		for _, group := range task {
			list, err := s.db.FetchRows(ctx,
				"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=? and u.state=0", uid, model.Int(group, "id"))
			if err != nil {
				return nil, err
			}
			cnt, err := s.CheckTaskCount(ctx, uid, list)
			if err != nil {
				return nil, err
			}
			group["count"] = cnt
		}
		ret = append(ret, task)
		// 裁剪：mem_state state=7/8（黄巾/董卓阶段）恒 0。
		ret = append(ret, 0)
		ret = append(ret, 0)
		real, err := s.nobilityOf(ctx, uid)
		if err != nil {
			return nil, err
		}
		nb, err := s.getBufferNobility(ctx, uid, real)
		if err != nil {
			return nil, err
		}
		ret = append(ret, nb >= 5)
		return ret, nil

	default: // type==4 委托任务——裁剪
		ret = append(ret, []any{})
		return ret, nil
	}
}

// checkDailyTask 对应 TaskFunc.php:878 checkDailyTask（801-805 开源节流任务）。
func (s *Service) checkDailyTask(ctx context.Context, uid int) error {
	n, err := s.cellInt(ctx, "select count(*) from user_tasks where tid in(801,802,803,804,805) and uid=?", uid)
	if err != nil {
		return err
	}
	if n != 0 {
		return nil
	}
	for _, tid := range []int{801, 802, 803, 804, 805} {
		if _, err := s.db.Exec(ctx, "insert into user_tasks (uid,tid,state) values (?,?,0)", uid, tid); err != nil {
			return err
		}
	}
	return nil
}

// ── getAllTaskByType（TaskFunc.php:889）───────────────────────────────────

// GetAllTaskByType 对应 getAllTaskByType。返回 [groups, list_per_group..., systask_num]（type<=3||>=5）。
// 裁剪：sys_hero_task、mem_user_systask_num→user_systask_num、type4 委托任务。
func (s *Service) GetAllTaskByType(ctx context.Context, uid, typ int) ([]any, error) {
	if typ < 0 || typ > 7 {
		typ = 0
	}
	ret := []any{}
	if typ <= 3 || typ >= 5 {
		rec, err := s.db.FetchRows(ctx,
			"select distinct g.* from user_tasks u, cfg_tasks t, cfg_task_groups g where t.`group`=g.id and u.tid=t.id and u.uid=? and u.state=0 and g.type=? order by g.id", uid, typ)
		if err != nil {
			return nil, err
		}
		// 裁剪：sys_hero_task 武将任务组。
		herotask := rec
		for _, group := range herotask {
			list, err := s.db.FetchRows(ctx,
				"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=? and u.state=0", uid, model.Int(group, "id"))
			if err != nil {
				return nil, err
			}
			cnt, err := s.CheckTaskCount(ctx, uid, list)
			if err != nil {
				return nil, err
			}
			group["count"] = cnt
		}
		ret = append(ret, herotask)
		for _, group := range herotask {
			list, err := s.GetTaskList(ctx, uid, model.Int(group, "id"))
			if err != nil {
				return nil, err
			}
			ret = append(ret, list)
		}
		n, err := s.cellInt(ctx, "select `count` from user_systask_num where user_id=?", uid)
		if err != nil {
			return nil, err
		}
		ret = append(ret, int(n))
		return ret, nil
	}
	// type==4 委托任务——裁剪：返回 [[], ...]。
	ret = append(ret, []any{})
	return ret, nil
}

// fetchGroupConcat 执行 group_concat 查询，返回去 NULL 的字符串（无行 → 空串）。
func (s *Service) fetchGroupConcat(ctx context.Context, q string, args ...any) (string, error) {
	v, err := s.db.FetchCellString(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// ── getTaskList（TaskFunc.php:963）────────────────────────────────────────

// GetTaskList 对应 getTaskList。返回 [tasklist, checkTaskCount]（tasklist 内每项被写入 state）。
func (s *Service) GetTaskList(ctx context.Context, uid, groupid int) ([]any, error) {
	if groupid >= 200000 {
		// 裁剪：委托/悬赏任务（sys_user_reward_task 无表）。
		return []any{[]any{}, 0}, nil
	}
	tasklist, err := s.db.FetchRows(ctx,
		"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=? and u.state=0 order by t.id", uid, groupid)
	if err != nil {
		return nil, err
	}
	// 裁剪：sys_hero_task 混合任务合并（templist）。
	if _, err := s.CheckTaskComplete(ctx, uid, tasklist); err != nil {
		return nil, err
	}
	cnt, err := s.CheckTaskCount(ctx, uid, tasklist)
	if err != nil {
		return nil, err
	}
	return []any{tasklist, cnt}, nil
}

// ── getTaskDetail（TaskFunc.php:1098）────────────────────────────────────

// GetTaskDetail 对应 getTaskDetail。返回 [cfg_task, goals(带 state), rewards]。
// 裁剪：250/251/279 的官职/爵位/联盟特殊文案与奖励（依赖 cfg_office_pos/cfg_nobility/union），
// sort 100/110/120 的战场奖励加成。
func (s *Service) GetTaskDetail(ctx context.Context, uid, tid int) ([]any, error) {
	if tid >= 200000 && tid < 400000 {
		// 裁剪：悬赏任务详情（getRewardTaskDetail）。
		return []any{}, nil
	}
	goalSortType, err := s.cellInt(ctx, "select sort from cfg_task_goals where tid=? limit 1", tid)
	if err != nil {
		return nil, err
	}
	ret := []any{}

	taskRow, err := s.db.FetchOne(ctx, "select * from cfg_tasks where id=?", tid)
	if errors.Is(err, sql.ErrNoRows) {
		ret = append(ret, nil)
	} else if err != nil {
		return nil, err
	} else {
		ret = append(ret, taskRow)
	}

	goals, err := s.db.FetchRows(ctx,
		"select g.*,u.uid,u.currentcount from cfg_task_goals g left join user_goals u on u.gid=g.id and u.uid=? where g.tid=? order by g.id", uid, tid)
	if err != nil {
		return nil, err
	}
	for _, goal := range goals {
		ok, err := s.CheckGoalComplete(ctx, uid, goal)
		if err != nil {
			return nil, err
		}
		goal["state"] = ok
	}
	// 裁剪：tid 250/251 用 cfg_office_pos/cfg_nobility 覆盖 content（新库无表，保留 cfg content）。
	ret = append(ret, goals)

	switch {
	case tid == 250: // 食君之禄（裁剪：cfg_office_pos 无表 → salary 0）
		ret = append(ret, []map[string]any{{"sort": 1, "type": 1, "count": 0}})
	case tid == 251: // 采食封邑（裁剪：cfg_nobility 无表 → salary 0）
		ret = append(ret, []map[string]any{
			{"sort": 1, "type": 2, "count": 0}, {"sort": 1, "type": 3, "count": 0},
			{"sort": 1, "type": 4, "count": 0}, {"sort": 1, "type": 5, "count": 0}})
	case tid == 279: // 共享利益（裁剪：getUnionFamousCityGold 恒 0）
		ret = append(ret, []map[string]any{{"sort": 1, "type": 1, "count": 0}})
	default:
		rewards, err := s.db.FetchRows(ctx, "select * from cfg_task_rewards where tid=? order by type asc", tid)
		if err != nil {
			return nil, err
		}
		// 裁剪：goalSortType 100/110/120 的战场等级奖励倍率。
		_ = goalSortType
		ret = append(ret, rewards)
	}
	return ret, nil
}

// formatUIDSQL 把 legacy 配置 SQL 模板里的 %d/%s 占位替换为 uid（AchivementFunc.php sprintf 语义）。
func formatUIDSQL(tmpl string, uid int) string {
	s := strconv.Itoa(uid)
	s = strings.ReplaceAll(tmpl, "%d", s)
	s = strings.ReplaceAll(s, "%s", strconv.Itoa(uid))
	return s
}
