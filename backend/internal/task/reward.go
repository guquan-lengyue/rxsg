package task

// reward.go 1:1 复刻 legacy TaskFunc.php:1204-1520 的奖励发放/目标扣减，以及 :1738 getReward 入口。
//   giveResource(:1204) giveGoods(:1344) giveArmy(:1355) giveDefence(:1368) giveThings(:1379)
//   cutArmorBySid(:1390) cutArmor(:1395) giveArmor(:1400) giveMoney(:1413) cutActEvent(:1419)
//   giveReward(:1424) reduceGoal(:1476) giveTask(utils:1038) giveTasks(utils:1049)
//   底层 addGoods(utils:971) addThings(utils:1030) addArmor(utils:1067) addMoney(utils:1119) addGift(utils:1135)
//   finishAchivement(utils:1777)
//
// 裁剪：log_city_soldier / sys_inform 广播 / log_day_money / sendDesignation / updateBattleOpenState /
//   战场网络奖励（case 500/501/502）/ NPC攻城（sort=8）/ 联盟与武将相关扣减。

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"

	"rxsg/backend/internal/model"
)

// lang 文案（server/game/lang.php resPackage / getReward 逐字）。
const (
	msgGainGold      = "获得黄金%d"
	msgGainFood      = "获得粮食%d"
	msgGainWood      = "获得木材%d"
	msgGainRock      = "获得石料%d"
	msgGainIron      = "获得铁锭%d"
	msgGainPeople    = "获得人口%d"
	msgGainMorale    = "增加民心%d"
	msgGainPrestige  = "增加声望%d"
	msgGainOfficepos = "晋升官职为%s"
	msgGainNobility  = "晋升爵位为%s"
	msgGainYuanbao   = "获得了【礼金*%d】"
	msgGainGoods     = "获得了【%s】"
	msgGainSoldier   = "获得了【%s】"
	msgGainDefence   = "获得了【%s】"
	msgGainThings    = "获得了【%s】"
	msgGainArmor     = "获得了【%s】"
	msgUnsupported   = "该任务暂未开放。" // 裁剪分支占位（非原版文案）
)

// ── 底层入账（addGift / addMoney / addGoods / addThings / addArmor）────────

func (s *Service) addGift(ctx context.Context, uid, gift, typ int) error {
	if gift == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, "insert into log_gifts (user_id,count,time,type) values (?,?,unix_timestamp(),?)", uid, gift, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", gift, uid)
	return err
}

// addMoney 对齐 utils.php:1119（log_day_money 每日消耗统计裁剪）。
func (s *Service) addMoney(ctx context.Context, uid int, money int64, typ int) error {
	if money == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, "insert into log_money (user_id,count,time,type) values (?,?,unix_timestamp(),?)", uid, money, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set money=money+? where id=?", money, uid)
	return err
}

// addGoods 对齐 utils.php:971 addGoods（含 gid=0 礼金、gid=152 铜钱双记账怪癖）。gid=160052 誓约依赖 log_qiyue（裁剪→当普通道具）。
func (s *Service) addGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if gid == 0 {
		if _, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", cnt, uid); err != nil {
			return err
		}
		_, err := s.db.Exec(ctx, "insert into log_gifts (user_id,gid,count,time,type) values (?,0,?,unix_timestamp(),?)", uid, cnt, typ)
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_goods (user_id,gid,`count`) values (?,?,?) on duplicate key update `count`=`count`+?", uid, gid, cnt, cnt); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "insert into log_goods (user_id,gid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, gid, cnt, typ); err != nil {
		return err
	}
	if gid == 152 { // 原版怪癖：铜钱按入账后总数再记一份积分(888888)入账与流水。
		total, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=152", uid)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx,
			"insert into user_goods (user_id,gid,`count`) values (?,888888,?) on duplicate key update `count`=`count`+?", uid, total, total); err != nil {
			return err
		}
		_, err = s.db.Exec(ctx, "insert into log_goods (user_id,gid,count,time,type) values (?,888888,?,unix_timestamp(),?)", uid, total, typ)
		return err
	}
	return nil
}

func (s *Service) addThings(ctx context.Context, uid, tid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, "insert into log_things (user_id,tid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, tid, cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "insert into things (user_id,tid,`count`) values (?,?,?) on duplicate key update `count`=`count`+?", uid, tid, cnt, cnt)
	return err
}

// addArmor 对齐 utils.php:1067 addArmor（updateBattleOpenState/sendDesignation 裁剪）。
func (s *Service) addArmor(ctx context.Context, uid, armorid int, cnt int64, typ, stronglevel, combineLevel int) error {
	if cnt == 0 {
		return nil
	}
	armor, err := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", armorid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	oriHpMax := model.Int(armor, "ori_hp_max")
	strongValue, err := s.cellInt(ctx, "select strong_value from cfg_strong_probability where level=?", stronglevel)
	if err != nil {
		return err
	}
	for i := int64(0); i < cnt; i++ {
		if _, err := s.db.Exec(ctx,
			"insert into user_armors (user_id,armorid,hp,hp_max,hid,strong_level,strong_value,combine_level) values (?,?,?,?,0,?,?,?)",
			uid, armorid, oriHpMax*10, oriHpMax, stronglevel, strongValue, combineLevel); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, "insert into log_armor (user_id,armorid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, armorid, cnt, typ)
	return err
}

// markResourceChanging 对齐 utils.php:926 updateCityResourceAdd（无 mem_world，直接用 cid 标记）。
func (s *Service) markResourceChanging(ctx context.Context, cid int) error {
	if cid <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx,
		"insert into city_res_add (city_id,resource_changing) values (?,1) on duplicate key update resource_changing=1", cid)
	return err
}

// ── giveResource（TaskFunc.php:1204）──────────────────────────────────────

func (s *Service) giveResource(ctx context.Context, uid, cid, typ int, count int64, logMoneyType int, retmsg bool) (string, error) {
	msg := ""
	switch typ {
	case 1:
		if _, err := s.db.Exec(ctx, "update city_resources set gold=gold+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainGold, count)
		}
	case 2:
		if _, err := s.db.Exec(ctx, "update city_resources set food=food+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainFood, count)
		}
	case 3:
		if _, err := s.db.Exec(ctx, "update city_resources set wood=wood+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainWood, count)
		}
	case 4:
		if _, err := s.db.Exec(ctx, "update city_resources set rock=rock+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainRock, count)
		}
	case 5:
		if _, err := s.db.Exec(ctx, "update city_resources set iron=iron+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainIron, count)
		}
	case 6:
		if _, err := s.db.Exec(ctx, "update city_resources set people=people+? where city_id=?", count, cid); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainPeople, count)
		}
	case 7: // 民心：LEAST(100, morale+count) 并同步 people_stable
		if retmsg {
			gain, err := s.cellInt(ctx, "select LEAST(100-morale,?) from city_resources where city_id=?", count, cid)
			if err != nil {
				return msg, err
			}
			msg = sprintf1(msgGainMorale, gain)
		}
		if _, err := s.db.Exec(ctx,
			"update city_resources set morale=LEAST(100,morale+?),`people_stable`=`people_max`*LEAST(100,morale+?)*0.01 where city_id=?",
			count, count, cid); err != nil {
			return msg, err
		}
	case 8: // 民怨：GREATEST(0, complaint+count)
		if _, err := s.db.Exec(ctx, "update city_resources set complaint=GREATEST(0,complaint+?) where city_id=?", count, cid); err != nil {
			return msg, err
		}
	case 9: // 声望：sys_user.prestige + warprestige
		if retmsg {
			gain, err := s.cellInt(ctx, "select GREATEST(100-morale,?) from city_resources where city_id=?", count, cid)
			if err != nil {
				return msg, err
			}
			msg = sprintf1(msgGainPrestige, gain)
		}
		if _, err := s.db.Exec(ctx, "update users set prestige=prestige+?,warprestige=warprestige+? where id=?", count, count, uid); err != nil {
			return msg, err
		}
	case 17: // 官职
		if _, err := s.db.Exec(ctx, "update users set officepos=greatest(officepos,?) where id=?", count, uid); err != nil {
			return msg, err
		}
		if retmsg {
			// 裁剪：cfg_office_pos 无表 → 名称不可得。
			msg = sprintfS(msgGainOfficepos, "")
		}
	case 18: // 爵位（新库 users.nobility 为 varchar，按原版数值语义 CAST 比较）
		if _, err := s.db.Exec(ctx, "update users set nobility=greatest(CAST(nobility AS SIGNED),?) where id=?", count, uid); err != nil {
			return msg, err
		}
		if retmsg {
			// 裁剪：cfg_nobility 无表 → 名称不可得。
			msg = sprintfS(msgGainNobility, "")
		}
		// 成就系统：爵位成就（utils.php:1280）
		switch {
		case count == 1:
			_ = s.finishAchivement(ctx, uid, 37)
		case count == 5:
			_ = s.finishAchivement(ctx, uid, 38)
		case count == 11:
			_ = s.finishAchivement(ctx, uid, 39)
		case count >= 14:
			_ = s.finishAchivement(ctx, uid, int(count)+26)
		}
	case 19: // 礼金/元宝（addGift）
		if err := s.addGift(ctx, uid, int(count), logMoneyType); err != nil {
			return msg, err
		}
		if retmsg {
			msg = sprintf1(msgGainYuanbao, count)
		}
	case 20:
		if err := s.addMoney(ctx, uid, count, 54); err != nil {
			return msg, err
		}
	case 22:
		if err := s.addMoney(ctx, uid, count, 57); err != nil {
			return msg, err
		}
	case 122:
		if err := s.addMoney(ctx, uid, count, logMoneyType); err != nil {
			return msg, err
		}
	case 30:
		if _, err := s.db.Exec(ctx, "update users set honour=honour+? where id=?", count, uid); err != nil {
			return msg, err
		}
	case 500, 501, 502:
		// 裁剪：战场网络奖励（sendRemoteRequest/sendRemote9001Request）。
	}
	return msg, nil
}

func (s *Service) giveGoods(ctx context.Context, uid, typ int, count int64, logType int, retmsg bool) (string, error) {
	msg := ""
	if err := s.addGoods(ctx, uid, typ, count, logType); err != nil {
		return msg, err
	}
	if retmsg {
		name, _ := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", typ)
		if count > 0 {
			name = name + "*" + itoa(count)
		}
		msg = sprintfS(msgGainGoods, name)
	}
	return msg, nil
}

func (s *Service) giveArmy(ctx context.Context, cid, typ int, count int64, retmsg bool) (string, error) {
	msg := ""
	if _, err := s.db.Exec(ctx,
		"insert into city_soldiers (`city_id`,`soldier_id`,`count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		cid, typ, count, count); err != nil {
		return msg, err
	}
	// 裁剪：log_city_soldier 无表。
	if err := s.markResourceChanging(ctx, cid); err != nil {
		return msg, err
	}
	if retmsg {
		name, _ := s.db.FetchCellString(ctx, "select name from cfg_soldiers where sid=?", typ)
		if count > 0 {
			name = name + "*" + itoa(count)
		}
		msg = sprintfS(msgGainSoldier, name)
	}
	return msg, nil
}

func (s *Service) giveDefence(ctx context.Context, cid, typ int, count int64, retmsg bool) (string, error) {
	msg := ""
	if _, err := s.db.Exec(ctx,
		"insert into city_defences (`city_id`,`did`,`count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		cid, typ, count, count); err != nil {
		return msg, err
	}
	if retmsg {
		name, _ := s.db.FetchCellString(ctx, "select name from cfg_defence where did=?", typ)
		if count > 0 {
			name = name + "*" + itoa(count)
		}
		msg = sprintfS(msgGainDefence, name)
	}
	return msg, nil
}

func (s *Service) giveThings(ctx context.Context, uid, typ int, count int64, logType int, retmsg bool) (string, error) {
	msg := ""
	if err := s.addThings(ctx, uid, typ, count, logType); err != nil {
		return msg, err
	}
	if retmsg {
		name, _ := s.db.FetchCellString(ctx, "select name from cfg_things where tid=?", typ)
		if count > 0 {
			name = name + "*" + itoa(count)
		}
		msg = sprintfS(msgGainThings, name)
	}
	return msg, nil
}

func (s *Service) giveArmor(ctx context.Context, uid, typ int, count int64, logType int, retmsg bool, stronglevel int) (string, error) {
	msg := ""
	if err := s.addArmor(ctx, uid, typ, count, logType, stronglevel, 0); err != nil {
		return msg, err
	}
	if retmsg {
		name, _ := s.db.FetchCellString(ctx, "select name from cfg_armor where id=?", typ)
		if count > 0 {
			name = name + "*" + itoa(count)
		}
		msg = sprintfS(msgGainArmor, name)
	}
	return msg, nil
}

func (s *Service) giveMoney(ctx context.Context, uid int, count int64, logMoneyType int, retmsg bool) (string, error) {
	msg := ""
	if err := s.addGift(ctx, uid, int(count), logMoneyType); err != nil {
		return msg, err
	}
	if retmsg {
		msg = sprintf1(msgGainYuanbao, count)
	}
	return msg, nil
}

// cutArmor 对齐 TaskFunc.php:1395（按 armorid 删 count 件，记 -count 流水）。
func (s *Service) cutArmor(ctx context.Context, uid, armorid int, count int64, logType int) error {
	if _, err := s.db.Exec(ctx, "delete from user_armors where user_id=? and armorid=? and hid=0 limit ?", uid, armorid, count); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "insert into log_armor (user_id,armorid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, armorid, -count, logType)
	return err
}

// cutArmorBySid 对齐 TaskFunc.php:1390。
func (s *Service) cutArmorBySid(ctx context.Context, uid, armorid, sid, logType int) error {
	if _, err := s.db.Exec(ctx, "delete from user_armors where user_id=? and sid=? and hid=0", uid, sid); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "insert into log_armor (user_id,armorid,count,time,type) values (?,?,-1,unix_timestamp(),?)", uid, armorid, logType)
	return err
}

// giveTask / giveTasks 对齐 utils.php:1038/1049。
func (s *Service) giveTask(ctx context.Context, uid, tid int) (string, error) {
	if _, err := s.db.Exec(ctx, "insert into user_tasks (uid,tid,state) values (?,?,0) on duplicate key update state=state", uid, tid); err != nil {
		return "", err
	}
	goals, err := s.db.FetchRows(ctx, "select id as gid from cfg_task_goals where tid=? and (sort=80 or sort=50)", tid)
	if err != nil {
		return "", err
	}
	for _, g := range goals {
		if _, err := s.db.Exec(ctx,
			"insert into user_goals (uid,gid,currentcount) values (?,?,0) on duplicate key update currentcount=currentcount", uid, model.Int(g, "gid")); err != nil {
			return "", err
		}
	}
	name, _ := s.db.FetchCellString(ctx, "select name from cfg_tasks where id=?", tid)
	return name, nil
}

func (s *Service) giveTasks(ctx context.Context, uid, groupid int) (string, error) {
	tasks, err := s.db.FetchRows(ctx, "select id from cfg_tasks where `group`=?", groupid)
	if err != nil {
		return "", err
	}
	for _, t := range tasks {
		if _, err := s.db.Exec(ctx, "insert into user_tasks (uid,tid,state) values (?,?,0) on duplicate key update state=state", uid, model.Int(t, "id")); err != nil {
			return "", err
		}
	}
	for _, t := range tasks {
		tid := model.Int(t, "id")
		goals, err := s.db.FetchRows(ctx, "select id as gid from cfg_task_goals where tid=? and (sort=80 or sort=50)", tid)
		if err != nil {
			return "", err
		}
		for _, g := range goals {
			if _, err := s.db.Exec(ctx,
				"insert into user_goals (uid,gid,currentcount) values (?,?,0) on duplicate key update currentcount=currentcount", uid, model.Int(g, "gid")); err != nil {
				return "", err
			}
		}
	}
	name, _ := s.db.FetchCellString(ctx, "select name from cfg_task_groups where id=?", groupid)
	return name, nil
}

// GiveReward 对齐 TaskFunc.php:1424 giveReward。
func (s *Service) GiveReward(ctx context.Context, uid, cid int, reward map[string]any, logType, logMoneyType int, retmsg bool, stronglevel int) (string, error) {
	sort := model.Int(reward, "sort")
	typ := model.Int(reward, "type")
	count := model.Int64(reward, "count")
	switch sort {
	case 1:
		return s.giveResource(ctx, uid, cid, typ, count, logMoneyType, retmsg)
	case 2:
		if typ == 0 {
			return s.giveMoney(ctx, uid, count, logMoneyType, retmsg)
		}
		return s.giveGoods(ctx, uid, typ, count, logType, retmsg)
	case 3:
		soldierType := typ
		if soldierType == 0 { // 随机兵种
			soldierType = rand.Intn(12) + 1
		}
		return s.giveArmy(ctx, cid, soldierType, count, retmsg)
	case 4:
		return s.giveDefence(ctx, cid, typ, count, retmsg)
	case 5:
		return s.giveThings(ctx, uid, typ, count, logType, retmsg)
	case 6:
		return s.giveArmor(ctx, uid, typ, count, logType, retmsg, stronglevel)
	case 10:
		return s.giveTask(ctx, uid, typ)
	case 11:
		return s.giveTasks(ctx, uid, typ)
	case 8:
		// 裁剪：NPC前来攻城（sys_troops 固定 uid=443 行，依赖完整部队字段）。
	}
	return "", nil
}

// ReduceGoal 对齐 TaskFunc.php:1476 reduceGoal。
func (s *Service) ReduceGoal(ctx context.Context, uid, cid int, goal map[string]any, logType, logMoneyType int, retmsg bool) error {
	sort := model.Int(goal, "sort")
	typ := model.Int(goal, "type")
	count := model.Int64(goal, "count")
	switch sort {
	case 1:
		_, err := s.giveResource(ctx, uid, cid, typ, -count, logMoneyType, retmsg)
		return err
	case 2:
		_, err := s.giveGoods(ctx, uid, typ, -count, logType, retmsg)
		return err
	case 3:
		_, err := s.giveArmy(ctx, cid, typ, -count, false)
		return err
	case 4:
		_, err := s.giveDefence(ctx, cid, typ, -count, false)
		return err
	case 5:
		_, err := s.giveThings(ctx, uid, typ, -count, logType, retmsg)
		return err
	case 6:
		return s.cutArmor(ctx, uid, typ, count, logType)
	case 101:
		// 裁剪：temp_act_event 无表（cutActEvent）。
		return nil
	case 203:
		// 裁剪：luoyang_progress/联盟任务扣减。
		return nil
	case 201:
		// 裁剪：活动将 deleteHero。
		return nil
	}
	return nil
}

// finishAchivement 对齐 utils.php:1777（广播/联盟公告裁剪）。
func (s *Service) finishAchivement(ctx context.Context, uid, achivementID int) error {
	if uid < 897 { // 原版怪癖：uid<897 直接返回
		return nil
	}
	closed, err := s.db.Exists(ctx, "select 1 from cfg_achivements where id=? and state=0", achivementID)
	if err != nil {
		return err
	}
	if closed { // 成就已关闭
		return nil
	}
	got, err := s.db.Exists(ctx, "select 1 from user_achivements where uid=? and achivement_id=?", uid, achivementID)
	if err != nil {
		return err
	}
	if got {
		return nil
	}
	openTime, err := s.cellInt(ctx, "select open_time from cfg_achivements where id=?", achivementID)
	if err != nil {
		return err
	}
	if openTime == 1 { // 脱离新手保护
		st, err := s.cellInt(ctx, "select state from users where id=?", uid)
		if err != nil {
			return err
		}
		if st != 0 {
			return nil
		}
	} else if openTime == 2 {
		// 裁剪：mem_state state=5（黄巾史诗完成）无表 → 恒不满足，直接返回。
		return nil
	}
	if _, err := s.db.Exec(ctx, "INSERT INTO user_achivements(uid,achivement_id,time) VALUES(?,?,UNIX_TIMESTAMP())", uid, achivementID); err != nil {
		return err
	}
	point, err := s.cellInt(ctx, "select point from cfg_achivements where id=?", achivementID)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"update users set achivement_count=achivement_count+1,achivement_point=achivement_point+? where id=?", point, uid); err != nil {
		return err
	}
	// 裁剪：union_inform/system_inform 公告（addUnionEvent/sendSysInform 无表）。
	return nil
}

// ── getReward（TaskFunc.php:1738）────────────────────────────────────────

// GetReward 领取任务奖励。selectID/selectID2 对应 legacy $selectId/$selectId2（装备/武将选择）。
func (s *Service) GetReward(ctx context.Context, uid, tid, selectID, selectID2 int) ([]any, error) {
	// ── 裁剪分支（保留判断，未实现按不支持返回）──
	switch {
	case tid >= 200000 && tid < 400000: // 委托/悬赏任务
		return nil, errLegacy(msgUnsupported)
	case tid >= 86000 && tid <= 86300: // 君主将任务（sys_user_task_num）
		return nil, errLegacy(msgUnsupported)
	case tid > 400 && tid < 800: // 名将公共任务（sys_hero_task/sys_lionize）
		return nil, errLegacy(msgUnsupported)
	case tid > 400000 && tid < 420000: // 名将专属任务
		return nil, errLegacy(msgUnsupported)
	case tid > 11000 && tid < 16000: // 黄巾/董卓史诗（huangjin_/dongzhuo_progress）
		return nil, errLegacy(msgUnsupported)
	case tid >= 112001 && tid <= 112004: // 洛阳备战
		return nil, errLegacy(msgUnsupported)
	case tid == 216 || tid == 219: // 激活邮件（mem_state）
		return nil, errLegacy(msgUnsupported)
	}

	var ret []any
	err := s.WithUserLock(ctx, uid, "getReward", func(ctx context.Context) error {
		r, e := s.getRewardLocked(ctx, uid, tid, selectID, selectID2)
		ret = r
		return e
	})
	return ret, err
}

func (s *Service) getRewardLocked(ctx context.Context, uid, tid, selectID, selectID2 int) ([]any, error) {
	// 非黄巾史诗任务，只能交一次（原版 `($tid>800||$tid<400)` 恒真怪癖）。
	recomplete, err := s.taskCanRecomplete(ctx, tid)
	if err != nil {
		return nil, err
	}
	if !recomplete {
		done, err := s.db.Exists(ctx, "select * from user_tasks where uid=? and tid=? and state=1", uid, tid)
		if err != nil {
			return nil, err
		}
		if done {
			return nil, errLegacy(msgAlreadyGot)
		}
	}
	// 存在性校验（原版同时校验 sys_user_task / sys_hero_task，hero 裁剪）。
	exists, err := s.db.Exists(ctx, "select 1 from user_tasks where uid=? and tid=?", uid, tid)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errLegacy(msgAlreadyGot)
	}

	task, err := s.db.FetchOne(ctx, "select * from cfg_tasks where id=?", tid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy(msgNoCfgTask)
	}
	if err != nil {
		return nil, err
	}
	if tid <= 0 {
		return nil, errLegacy(msgNoCfgTask)
	}
	inform := model.Int(task, "inform")
	retmsg := inform != 0

	goals, err := s.goalsWithUserGoals(ctx, uid, tid)
	if err != nil {
		return nil, err
	}
	complete := true
	for _, goal := range goals {
		ok, err := s.CheckGoalComplete(ctx, uid, goal)
		if err != nil {
			return nil, err
		}
		if !ok {
			complete = false
			break
		}
		if model.Int(goal, "sort") == 1 {
			typ := model.Int(goal, "type")
			if typ == 51 || typ == 52 { // 装备颜色/部位（保留原版"仅在空行时校验"的 bug）
				armor, err := s.db.FetchOne(ctx,
					"select b.part,b.type from user_armors a, cfg_armor b where a.armorid=b.id and a.user_id=? and a.sid=?", uid, selectID)
				armorMissing := errors.Is(err, sql.ErrNoRows)
				if err != nil && !armorMissing {
					return nil, err
				}
				if armorMissing { // 原版：empty($armor) 分支里 null 与 count 比较 → 恒不等 → 未完成
					if typ == 51 || typ == 52 {
						complete = false
						break
					}
				}
				_ = armor
			} else if typ == 50 { // 指定武将等级
				cnt, err := s.cellInt(ctx,
					"select count(1) from heroes where user_id=? and id=? and hero_type!=1000 and level>=?", uid, selectID, model.Int64(goal, "count"))
				if err != nil {
					return nil, err
				}
				if cnt == 0 {
					complete = false
					break
				}
			}
		}
	}
	// 事件任务 104430（两个装备同部位校验，原版逻辑）
	if model.Int(task, "group") == 104430 && tid != 104431 {
		a1, err := s.armorPartType(ctx, uid, selectID)
		if err != nil {
			return nil, err
		}
		a2, err := s.armorPartType(ctx, uid, selectID2)
		if err != nil {
			return nil, err
		}
		if a1 == nil || a2 == nil {
			complete = false
		} else if model.Int(a1, "part") != model.Int(a2, "part") {
			complete = false
		}
	}
	if !complete {
		return nil, errLegacy(msgTaskNotFinished)
	}

	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return nil, err
	}

	msg := ""
	switch {
	case tid == 250: // 食君之禄（裁剪：cfg_office_pos 无表 → salary 0）
		m, err := s.giveResource(ctx, uid, cid, 1, 0, 60, false)
		if err != nil {
			return nil, err
		}
		msg += m
	case tid == 251: // 采食封邑（裁剪：cfg_nobility 无表 → salary 0）
		for _, t := range []int{2, 3, 4, 5} {
			if _, err := s.giveResource(ctx, uid, cid, t, 0, 60, false); err != nil {
				return nil, err
			}
		}
	case tid == 279: // 共享利益（裁剪：getUnionFamousCityGold 恒 0）
		if _, err := s.giveResource(ctx, uid, cid, 1, 0, 60, false); err != nil {
			return nil, err
		}
	default:
		if tid == 101961 || tid == 101963 || tid == 101964 {
			if _, err := s.db.Exec(ctx, "update user_tasks set state=1 where uid=? and tid in (101961,101963,101964)", uid); err != nil {
				return nil, err
			}
		}
		rewards, err := s.db.FetchRows(ctx, "select * from cfg_task_rewards where tid=?", tid)
		if err != nil {
			return nil, err
		}
		for _, reward := range rewards {
			// 裁剪：goalSortType 100/110/120 的战场等级奖励倍率、log_everyday_task。
			m, err := s.GiveReward(ctx, uid, cid, reward, 4, 60, retmsg, 0)
			if err != nil {
				return nil, err
			}
			msg += m
		}
	}

	// 只能完成一次的任务：置 state=1（原版特殊活动组按区间批量置 1）。
	if !recomplete {
		switch {
		case tid >= 100632 && tid <= 100634:
			_, err = s.db.Exec(ctx, "update user_tasks set state=1 where uid=? and tid between 100632 and 100634", uid)
		case tid >= 100642 && tid <= 100644:
			_, err = s.db.Exec(ctx, "update user_tasks set state=1 where uid=? and tid between 100642 and 100644", uid)
		case tid >= 100652 && tid <= 100654:
			_, err = s.db.Exec(ctx, "update user_tasks set state=1 where uid=? and tid between 100652 and 100654", uid)
		default:
			_, err = s.db.Exec(ctx, "insert into user_tasks (uid,tid,state) values (?,?,1) on duplicate key update state=1", uid, tid)
		}
		if err != nil {
			return nil, err
		}
	}

	// 目标扣除（cfg_task_goal.reduce==1）
	reduceGoals, err := s.db.FetchRows(ctx, "select * from cfg_task_goals where tid=?", tid)
	if err != nil {
		return nil, err
	}
	group := model.Int(task, "group")
	for _, goal := range reduceGoals {
		if model.Int(goal, "reduce") != 1 {
			continue
		}
		gsort := model.Int(goal, "sort")
		gtype := model.Int(goal, "type")
		switch {
		case gsort == 6 && group == 104430 && tid != 104431:
			a1, _ := s.armorPartType(ctx, uid, selectID)
			a2, _ := s.armorPartType(ctx, uid, selectID2)
			if a1 != nil {
				_ = s.cutArmorBySid(ctx, uid, model.Int(a1, "id"), selectID, 4)
			}
			if a2 != nil {
				_ = s.cutArmorBySid(ctx, uid, model.Int(a2, "id"), selectID2, 4)
			}
		case gsort == 1 && (gtype == 51 || gtype == 52):
			a, _ := s.armorPartType(ctx, uid, selectID)
			if a != nil {
				_ = s.cutArmorBySid(ctx, uid, model.Int(a, "id"), selectID, 4)
			}
		case gsort == 1 && gtype == 50:
			// 裁剪：deleteHero（武将）。
		case gsort == 50:
			if _, err := s.db.Exec(ctx, "update user_goals set currentcount=0 where gid=? and uid=?", model.Int(goal, "id"), uid); err != nil {
				return nil, err
			}
		case gsort == 203:
			// 裁剪：luoyang_progress。
		default:
			if err := s.ReduceGoal(ctx, uid, cid, goal, 4, 60, false); err != nil {
				return nil, err
			}
		}
	}

	// 后续任务勾出（cfg_task.pretid）
	if err := s.triggerTasks(ctx, uid, tid); err != nil {
		return nil, err
	}
	// 裁剪：tid==243 董卓事件、inform 全服广播、log_task、recordUnionTask、checkXianDiMiZhaoEnoughTask、86000-86300 计数。

	_ = msg
	return []any{1}, nil
}

// armorPartType 取用户某件装备的 cfg_armor 行（含 id/part/type），无行返回 nil。
func (s *Service) armorPartType(ctx context.Context, uid, sid int) (map[string]any, error) {
	if sid <= 0 {
		return nil, nil
	}
	row, err := s.db.FetchOne(ctx,
		"select b.id,b.part,b.type from user_armors a, cfg_armor b where a.armorid=b.id and a.hid=0 and a.user_id=? and a.sid=?", uid, sid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// taskCanRecomplete 对齐 TaskFunc.php:1522。
func (s *Service) taskCanRecomplete(ctx context.Context, tid int) (bool, error) {
	if tid < 100000 {
		return (tid > 10000 && tid < 10600) || (tid > 11000 && tid < 15000) || (tid > 15000 && tid < 16000), nil
	}
	if tid <= 100396 {
		return (tid > 100021 && tid < 100025) || (tid > 100140 && tid < 100144) || (tid == 100171) || (tid == 100201) ||
			(tid == 100261 || tid == 100262) || (tid == 100281 || tid == 100291 || tid == 100292) || (tid >= 100311 && tid <= 100314) ||
			(tid >= 100321 && tid <= 100324) || (tid >= 100341 && tid <= 100343) || (tid >= 100361 && tid <= 100364) ||
			(tid >= 100381 && tid <= 100384) || (tid >= 100391 && tid <= 100396), nil
	}
	v, err := s.cellInt(ctx, "select default_state from cfg_tasks where id=?", tid)
	if err != nil {
		return false, err
	}
	return v == 100, nil
}

// triggerTasks 对齐 TaskFunc.php:2175 后续任务勾出（事件条件裁剪，state 与 goal 初始化保留）。
func (s *Service) triggerTasks(ctx context.Context, uid, tid int) error {
	triggers, err := s.db.FetchRows(ctx, "select * from cfg_tasks where pretid=?", tid)
	if err != nil {
		return err
	}
	for _, trigger := range triggers {
		tidNext := model.Int(trigger, "id")
		state := model.Int(trigger, "default_state")
		if state == 100 { // 100 仅表示可重复完成
			state = 0
		}
		// 裁剪：tid==243 黄巾/董卓阶段条件、103005-103009、10501/6301/6302/6401/6402、7001-7003(BATTLE_NET)。
		nextgoals, err := s.db.FetchRows(ctx, "select id from cfg_task_goals where tid=? and sort in (50,80)", tidNext)
		if err != nil {
			return err
		}
		for _, ng := range nextgoals {
			if _, err := s.db.Exec(ctx,
				"insert into user_goals (uid,gid,currentcount) values (?,?,0) on duplicate key update currentcount=currentcount",
				uid, model.Int(ng, "id")); err != nil {
				return err
			}
		}
		if _, err := s.db.Exec(ctx,
			"insert into user_tasks (uid,tid,state) values (?,?,?) on duplicate key update state=?", uid, tidNext, state, state); err != nil {
			return err
		}
	}
	return nil
}
