package armor

import (
	"context"
	"fmt"
	"strconv"
)

// combine.go 复刻 legacy EquipmentFunc.php combineArmor(1487)。
// 参数序：mainSid, mainFlag(必=1), subSid1, subSid2, goodsFlag(0|1)。
// 怪癖/legacy 原样：
//   - 必耗/保护符扣减在末尾统一执行（成功失败都扣）；
//   - goodsFlag=0 时 usegoods=-11 → addGoods(uid,-11,-1,0)（gid=-11 入 user_goods，
//     upsert count+(-1)，legacy 原样保留）；
//   - 成功仅主件 combine_level+1（least 7），两副件删除；失败无保护符主+副全毁；
//   - 备份 insert...select 在 legacy 带 @（静默失败），Go 同样忽略错误；
//   - 2013-09 活动奖励时间窗（1384416000-1385020800）恒不触发；
//   - sendSysInform 公告属 M8/M9 未接线；ret 数组中 addSpecialArr 等展示辅助省略，
//     Go 版返回 {success, goodsFlag, newCombineLevel}。

// CombineResult combineArmor 返回。
type CombineResult struct {
	Success         int `json:"success"`           // 1/0
	GoodsFlag       int `json:"goods_flag"`        // 是否使用保护符
	NewCombineLevel int `json:"new_combine_level"` // 结算后主件熔炼等级
}

// CombineArmor 对齐 EquipmentFunc.php:1487。
func (s *Service) CombineArmor(ctx context.Context, uid, mainSid, mainFlag int, subSid1, subSid2, goodsFlag int) (*CombineResult, error) {
	if mainFlag != 1 {
		return nil, errf(msgInvalidParam)
	}
	if subSid1 == 0 || subSid2 == 0 {
		return nil, errf(msgWaiguaInvalid)
	}
	if goodsFlag > 1 || goodsFlag < 0 {
		return nil, errf(msgWaiguaInvalid)
	}
	if mainSid == subSid1 || mainSid == subSid2 || subSid1 == subSid2 {
		return nil, errf(msgWaiguaInvalid)
	}

	mainArmorID, err := cellInt(ctx, s.db, "select armorid from user_armors where sid=? and user_id=? limit 1", mainSid, uid)
	if err != nil {
		return nil, err
	}
	if mainArmorID == 0 { // legacy empty
		return nil, errf(msgWaiguaInvalid)
	}
	armorPart, err := cellInt(ctx, s.db, `select a.part from cfg_armor a, user_armors b
		where a.id=b.armorid and b.sid=? limit 1`, mainSid)
	if err != nil {
		return nil, err
	}
	if armorPart == partMount {
		return nil, errf(msgCombineZuoji)
	}
	sub1, err := s.fetchOneOrNil(ctx, "select * from user_armors where user_id=? and sid=? and armorid=? limit 1", uid, subSid1, int(mainArmorID))
	if err != nil {
		return nil, err
	}
	sub2, err := s.fetchOneOrNil(ctx, "select * from user_armors where user_id=? and sid=? and armorid=? limit 1", uid, subSid2, int(mainArmorID))
	if err != nil {
		return nil, err
	}
	if sub1 == nil || sub2 == nil {
		return nil, errf(msgWaiguaInvalid)
	}
	// 副件在 hero_armors（穿戴中）拒
	inHero, err := s.db.Exists(ctx, "select 1 from hero_armors where sid in (?,?) limit 1", subSid1, subSid2)
	if err != nil {
		return nil, err
	}
	if inHero {
		return nil, errf(msgArmorInHero)
	}

	combineLevel := 0
	{ // select combine_level from sys_user_armor where sid=$mainSid（行存在性已校验）
		clv, err := cellInt(ctx, s.db, "select combine_level from user_armors where sid=? and user_id=? limit 1", mainSid, uid)
		if err != nil {
			return nil, err
		}
		combineLevel = int(clv)
	}
	l1, err := cellInt(ctx, s.db, "select combine_level from user_armors where sid=? limit 1", subSid1)
	if err != nil {
		return nil, err
	}
	l2, err := cellInt(ctx, s.db, "select combine_level from user_armors where sid=? limit 1", subSid2)
	if err != nil {
		return nil, err
	}
	if combineLevel != int(l1) || combineLevel != int(l2) {
		return nil, errf(msgCombineLevelNE)
	}
	if combineLevel >= maxCombine {
		return nil, errf(msgCombineMaxLevel)
	}

	usegoods := -11
	mustGoods := gidFuse1
	if combineLevel >= 3 {
		mustGoods = gidFuse2
	}
	cnt, err := s.goodsCount(ctx, uid, mustGoods)
	if err != nil {
		return nil, err
	}
	if cnt == 0 { // legacy empty($count)
		if mustGoods == gidFuse1 {
			return nil, errf(msgNotEnoughFuse1)
		}
		return nil, errf(msgNotEnoughFuse2)
	}
	if goodsFlag == 1 {
		usegoods = gidFusePro
		cnt, err := s.goodsCount(ctx, uid, usegoods)
		if err != nil {
			return nil, err
		}
		if cnt == 0 {
			return nil, errf(fmt.Sprintf("not_enough_goods%d", usegoods))
		}
	}

	rate := combineRate[combineLevel+1]
	tmpRate := mtRand(1, 10000)
	success := 0
	if float64(tmpRate) < float64(rate)*100 {
		success = 1
	}

	newCombineLevel := combineLevel
	if success == 1 {
		if _, err := s.db.Exec(ctx, "update user_armors set combine_level=LEAST(?,combine_level+1) where sid=?", maxCombine, mainSid); err != nil {
			return nil, err
		}
		newCombineLevel = combineLevel + 1
		if newCombineLevel > maxCombine {
			newCombineLevel = maxCombine
		}
		// 备份（legacy @sql_query 静默）
		s.combineBackup(ctx, subSid1, usegoods, success, mainSid)
		s.combineBackup(ctx, subSid2, usegoods, success, mainSid)
		if _, err := s.db.Exec(ctx, "delete from user_tie_deify_attribute where sid in (?,?)", subSid1, subSid2); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "delete from user_armors where sid in (?,?)", subSid1, subSid2); err != nil {
			return nil, err
		}
	}
	if success == 0 && goodsFlag == 0 {
		s.combineBackup(ctx, mainSid, usegoods, success, mainSid)
		s.combineBackup(ctx, subSid1, usegoods, success, mainSid)
		s.combineBackup(ctx, subSid2, usegoods, success, mainSid)
		if _, err := s.db.Exec(ctx, "delete from user_tie_deify_attribute where sid in (?,?,?)", subSid1, subSid2, mainSid); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "delete from user_armors where sid in (?,?,?)", subSid1, subSid2, mainSid); err != nil {
			return nil, err
		}
	}
	// 成功 ≥3 级公告（sendSysInform）与 2013-09 活动奖励时间窗恒不触发 → 省略

	if err := s.addGoods(ctx, uid, mustGoods, -1, 0); err != nil {
		return nil, err
	}
	if err := s.addGoods(ctx, uid, usegoods, -1, 0); err != nil { // 怪癖：goodsFlag=0 时 gid=-11
		return nil, err
	}
	return &CombineResult{Success: success, GoodsFlag: goodsFlag, NewCombineLevel: newCombineLevel}, nil
}

// combineBackup 对齐 legacy 备份 insert...select（@ 静默失败）。
func (s *Service) combineBackup(ctx context.Context, sid, usegoods, success, mainsid int) {
	_, _ = s.db.Exec(ctx, `insert into log_armor_combine
		(sid, user_id, armorid, hp, hp_max, hid, strong_level, strong_value, embed_pearls, embed_holes,
		 deified, active_special, strong_times, combine_level, usegoods, success, mainsid, time)
		select sid, user_id, armorid, hp, hp_max, hid, strong_level, strong_value, embed_pearls, embed_holes,
			deified, active_special, strong_times, combine_level, ?, ?, ?, unix_timestamp()
		from user_armors where sid=?`,
		strconv.Itoa(usegoods), success, mainsid, sid)
}
