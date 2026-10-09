package hero

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"rxsg/backend/internal/model"
)

// expr.go 1:1 复刻 legacy 将领历练：
//   HeroFunc.php beginExprHero(1992) / cancelHeroExpr(2073) / fasterHeroExpr(2084)
//   HeroExpr.php ordinaryHeroExpr(17) / finishHeroExpr(42) / getHeroBaseExp(310) /
//   addHerolevels(101) / baseEvents/heroFightEvents/commonExprEvents 奇遇调度。
//
// 关键公式（记忆硬约束）：
//   - 历练基础经验 getHeroBaseExp：修身(type1) hours*level*6000 + rand(1,1000)；
//     闯荡(type2) hours*level*12000 + rand(1,1000)。
//   - 闭关突破 addHerolevels：120→125(+5%)、125→130(+10%)、130→135(+15%)、135→(+18%不升级)。
//
// 原版怪癖 1:1 保留：
//   - beginExprHero 时间窗口拦截（expstarttime=1301043600, expsendtime=1301472000）；
//   - 查武将只按 hid（不校验 uid/cid），不存在时 heroState/heroLevel/heroType 均按 null：
//     null!=0 为 false → 跳过 not_kong；null==1000 为 false → 跳过 cannot_expr；heroLevel null→0；
//   - maxHeroCount=2，有巡查令(buftype100)→5；heroCount==5 throw，>=maxHeroCount return[1,toomany]；
//   - need_gold=heroLevel*hours*hour_gold、need_money=hours*hour_money；
//   - 扣城金 `gold=gold-need_gold`（无 GREATEST，可为负）；addMoney 扣 -(carrymoney+need_money) type120；
//   - cancelHeroExpr：endtime=if(now-starttime>hours*1800, endtime, 2*now-starttime)、state=1；hero state 10→11；
//   - fasterHeroExpr 通关文书(143)：reduce=floor((end-cur)*0.3)，<1800 则取 1800；
//     急召令(144)：exp_add=intval(item.exp_add)+10*rand(hours,2*hours)，其中 item.exp_add 来自
//     cancelHeroExpr 未存的字段——legacy fasterHeroExpr 读 sys_hero_expr.exp_add 列（该列由
//     cancelHeroExpr 的 select 别名产生、并非落库列），实际恒 null→intval 0。此处按 legacy 行为：
//     急召令读 hero_exprs 无 exp_add 列 → 视作 0。
//
// 降级：sys_user_state.vacend（休假）无表→不拦截；cfg_hero_expr_reward/sys_hero_expr_reward 无种子→
// 奇遇奖励省略；sys_king_expr 君主历练无表→MoranchHeroExpr 省略；sys_report/sys_inform 广播省略。

// 历练时间窗口（beginExprHero:1995-1996，绝对 Unix 秒，早已过期→正常不拦截）。
const (
	exprStartTime = 1301043600
	exprSendTime  = 1301472000
)

// StartExpr 对齐 beginExprHero（param: hid,cid,exprType,hours,carrymoney）。
func (s *Service) StartExpr(ctx context.Context, uid, cid, hid, exprType, hours, carrymoney int) (*Info, error) {
	now := timeNow(ctx, s)
	if now <= exprSendTime && now >= exprStartTime {
		return nil, errHero(msgExpUpdateAlert)
	}
	// sys_user_state.vacend 无表 → 休假拦截省略。
	if hours < 1 {
		return nil, errHero(msgWaiguaInvalid)
	}
	// legacy 只按 hid 查询（怪癖：不校验 uid/cid）。
	hero, err := s.heroRowByID(ctx, hid)
	if err != nil {
		return nil, err
	}
	// 无行时按 null 处理：state/level/herotype 全 0。
	var heroState, heroLevel, heroType int
	if hero != nil {
		heroState = model.Int(hero, "state")
		heroLevel = model.Int(hero, "level")
		heroType = model.Int(hero, "hero_type")
	}
	if heroState != 0 {
		return nil, errHero(msgHeroNotKong)
	}
	if heroType == 1000 {
		return nil, errHero(msgCannotExpr)
	}
	maxHeroCount := 2
	hasXunCha, err := s.bufferActive(ctx, uid, 100)
	if err != nil {
		return nil, err
	}
	if hasXunCha {
		maxHeroCount = 5
	}
	heroCount, err := s.cellInt64Zero(ctx, "select count(1) from hero_exprs where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	if heroCount == 5 {
		return nil, errHero(msgExprCountMax)
	}
	if heroCount >= int64(maxHeroCount) {
		// legacy return [1, toomany]——非错误，Go 以 Info + 提示字段返回。
		info, err := s.Info(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		info.Toomany = msgToomanyHeroExpr
		return info, nil
	}
	// cfg_hero_expr_types 行。
	trow, err := s.db.FetchOne(ctx, "select * from cfg_hero_expr_types where `type`=?", exprType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errHero("waigua gua le!")
	}
	if err != nil {
		return nil, err
	}
	hourMoney := model.Int64(trow, "hour_money")
	hourGold := model.Int64(trow, "hour_gold")
	minHour := model.Int(trow, "min_hour")
	maxHour := model.Int(trow, "max_hour")
	typeName := model.Str(trow, "name")
	needMoney := int64(hours) * hourMoney
	needGold := int64(heroLevel) * int64(hours) * hourGold
	if hours > maxHour || hours < minHour {
		return nil, errHero(fmt.Sprintf(msgExprTimeError, typeName, minHour, maxHour))
	}
	cityGold, err := s.cityGold(ctx, cid)
	if err != nil {
		return nil, err
	}
	if cityGold < needGold {
		return nil, errHero(msgExprNotEnoughGold)
	}
	haveMoney, err := s.userMoney(ctx, uid)
	if err != nil {
		return nil, err
	}
	if haveMoney < int64(carrymoney)+needMoney {
		return nil, errHero(msgExprNotEnoughMoney)
	}
	if carrymoney < 0 {
		return nil, errHero(msgExprNotEnoughMoney)
	}
	// 扣城金（无 GREATEST，legacy 原样）。
	if _, err := s.db.Exec(ctx, "update city_resources set gold=gold-? where city_id=?", needGold, cid); err != nil {
		return nil, err
	}
	// addMoney(-(carrymoney+need_money), 120)。
	if err := s.addMoney(ctx, uid, -(int64(carrymoney) + needMoney), 120); err != nil {
		return nil, err
	}
	// logUserAction(21) 属 M8，未接线。
	if _, err := s.db.Exec(ctx, `insert into hero_exprs
		(user_id, city_id, hero_id, expr_type, hours, state, started_at, end_at, carrymoney, acc_times)
		values (?,?,?,?,?,0,?,?,?,0)`,
		uid, cid, hid, exprType, hours, now, now+int64(hours)*3600, carrymoney); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update heroes set state=10 where id=?", hid); err != nil {
		return nil, err
	}
	// completeTaskWithTaskid(312) 属 M8，未接线。
	return s.Info(ctx, uid, cid)
}

// CancelExpr 对齐 cancelHeroExpr。
// legacy 先 select exp_add/hours 预览（该值不落库、随后结算也不读取）→ Go 省略该查询，
// 仅做存在性检查（无行 return）。
func (s *Service) CancelExpr(ctx context.Context, uid, cid, hid int) (*Info, error) {
	affected, err := s.db.Exec(ctx, `update hero_exprs set state=1,
		end_at=if(unix_timestamp()-started_at > hours*1800, end_at, 2*unix_timestamp()-started_at)
		where hero_id=?`, hid)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		// legacy 无行 return（void）——logActionCountback(21) 不执行。
		return s.Info(ctx, uid, cid)
	}
	// logActionCountback(21) 属 M8，未接线。
	if _, err := s.db.Exec(ctx, "update heroes set state=11 where id=? and state=10", hid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// FasterExpr 对齐 fasterHeroExpr（143 通关文书 / 144 急召令）。
func (s *Service) FasterExpr(ctx context.Context, uid, cid, hid int) (*Info, error) {
	item, err := s.db.FetchOne(ctx, "select * from hero_exprs where hero_id=? limit 1", hid)
	if errors.Is(err, sql.ErrNoRows) {
		return s.Info(ctx, uid, cid)
	}
	if err != nil {
		return nil, err
	}
	const minReduceTime = 1800         // 最小缩短30分钟
	if model.Int(item, "state") == 0 { // 通关文书
		ok, err := s.checkGoods(ctx, uid, 143)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errHero("not_enough_goods143")
		}
		now := timeNow(ctx, s)
		oldEnd := model.Int64(item, "end_at")
		reduce := int64(math.Floor(float64(oldEnd-now) * 0.3))
		if reduce < minReduceTime {
			reduce = minReduceTime
		}
		newEnd := now + (oldEnd - now - reduce)
		if _, err := s.db.Exec(ctx, "update hero_exprs set end_at=?, acc_times=acc_times+1 where hero_id=?", newEnd, hid); err != nil {
			return nil, err
		}
		if err := s.reduceGoods(ctx, uid, 143, 1); err != nil {
			return nil, err
		}
	} else { // 急召令
		ok, err := s.checkGoods(ctx, uid, 144)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errHero("not_enough_goods144")
		}
		carrymoney := model.Int64(item, "carrymoney")
		if carrymoney > 0 {
			if err := s.addMoney(ctx, uid, carrymoney, 121); err != nil {
				return nil, err
			}
		}
		if err := s.reduceGoods(ctx, uid, 144, 1); err != nil {
			return nil, err
		}
		hours := int(model.Int64(item, "hours"))
		// legacy intval(item.exp_add)：sys_hero_expr 无 exp_add 落库列 → 恒 0。
		expAdd := 0 + 10*randInclusive(hours, 2*hours)
		if _, err := s.db.Exec(ctx, "delete from hero_exprs where hero_id=?", hid); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "update heroes set state=0, exp=exp+? where id=?", expAdd, hid); err != nil {
			return nil, err
		}
	}
	return s.Info(ctx, uid, cid)
}

// Settle 对齐 HeroExpr.php ordinaryHeroExpr：结算到期的历练（state 0/1 且 end_at<now）。
// 惰性结算（无 cron），每次 Info/Upgrade/StartExpr 前触发。
func (s *Service) Settle(ctx context.Context, cid int) error {
	now := timeNow(ctx, s)
	rows, err := s.db.FetchRows(ctx, `select * from hero_exprs
		where city_id=? and end_at<? and state in (0,1) order by end_at`, cid, now)
	if err != nil {
		return err
	}
	for _, e := range rows {
		hid := model.Int(e, "hero_id")
		state := model.Int(e, "state")
		id := model.Int(e, "id")
		if state == 0 { // 正常结束
			if err := s.finishExpr(ctx, e); err != nil {
				return err
			}
		} else if state == 1 { // 取消后到期
			uid := model.Int(e, "user_id")
			carrymoney := model.Int64(e, "carrymoney")
			if err := s.addMoney(ctx, uid, carrymoney, 121); err != nil {
				return err
			}
			hours := int(model.Int64(e, "hours"))
			// exp_add 列 legacy 未落库→0；+10*randomRange(hours,2*hours)
			expAdd := 0 + 10*randRange(hours, 2*hours)
			if expAdd > 0 {
				if _, err := s.db.Exec(ctx, "update heroes set exp=exp+? where id=?", expAdd, hid); err != nil {
					return err
				}
			}
		}
		if _, err := s.db.Exec(ctx, "delete from hero_exprs where id=?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", hid); err != nil {
			return err
		}
	}
	// MoranchHeroExpr（sys_king_expr 君主历练）无表 → 省略。
	return nil
}

// finishExpr 对齐 HeroExpr.php finishHeroExpr：加基础经验 + 闭关突破判定。
func (s *Service) finishExpr(ctx context.Context, e map[string]any) error {
	hid := model.Int(e, "hero_id")
	hours := model.Int(e, "hours")
	typ := model.Int(e, "expr_type")
	hero, err := s.db.FetchOne(ctx, "select * from heroes where id=? limit 1", hid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	expAdd := getHeroBaseExp(typ, model.Int(hero, "level"), hours)
	if _, err := s.db.Exec(ctx, "update heroes set exp=exp+? where id=?", expAdd, hid); err != nil {
		return err
	}
	// 闭关突破（HeroExpr.php:54-85）：tec_level(sys_technic tid29) 无表→恒 0。
	actrate := randInclusive(0, 27)
	if actrate%5 == 0 {
		// baseEvents 奇遇——cfg_hero_expr_reward 无种子→省略。
	} else if actrate%7 == 0 {
		// heroFightEvents——省略。
	} else if actrate%12 == 0 {
		// commonExprEvents——省略。
	} else {
		level := model.Int(hero, "level")
		tecLevel := 0 // sys_technic tid=29 无表
		if level > 119 && tecLevel > 0 {
			if level < 125 {
				level = 120
			}
			if actrate != 6 && actrate != 17 {
				for i := 0; i < tecLevel; i++ {
					actrate = randInclusive(i, 27-tecLevel)
					if actrate == 6 || actrate == 17 {
						break
					}
				}
			}
			if level == 120 && tecLevel < 1 {
				actrate = 0
			}
			if level == 125 && tecLevel < 2 {
				actrate = 0
			}
			if level == 130 && tecLevel < 3 {
				actrate = 0
			}
			npcID := model.Int(hero, "npc_id")
			heroType := model.Int(hero, "hero_type")
			if npcID > 0 || heroType == 1000 {
				rate := 0
				if heroType == 1000 {
					rate = 1
				}
				if exprBigHids[hid] {
					rate = 1
				} else {
					if randInclusive(0, 3) == 1 {
						rate = 1
					}
				}
				if rate == 1 && (actrate == 6 || actrate == 17) {
					if err := s.addHeroLevels(ctx, hid, level); err != nil {
						return err
					}
				}
			}
		}
	}
	// sendReport(RT_HEROEXPR_END) / sys_inform 广播无表 → 省略。
	return nil
}

// getHeroBaseExp 对齐 HeroExpr.php:310（记忆硬约束公式）。
func getHeroBaseExp(typ, level, hours int) int64 {
	switch typ {
	case 1: // 修身养性
		return int64(hours*level*6000 + randInclusive(1, 1000))
	case 2: // 闯荡江湖
		return int64(hours*level*12000 + randInclusive(1, 1000))
	default:
		return 0
	}
}

// addHeroLevels 对齐 HeroExpr.php:101 addHerolevels（闭关突破）。
func (s *Service) addHeroLevels(ctx context.Context, hid, level int) error {
	var attValue float64
	newLevel := level
	if level == 135 {
		attValue = 0.18
	} else {
		switch level {
		case 120:
			attValue = 0.05
			newLevel = 125
		case 125:
			attValue = 0.10
			newLevel = 130
		case 130:
			attValue = 0.15
			newLevel = 135
		}
		if level != 135 {
			if _, err := s.db.Exec(ctx, "update heroes set level=? where id=?", newLevel, hid); err != nil {
				return err
			}
		}
	}
	hero, err := s.db.FetchOne(ctx, "select * from heroes where id=? limit 1", hid)
	if err != nil {
		return err
	}
	uid := model.Int(hero, "user_id")
	ba := floorPct(model.Int(hero, "bravery_base"), attValue)
	wi := floorPct(model.Int(hero, "wisdom_base"), attValue)
	af := floorPct(model.Int(hero, "affairs_base"), attValue)
	co := floorPct(model.Int(hero, "command_base"), attValue)
	attackAddOn := newLevel
	defenceAddOn := newLevel
	// insertHeroBaseAdd（GREATEST(0,…) 夹底）。
	if err := s.insertHeroBaseAdds(ctx, hid, uid, ba, wi, af, co, attackAddOn, defenceAddOn); err != nil {
		return err
	}
	// sendSysInform 广播无表 → 省略。
	return nil
}
