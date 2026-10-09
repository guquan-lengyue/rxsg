package armor

import (
	"context"
	"fmt"
	"strings"

	"rxsg/backend/internal/model"
)

// strong.go 复刻 legacy EquipmentFunc.php doStrong(178) 全链：
//   strongLimit(131) / reCalculateSuccess(569) / getBestQuality(472) / getProperty(492) /
//   pValue(1407) / fitPValue(1402) / probability(1396)。
// 参数序（legacy 逐位 array_shift）：cid, gid1, good1Count, gid2, good2Count, sid, is_zuoji。
// 降级：cfg_act 无表 → inact 恒 false（succ_add 无 +3、reCalculateSuccess 直通、
//   保底 incount>49/79 分支仍按原逻辑生效）；sendSysInform/completeTask/logUserAction 属 M8/M9 未接线；
//   strongActOnce（2011-12 坐骑强化活动）与 checkAndDoStrongArmorAct 时间窗恒不触发。
// 怪癖保留：
//   - 行缺失返回 ret=[0,文案]（非 throw）；
//   - 强化 1 级失败降级 before_level=0 → cfg_strong_probability 无 0 级行（legacy 同样无，
//     Chaijie 位置索引 $probability[i-1] 依赖首行为 1 级）→ update 语句含空值执行失败（静默），
//     装备等级不变但 ret=2、日志 endlevel=startlevel-1；
//   - startlevel>15 时材料检查两个 if 均不命中（无 16 级材料校验）；
//   - 失败且 good2Count==1（乾坤/师皇）恒 ret3 无损；
//   - gid1/gid2 与 is_zuoji 不匹配不校验（如普通装用伯乐符仍放行并扣 212）。

// StrongResult doStrong 返回（legacy ret 数组语义化）。
type StrongResult struct {
	Started     bool   `json:"started"`      // ret[0]==1（确实执行了强化）
	Outcome     int    `json:"outcome"`      // 0成功 1归零 2降级 3完好/无损
	StrongValue int    `json:"strong_value"` // 结算后 strong_value
	EndLevel    int    `json:"end_level"`    // 结算后等级（日志口径）
	BestQuality string `json:"best_quality"` // 成功时的极品属性串（失败为空，legacy undefined→null）
	Msg         string `json:"msg"`          // 行缺失时的文案（Started=false）
}

// DoStrong 对齐 EquipmentFunc.php:178。
func (s *Service) DoStrong(ctx context.Context, uid, cid, gid1, good1Count, gid2, good2Count, sid, isZuoji int) (*StrongResult, error) {
	if good1Count > 1 || good1Count < 0 {
		return nil, errf(msgWaiguaForbidden)
	}
	if good2Count > 1 || good2Count < 0 {
		return nil, errf(msgWaiguaForbidden)
	}
	if gid1 != gidBoLe && gid1 != gidTianGong {
		return nil, errf(msgWrongItem)
	}
	if gid2 != gidShiHuang && gid2 != gidQianKun {
		return nil, errf(msgWrongItem)
	}

	armor, err := s.fetchOneOrNil(ctx, `select u.*, c.part, c.type, c.name
		from user_armors u left join cfg_armor c on c.id=u.armorid where u.sid=? and u.user_id=?`, sid, uid)
	if err != nil {
		return nil, err
	}
	if armor == nil { // 怪癖：返回 ret=[0,文案] 而非 throw
		msg := msgNoSuchArmor
		if isZuoji == 1 {
			msg = msgNoSuchHorse
		}
		return &StrongResult{Msg: msg}, nil
	}
	if model.Int(armor, "hid") != 0 {
		return nil, errf(msgArmorInHero)
	}
	part := model.Int(armor, "part")
	if (part == partMount && isZuoji != 1) || (part != partMount && isZuoji == 1) {
		return nil, errf(msgDataException)
	}

	if good1Count == 1 {
		ok, err := s.checkGoods(ctx, uid, gid1)
		if err != nil {
			return nil, err
		}
		if !ok {
			if isZuoji == 1 {
				return nil, errf("not_enough_goods212")
			}
			return nil, errf("not_enough_goods203")
		}
	}
	if good2Count == 1 {
		ok, err := s.checkGoods(ctx, uid, gid2)
		if err != nil {
			return nil, err
		}
		if !ok {
			if isZuoji == 1 {
				return nil, errf("not_enough_goods213")
			}
			return nil, errf("not_enough_goods204")
		}
	}

	startLevel := model.Int(armor, "strong_level")
	strongValue := model.Int(armor, "strong_value")
	nLevel := startLevel + 1

	if isZuoji == 1 {
		if startLevel >= 0 && startLevel <= 9 {
			ok, err := s.checkGoods(ctx, uid, gidLingTong)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errf(msgNoTLGC)
			}
		} else if startLevel >= 10 && startLevel <= 15 {
			ok, err := s.checkGoods(ctx, uid, gidHighLTGC)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errf(msgNoGJLTGC)
			}
		}
	} else {
		isZuoji = 0 // legacy：$is_zuoji=0 归一化
		if startLevel >= 0 && startLevel <= 9 {
			ok, err := s.checkGoods(ctx, uid, gidStrong)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errf(msgNoStrongPearl)
			}
		}
		if startLevel >= 10 && startLevel <= 15 {
			ok, err := s.checkGoods(ctx, uid, gidHighStrong)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errf(msgNoHighStrong)
			}
		}
	}

	if err := s.strongLimit(ctx, armor, cid); err != nil {
		return nil, err
	}
	// legacy 重复校验（"前面不是判断过了吗"——原注释，1:1 保留）
	if isZuoji == 1 {
		if part != partMount {
			return nil, errf(msgNotZuoji)
		}
	} else if part == partMount {
		return nil, errf(msgIsZuoji)
	}

	// inact：cfg_act 表无 → 恒 false（活动 +3% 与保底重算不生效）
	incount, err := cellInt(ctx, s.db,
		"select count(*) from log_armor_strong where sid=? and user_id=? and startlevel=?", sid, uid, startLevel)
	if err != nil {
		return nil, err
	}

	next, err := s.fetchOneOrNil(ctx, "select * from cfg_strong_probability where level=? limit 1", nLevel)
	if err != nil {
		return nil, err
	}
	if next == nil {
		return nil, errf(msgCannotStrong)
	}

	succAdd := good1Count * 7 // 天工符/伯乐符 +7%
	// inact 恒 false → 无 +3

	isSucc := false
	if (isZuoji == 1 && nLevel <= 10) || isZuoji != 1 {
		randRate := mtRand(1, 10000)
		sucVal := float64(atoi(model.Str(next, "suc_value"))) * (1 + float64(succAdd)/100)
		isSucc = float64(randRate) <= sucVal*100 || incount > 49
	} else { // 坐骑 11+ 级独立概率
		randRate := mtRand(1, 10000)
		sucVal := float64(mountHighRate[nLevel]) * (1 + float64(succAdd)/100)
		isSucc = float64(randRate) <= sucVal*100 || incount > 79
	}
	// reCalculateSuccess：inact 恒 false → 直通（保底表见 reCalculateSuccess 注释）

	res := &StrongResult{Started: true}
	bestQuality := ""
	endLevel := startLevel

	if isSucc {
		bestQuality = model.Str(armor, "best_quality")
		if bestQuality == "" {
			bq, err := s.getBestQuality(ctx, model.Int(next, "xilian_rate"), model.Int(armor, "type"))
			if err != nil {
				return nil, err
			}
			bestQuality = bq
		}
		newSV := model.Int(next, "strong_value")
		newLevel := model.Int(next, "level")
		if bestQuality != "" {
			if _, err := s.db.Exec(ctx, `update user_armors set strong_times=0, strong_value=?, strong_level=?, best_quality=?
				where user_id=? and sid=?`, newSV, newLevel, bestQuality, uid, sid); err != nil {
				return nil, err
			}
		} else {
			if _, err := s.db.Exec(ctx, `update user_armors set strong_times=0, strong_value=?, strong_level=?
				where user_id=? and sid=?`, newSV, newLevel, uid, sid); err != nil {
				return nil, err
			}
		}
		strongValue = newSV
		endLevel = startLevel + 1
		res.Outcome = 0
		// strongActOnce（2011-12 活动窗）、level>=7 公告、completeTask(530)/103803、
		// checkAndDoStrongArmorAct 属活动/M8 → 未接线
	} else {
		zeroValue := atoi(model.Str(next, "zero_value"))
		degradeValue := atoi(model.Str(next, "degrade_value"))
		intactValue := atoi(model.Str(next, "intact_value"))
		p := mtRand(1, 100)
		if _, err := s.db.Exec(ctx, "update user_armors set strong_times=strong_times+1 where user_id=? and sid=?", uid, sid); err != nil {
			return nil, err
		}
		switch {
		case good2Count == 0 && fitPValue(p, 1, zeroValue): // 归零
			if _, err := s.db.Exec(ctx, "update user_armors set strong_value=0, strong_level=0 where user_id=? and sid=?", uid, sid); err != nil {
				return nil, err
			}
			strongValue = 0
			endLevel = 0
			res.Outcome = 1
		case good2Count == 0 && fitPValue(p, zeroValue+1, zeroValue+degradeValue): // 降级
			beforeLevel := atoi(model.Str(next, "level")) - 2
			before, err := s.fetchOneOrNil(ctx, "select * from cfg_strong_probability where level=? limit 1", beforeLevel)
			if err != nil {
				return nil, err
			}
			if before != nil {
				bSV := model.Int(before, "strong_value")
				bLevel := model.Int(before, "level")
				if _, err := s.db.Exec(ctx, "update user_armors set strong_value=?, strong_level=? where user_id=? and sid=?",
					bSV, bLevel, uid, sid); err != nil {
					return nil, err
				}
				strongValue = bSV
			} else {
				// 怪癖：0 级行不存在 → legacy SQL 含空值执行失败（静默），装备等级不变；
				// $strong_value=null→0（返回与日志口径）
				strongValue = 0
			}
			endLevel = startLevel - 1
			res.Outcome = 2
		case good2Count == 0 && fitPValue(p, zeroValue+degradeValue+1, zeroValue+degradeValue+intactValue): // 完好
			res.Outcome = 3
		default: // 保护符（乾坤/师皇）恒无损；区间未覆盖时 legacy 亦不改等级
			res.Outcome = 3
		}
	}

	// 材料扣减（addGoods 带符号 type0）
	if good1Count == 1 {
		if err := s.addGoods(ctx, uid, gid1, -1, 0); err != nil {
			return nil, err
		}
	}
	if good2Count == 1 {
		if err := s.addGoods(ctx, uid, gid2, -1, 0); err != nil {
			return nil, err
		}
	}
	if isZuoji == 1 {
		m := gidLingTong
		if startLevel > 9 {
			m = gidHighLTGC
		}
		if err := s.addGoods(ctx, uid, m, -1, 0); err != nil {
			return nil, err
		}
	} else {
		m := gidStrong
		if startLevel > 9 {
			m = gidHighStrong
		}
		if err := s.addGoods(ctx, uid, m, -1, 0); err != nil {
			return nil, err
		}
	}

	armorid := model.Int(armor, "armorid")
	usegoods := fmt.Sprintf("%d,%d,%d,%d", gid1, good1Count, gid2, good2Count)
	isSuccInt := 0
	if isSucc {
		isSuccInt = 1
	}
	if _, err := s.db.Exec(ctx, `insert into log_armor_strong
		(user_id, sid, armorid, is_zuoji, startlevel, endlevel, usegoods, success, time)
		values (?,?,?,?,?,?,?,?,unix_timestamp())`,
		uid, sid, armorid, isZuoji, startLevel, endLevel, usegoods, isSuccInt); err != nil {
		return nil, err
	}

	res.StrongValue = strongValue
	res.EndLevel = endLevel
	res.BestQuality = bestQuality
	return res, nil
}

// strongLimit 对齐 EquipmentFunc.php:131。
func (s *Service) strongLimit(ctx context.Context, armor map[string]any, cid int) error {
	part := model.Int(armor, "part")
	typ := model.Int(armor, "type")
	strongLevel := model.Int(armor, "strong_level")

	if strongLevel >= maxStrongLevel {
		return errf(msgStrongLevelLimit)
	}
	tid := tidBlacksmith
	info := msgStrongTechSmith
	if part == partMount {
		tid = tidHorse
		info = msgStrongTechBarn
	}
	techLevel, err := cellInt(ctx, s.db, "select level from city_technics where city_id=? and technic_id=? limit 1", cid, tid)
	if err != nil {
		return err
	}
	if strongLevel <= 9 { // 科技需比当前等级大 1
		if techLevel == 0 || int64(strongLevel) >= techLevel {
			return errf(fmt.Sprintf(info, strongLevel+1))
		}
	}
	if strongLevel >= 10 { // 科技需 ≥10
		if techLevel == 0 || techLevel < 10 {
			return errf(fmt.Sprintf(info, 10))
		}
	}
	if (typ == 1 && strongLevel >= 3) || (typ == 2 && strongLevel >= 6) || (typ == 3 && strongLevel >= 9) {
		return errf(fmt.Sprintf(msgStrongLimitFmt, strongLimitArray[typ], strongLevel))
	}
	if part != partMount && typ == 4 && strongLevel >= 10 && model.Int(armor, "combine_level") <= 2 {
		return errf(msgBlueStrongLimit)
	}
	return nil
}

// reCalculateSuccess 对齐 EquipmentFunc.php:569。
// 仅活动期（inact）生效；cfg_act 无表 → inact 恒 false → 本函数直通返回 success。
// maxtimes 表（1-15）：1,2,3,5,7,14,20,30,40,50,5,10,15,30,50；strong_times>=maxtimes-1 → true。
func reCalculateSuccessMaxtimes(level int) int {
	switch level {
	case 1, 2, 3:
		return level
	case 4:
		return 5
	case 5:
		return 7
	case 6:
		return 14
	case 7:
		return 20
	case 8:
		return 30
	case 9:
		return 40
	case 10:
		return 50
	case 11:
		return 5
	case 12:
		return 10
	case 13:
		return 15
	case 14:
		return 30
	case 15:
		return 50
	}
	return 1000
}

var _ = reCalculateSuccessMaxtimes // inact 恒 false，保底表保留备查（对照 PHP:569-615）

// getBestQuality 对齐 EquipmentFunc.php:472。
// 返回 "rand_id,rand_pro,value,name,cfg_property[rand_pro]"；未命中概率返回 ""。
func (s *Service) getBestQuality(ctx context.Context, rate, typ int) (string, error) {
	if mtRand(1, 100) > rate {
		return "", nil
	}
	sumRate, err := cellInt(ctx, s.db, "select count(1) from cfg_xilian")
	if err != nil {
		return "", err
	}
	if sumRate <= 0 {
		return "", nil
	}
	randID := mtRand(1, int(sumRate))
	xilian, err := s.fetchOneOrNil(ctx, "select * from cfg_xilian where id=? limit 1", randID)
	if err != nil {
		return "", err
	}
	if xilian == nil {
		return "", nil
	}
	propertyArr := strings.Split(model.Str(xilian, "property"), ",")
	pro, ok := getProperty(propertyArr)
	if !ok {
		return "", nil
	}
	cfgType, err := s.fetchOneOrNil(ctx, "select * from cfg_xilian_type where type=? limit 1", typ)
	if err != nil {
		return "", err
	}
	minAdd, maxAdd := 0, 0
	if cfgType != nil {
		minAdd = model.Int(cfgType, "minAdd")
		maxAdd = model.Int(cfgType, "maxAdd")
	}
	value := mtRand(minAdd, maxAdd)
	return fmt.Sprintf("%d,%d,%d,%s,%s", randID, pro, value, model.Str(xilian, "name"), cfgPropertyName(pro)), nil
}

// cfg_property（EquipmentFunc.php:483）。
var cfgPropertyNames = []string{"生命", "攻击", "防御", "射程", "速度", "负重"}

func cfgPropertyName(pro int) string {
	if pro >= 0 && pro < len(cfgPropertyNames) {
		return cfgPropertyNames[pro]
	}
	return ""
}

// getProperty 对齐 EquipmentFunc.php:492（i=1 步长 2 权重求和后加权随机，返回 propertyArr[i-1]）。
func getProperty(propertyArr []string) (int, bool) {
	length := len(propertyArr)
	sumRate := 0
	for i := 1; i < length; i += 2 {
		sumRate += atoi(propertyArr[i])
	}
	if sumRate <= 0 {
		return 0, false
	}
	randRate := mtRand(1, sumRate)
	sumRate = 0
	for i := 1; i < length; i += 2 {
		sumRate += atoi(propertyArr[i])
		if randRate <= sumRate {
			return atoi(propertyArr[i-1]), true
		}
	}
	return 0, false
}

// fitPValue 对齐 EquipmentFunc.php:1402（闭区间）。
func fitPValue(p, min, max int) bool {
	return min <= p && p <= max
}
