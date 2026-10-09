package pk

// service.go 1:1 复刻 legacy server/game/PKFunc.php 的"单机 PK 征战"(campaign) 部分。
// 入口函数对照（文件:行号）：
//   initHeroState(9) / PKDamage(40) / isTrigger(48) / battleComute(60) / startUserPK(151)
//   checkFirstPass(215) / getPkGid(227) / sendUserReward(240) / checkUserPassLevel(321) / checkUserLevel(351)
//   getOneBattleRet(371) / getPkFirstReward(424) / loadCampaignInitData(452) / loadPKRewardRank(497)
//   regetCampaignMaxData(537) / buyJunlingFunc(548) / getAllHeroByUid(607)
//
// 原版怪癖 1:1 保留：
//   - startUserPK(163) 先手判定 `$attackHero['speed'] >= $resistHero['speed']`（相等时攻方先手）；
//   - startUserPK(175-196) 血量归零后按 `$X['hid'] == $attacker['hid']` 决定把"胜者"剩余能量回填
//     `energy=intval(blood/50)` 并 array_unshift 回队（攻守两侧共用同一段判断，顺序敏感）；
//   - checkFirstPass(216-223) 名次占用顺序：先 rank1，再需 `$uid != $uid1`，再需 `$uid != $uid1 && $uid != $uid2`
//     （空缺位按 rank1→2→3 依次占用，同一 uid 不重复占位）；
//   - sendUserReward(245-248) `mt_rand(1,10)!=3 → gid=0`（只有 1/10 概率掉征战奖品）；
//   - sendUserReward(269) 首通奖励条件 `$battleId==$passBattleId && $maxLevel==$tmpLevel+1 && $battleLevel==$maxLevel`
//     （即在最后 1 关通关时才触发，normal/special 分别以 battleId*100+levelId 编码进度）；
//   - getPkFirstReward(433) `time>1` 判定为"已领取"（time 初值 0，领取后写 unix_timestamp）；
//   - buyJunlingFunc(584) 军令上限 100，`$count > $maxBuyCount` 报错。
//
// 裁剪（保留判断结构，按"原版会返回 false/空"语义返回并注释）：
//   沙场（ShaChangFunc.php / ShaChang.php / getCrossShaChangInfo / joinCrossShaChang / getAllHeroList /
//     getCrossShaChangReward）——纯跨服（CROSS_ShaChang_* + CROSS_SIGN_KEY + server_guid 外部网关，无该服务）；
//   竞技场（holdBattleHeroInfo / loadUserBattleHero / loadArenaBattleInitData / loadRankBefore10 / loadBeforeUser10 /
//     startArenaChallenge / getArenaRankRewardInfo / getArenaRankReward / clearArenaCoolTime / getArenaReward /
//     refreshAssembleRankInfo / gotoSelectedRank / initUserAssemBleInfo / loadUserAssembleRecord / loadArenaUserData /
//     dealWithResult / updatePartUserRank / addAttackArenaReward / resetResistArenaReward / doCaulRewardCount /
//     parseRewardGood / parseWinChatInform / updateUserArenaChallenge / checkHero / holdBattleHeroBaseInfo / loadWinRecord）
//     ——玩家间排行 PvP，属用户明确排除范围；
//   sys_inform / sendSysInform / sendSysInformHere 全服公告广播；
//   sys_user_level（君主修为）——未在已完成模块建表 → checkUserLevel(flag=1) 按无表 → 0 降级；
//   checkDesignation/sendDesignation（称号）、updateBattleOpenState、cfg_things（parseAndAddReward type=2）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// RATE 对齐 PKFunc.php:4 `define('RATE', 50)`。
const RATE = 50

type Service struct {
	db *db.DB
	lk *lock.Locker

	mu  sync.Mutex
	rnd *rand.Rand
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB), rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// SetSeed 注入随机种子（集成测试可复现；1:1 逻辑不可注入 mt_rand）。
func (s *Service) SetSeed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rnd = rand.New(rand.NewSource(seed))
}

// randInt 对齐 PHP mt_rand($min,$max)（含端点）。
func (s *Service) randInt(min, max int) int {
	if max <= min {
		return min
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rnd.Intn(max-min+1) + min
}

// WithUserLock 包裹写操作（对齐 legacy lockUser/unlockUser）。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	err := s.lk.WithUserLock(ctx, uid, key, fn)
	if err != nil {
		if err.Error() == "lock_busy" {
			return errLegacy(msgServerBusy)
		}
		return err
	}
	return nil
}

// errLegacy 构造与 legacy throw new Exception 等价的错误响应（HTTP 400，code=pk_error）。
func errLegacy(msg string) error {
	return httpx.BadRequest("pk_error", msg)
}

// lang 文案（server/game/lang.php 逐字）。
const (
	msgInvalidParam   = "参数错误"                // useMojiaGoods.invalid_param
	msgNoAdvLijianfu  = "物品不够，请先购买"           // changeCityPosition.no_adv_lijianfu
	msgNoHeroInfo     = "请刷新并重新选择将领"          // heroPk.no_hero_info
	msgHeroLevelLow   = "当前君主将等级太低，不能参加此战役。"  // heroPk.hero_level_low
	msgUserLevelLow   = "当前修为等级太低，不能参加此战役。"   // heroPk.user_level_low
	msgCommandExc     = "你异常了!!!"             // sendCommand.command_exception
	msgXidianUnvalid  = "使用外挂将导致账号数据异常，后果自负！" // hero.xidian_unvalid
	msgHasGetReward   = "您已经领取过奖励"            // king.has_get_reward
	msgInvalidPayType = ""                    // buyGoods.invalid_pay_type（lang 键缺失→空串）
	msgInvalidAmount  = "购买数量无效。"             // buyGoods.invalid_amount
	msgNoEnoughYuan   = "你的元宝不足，请充值。"         // buyGoods.no_enough_YuanBao
	msgJunlingMax     = "您目前最多只能购买%d个军令"      // buyGoods.junling_max_count
	msgPassBattleInfo = "%s在%s战役获得%s"         // userPK.pass_battle_info
	msgServerBusy     = "服务器忙，请稍后再进行操作。"      // pacifyPeople.server_busy
)

// ── 底层工具 ──────────────────────────────────────────────────────────────

func (s *Service) cellInt(ctx context.Context, q string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

func (s *Service) cellStr(ctx context.Context, q string, args ...any) (string, error) {
	v, err := s.db.FetchCellString(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// addGoods 对齐 utils.php:971（gid=0 礼金 / gid=152 铜钱双记账怪癖）。
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
	if gid == 152 {
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

// reduceGoods 对齐 utils.php:938。
func (s *Service) reduceGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt <= 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, "insert into log_goods (user_id,gid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, gid, -cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update user_goods set `count`=GREATEST(0,`count`-?) where user_id=? and gid=?", cnt, uid, gid)
	return err
}

// checkGoods 对齐 utils.php:933。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	v, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	return v >= 1, err
}

// checkMoney 对齐 utils.php:1106。
func (s *Service) checkMoney(ctx context.Context, uid int, need int64) (bool, error) {
	v, err := s.cellInt(ctx, "select money from users where id=?", uid)
	if err != nil {
		return false, err
	}
	if v == 0 { // legacy empty
		return false, nil
	}
	return v >= need, nil
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

// addGift 对齐 utils.php:1135。
func (s *Service) addGift(ctx context.Context, uid int, gift int64, typ int) error {
	if gift == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, "insert into log_gifts (user_id,count,time,type) values (?,?,unix_timestamp(),?)", uid, gift, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", gift, uid)
	return err
}

// addArmor 对齐 utils.php:1067（逐件插入 user_armors；updateBattleOpenState/sendDesignation 裁剪）。
func (s *Service) addArmor(ctx context.Context, uid int, armor map[string]any, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	aid := model.Int(armor, "id")
	oriHP := model.Int(armor, "ori_hp_max")
	strongvalue, err := s.cellInt(ctx, "select strong_value from cfg_strong_probability where level=0 limit 1")
	if err != nil {
		return err
	}
	for i := int64(0); i < cnt; i++ {
		if _, err := s.db.Exec(ctx,
			"insert into user_armors (user_id,armorid,hp,hp_max,ori_hp_max,hid,strong_level,strong_value,combine_level) values (?,?,?,?,?,0,0,?,0)",
			uid, aid, oriHP*10, oriHP, oriHP, strongvalue); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, "insert into log_armor (user_id,armorid,count,time,type) values (?,?,?,unix_timestamp(),?)", uid, aid, cnt, typ)
	return err
}

// parseAndAddReward 对齐 utils.php:7 parseAndAddReward（type=2 物品依赖 cfg_things 无表 → 裁剪）。
func (s *Service) parseAndAddReward(ctx context.Context, uid int, reward string, logGoodsType, logArmorType, logThingsType, logMoneyType int) ([]map[string]any, error) {
	goods := strings.Split(reward, ",")
	goodcnt := atoiSafe(safeIdx(goods, 0))
	money := 0
	isYuanBao := false
	ret := []map[string]any{}
	for i := 1; i < goodcnt*3; i += 3 {
		if i+2 >= len(goods) {
			break
		}
		typ := atoiSafe(goods[i])
		gid := atoiSafe(goods[i+1])
		cnt := atoiSafe(goods[i+2])
		if typ == 0 {
			if gid == 0 {
				money += cnt
			} else if gid == -100 {
				money += cnt
				isYuanBao = true
			} else {
				if err := s.addGoods(ctx, uid, gid, int64(cnt), logGoodsType); err != nil {
					return nil, err
				}
			}
			good, err := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", gid)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if good == nil {
				good = map[string]any{}
			}
			good["count"] = cnt
			good["gtype"] = 0
			ret = append(ret, good)
		} else if typ == 1 {
			armor, err := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", gid)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if armor == nil {
				armor = map[string]any{}
			}
			armor["count"] = cnt
			armor["gtype"] = 1
			armor["hp"] = model.Int(armor, "ori_hp_max")
			armor["hp_max"] = model.Int(armor, "ori_hp_max")
			ret = append(ret, armor)
			if err := s.addArmor(ctx, uid, armor, int64(cnt), logArmorType); err != nil {
				return nil, err
			}
		} else if typ == 2 {
			// 裁剪：cfg_things / addThings（物品）——新库无 cfg_things 配置表。
			_ = logThingsType
		}
	}
	if money > 0 {
		if isYuanBao {
			if err := s.addMoney(ctx, uid, int64(money), logMoneyType); err != nil {
				return nil, err
			}
		} else {
			if err := s.addGift(ctx, uid, int64(money), logMoneyType); err != nil {
				return nil, err
			}
		}
	}
	return ret, nil
}

// ── initHeroState / PKDamage / isTrigger / battleComute（PKFunc.php:9-144）──

// initHeroState 对齐 initHeroState($hero)。
func initHeroState(hero map[string]any) map[string]any {
	st := map[string]any{}
	st["hid"] = model.Int(hero, "hid")
	st["sex"] = model.Int(hero, "sex")
	st["blood"] = maxInt(model.Int(hero, "energy")*RATE, 500)
	st["attackValue"] = maxInt(model.Int(hero, "bravery")*10, 10)
	st["defenceValue"] = maxInt(model.Int(hero, "wisdom")*10, 10)
	baoji := maxInt(model.Int(hero, "bravery"), 10)
	if baoji > 3000 {
		baoji = 3000
	}
	st["baoji"] = baoji
	poji := maxInt(model.Int(hero, "command"), 10)
	if poji > 3000 {
		poji = 3000
	}
	st["poji"] = poji
	gedang := maxInt(model.Int(hero, "affair"), 10)
	if gedang > 3000 {
		gedang = 3000
	}
	st["gedang"] = gedang
	shanbi := maxInt(model.Int(hero, "wisdom"), 10)
	if shanbi > 3000 {
		shanbi = 3000
	}
	st["shanbi"] = shanbi
	st["standIndex"] = hero["standIndex"]
	return st
}

// PKDamage 对齐 PKDamage：intval(attackValue * (1 - defenceValue/(defenceValue+2000)))。
func PKDamage(attackValue, defenceValue float64) int {
	return int(math.Trunc(attackValue * (1 - defenceValue/(defenceValue+2000))))
}

// isTrigger 对齐 isTrigger：mt_rand(1,20000) <= value。
func (s *Service) isTrigger(value int) bool {
	r := s.randInt(1, 20000)
	return value >= r
}

// battleComute 对齐 battleComute(&$attacker,&$resister,$num)：就地扣减 resister.blood。
func (s *Service) battleComute(attacker, resister map[string]any, num int) map[string]any {
	attckAttack := float64(model.Int(attacker, "attackValue"))
	resistBlood := model.Int(resister, "blood")
	resistDefence := float64(model.Int(resister, "defenceValue"))
	curRet := map[string]any{}

	isBoji := s.isTrigger(model.Int(attacker, "baoji"))
	isPoji := s.isTrigger(model.Int(attacker, "poji"))
	isShanbi := s.isTrigger(model.Int(resister, "shanbi"))
	isGedang := s.isTrigger(model.Int(resister, "gedang"))
	if isShanbi {
		isGedang = false // 两者不能同时出现
	}
	if isPoji {
		isBoji = false // 两者不能同时出现
	}

	var flag, attack, resist int
	switch {
	case isShanbi:
		attckAttack = 0
		if isPoji {
			flag, attack, resist = 1, 2, 2
		} else if isBoji {
			flag, attack, resist = 1, 1, 2
		} else {
			flag, attack, resist = 1, 0, 2
		}
	case isGedang:
		if isPoji {
			flag, attack, resist = 1, 2, 1
		} else if isBoji {
			attckAttack *= 0.75
			flag, attack, resist = 1, 1, 1
		} else {
			attckAttack *= 0.5
			flag, attack, resist = 1, 0, 1
		}
	default:
		if isPoji {
			attckAttack *= 2
			flag, attack, resist = 1, 2, 0
		} else if isBoji {
			attckAttack *= 1.5
			flag, attack, resist = 1, 1, 0
		} else {
			flag, attack, resist = 0, 0, 0
		}
	}

	damage := PKDamage(attckAttack, resistDefence)
	if damage < 1 {
		damage = 1
	}
	if resistBlood > damage {
		resistBlood -= damage
	} else {
		resistBlood = 0
	}
	resister["blood"] = resistBlood

	curRet["flag"] = flag
	curRet["attack"] = attack
	curRet["resist"] = resist
	curRet["damage"] = damage
	curRet["blood"] = resistBlood
	curRet["attackhid"] = attacker["hid"]
	curRet["resisthid"] = resister["hid"]
	curRet["attacksex"] = attacker["sex"]
	curRet["resistsex"] = resister["sex"]
	curRet["battleId"] = num
	curRet["attackStandIndex"] = attacker["standIndex"]
	curRet["resistStandIndex"] = resister["standIndex"]
	return curRet
}

// startUserPK 对齐 startUserPK($attackUser,$resistUser)：3v3 顺序对战。
func (s *Service) startUserPK(attackUser, resistUser []map[string]any) map[string]any {
	ret := map[string]any{}
	if len(attackUser) == 0 || len(resistUser) == 0 {
		return ret
	}
	attackQ := append([]map[string]any{}, attackUser...)
	resistQ := append([]map[string]any{}, resistUser...)
	report := [][]map[string]any{}
	totalBattle := 0

	for len(attackQ) > 0 && len(resistQ) > 0 {
		attackHero := attackQ[0]
		attackQ = attackQ[1:]
		resistHero := resistQ[0]
		resistQ = resistQ[1:]
		battleRet := []map[string]any{}
		if attackHero == nil || resistHero == nil {
			break
		}
		var attacker, resister map[string]any
		if model.Int(attackHero, "speed") >= model.Int(resistHero, "speed") {
			attacker = initHeroState(attackHero)
			resister = initHeroState(resistHero)
		} else {
			attacker = initHeroState(resistHero)
			resister = initHeroState(attackHero)
		}

		for model.Int(attacker, "blood") > 0 && model.Int(resister, "blood") > 0 {
			totalBattle++
			battleRet = append(battleRet, s.battleComute(attacker, resister, totalBattle))
			if model.Int(resister, "blood") == 0 {
				if model.Int(attackHero, "hid") == model.Int(attacker, "hid") {
					attackHero["energy"] = model.Int(attacker, "blood") / RATE
					attackQ = append([]map[string]any{attackHero}, attackQ...)
				} else if model.Int(resistHero, "hid") == model.Int(attacker, "hid") {
					resistHero["energy"] = model.Int(attacker, "blood") / RATE
					resistQ = append([]map[string]any{resistHero}, resistQ...)
				}
				break
			}
			totalBattle++
			battleRet = append(battleRet, s.battleComute(resister, attacker, totalBattle))
			if model.Int(attacker, "blood") == 0 {
				if model.Int(attackHero, "hid") == model.Int(resister, "hid") {
					attackHero["energy"] = model.Int(resister, "blood") / RATE
					attackQ = append([]map[string]any{attackHero}, attackQ...)
				} else if model.Int(resistHero, "hid") == model.Int(resister, "hid") {
					resistHero["energy"] = model.Int(resister, "blood") / RATE
					resistQ = append([]map[string]any{resistHero}, resistQ...)
				}
				break
			}
		}
		report = append(report, battleRet)
	}

	ret["report"] = report
	ret["totalNum"] = totalBattle
	ret["endflag"] = 1
	if len(resistQ) == 0 {
		ret["winer"] = 1
	} else {
		ret["winer"] = 0
	}
	return ret
}

// ── checkFirstPass / getPkGid（PKFunc.php:215/227）───────────────────────

// checkFirstPass 对齐 checkFirstPass：空缺榜位按 rank1→2→3 依次占用，同一 uid 不重复占位。
func (s *Service) checkFirstPass(ctx context.Context, uid, battleId, flag int) error {
	uid1, err := s.cellInt(ctx, "select uid from cfg_pk_first where battleid=? and type=? and rankid=1", battleId, flag)
	if err != nil {
		return err
	}
	uid2, err := s.cellInt(ctx, "select uid from cfg_pk_first where battleid=? and type=? and rankid=2", battleId, flag)
	if err != nil {
		return err
	}
	if ok, err := s.db.Exists(ctx, "select 1 from cfg_pk_first where battleid=? and type=? and rankid=1 and uid=0", battleId, flag); err != nil {
		return err
	} else if ok {
		_, err = s.db.Exec(ctx, "update cfg_pk_first set uid=?,passtime=unix_timestamp() where battleid=? and type=? and rankid=1", uid, battleId, flag)
		return err
	}
	if ok, err := s.db.Exists(ctx, "select 1 from cfg_pk_first where battleid=? and type=? and rankid=2 and uid=0", battleId, flag); err != nil {
		return err
	} else if ok && int64(uid) != uid1 {
		_, err = s.db.Exec(ctx, "update cfg_pk_first set uid=?,passtime=unix_timestamp() where battleid=? and type=? and rankid=2", uid, battleId, flag)
		return err
	}
	if ok, err := s.db.Exists(ctx, "select 1 from cfg_pk_first where battleid=? and type=? and rankid=3 and uid=0", battleId, flag); err != nil {
		return err
	} else if ok && int64(uid) != uid1 && int64(uid) != uid2 {
		_, err = s.db.Exec(ctx, "update cfg_pk_first set uid=?,passtime=unix_timestamp() where battleid=? and type=? and rankid=3", uid, battleId, flag)
		return err
	}
	return nil
}

// getPkGid 对齐 getPkGid：18000 + battleId*10 + flag。
func getPkGid(battleId, flag int) int { return 18000 + battleId*10 + flag }

// ── sendUserReward（PKFunc.php:240）──────────────────────────────────────

// sendUserReward 对齐 sendUserReward：返回掉落物品描述数组（广播裁剪）。
func (s *Service) sendUserReward(ctx context.Context, uid, battleId, battleLevel, battleFlag int) ([]any, error) {
	ret := []any{}
	gid := getPkGid(battleId, battleFlag)
	num := s.randInt(1, 10)
	if num != 3 {
		gid = 0
	}
	if gid > 0 {
		if err := s.addGoods(ctx, uid, gid, 1, 1); err != nil {
			return nil, err
		}
		info, _ := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", gid)
		ret = append(ret, map[string]any{"flag": 0, "info": info, "count": 1})
	}
	if ok, err := s.db.Exists(ctx, "select 1 from sys_pk_user where uid=?", uid); err != nil {
		return nil, err
	} else if !ok {
		if _, err := s.db.Exec(ctx, "insert into sys_pk_user(uid,normal,special) values(?,1,1)", uid); err != nil {
			return nil, err
		}
	}
	var passId int64
	if battleFlag == 1 {
		passId, _ = s.cellInt(ctx, "select special from sys_pk_user where uid=?", uid)
	} else {
		passId, _ = s.cellInt(ctx, "select normal from sys_pk_user where uid=?", uid)
	}
	passBattleId := int(passId / 100)
	tmpLevel := int(passId % 100)
	maxLevel, err := s.cellInt(ctx, "select max(levelid) from cfg_pk_level where battleid=? and type=?", battleId, battleFlag)
	if err != nil {
		return nil, err
	}
	if battleId == passBattleId && maxLevel == int64(tmpLevel+1) && int64(battleLevel) == maxLevel {
		armors, err := s.db.FetchRows(ctx, "select rewardid,count from cfg_pk_reward where battleid=? and type=? and rewardtype=1", passBattleId, battleFlag)
		if err != nil {
			return nil, err
		}
		for _, a := range armors {
			info, err := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", model.Int(a, "rewardid"))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			cnt := int64(model.Int(a, "count"))
			if info != nil {
				if err := s.addArmor(ctx, uid, info, cnt, 1); err != nil {
					return nil, err
				}
			}
			ret = append(ret, map[string]any{"flag": 1, "info": info, "count": cnt})
		}
		goodses, err := s.db.FetchRows(ctx, "select rewardid,count from cfg_pk_reward where battleid=? and type=? and rewardtype=0", passBattleId, battleFlag)
		if err != nil {
			return nil, err
		}
		for _, g := range goodses {
			rid := model.Int(g, "rewardid")
			cnt := int64(model.Int(g, "count"))
			if err := s.addGoods(ctx, uid, rid, cnt, 1); err != nil {
				return nil, err
			}
			info, _ := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", rid)
			ret = append(ret, map[string]any{"flag": 0, "info": info, "count": cnt})
		}
		// 首通：记录名次（原版此处还会按 isFirst 改变公告颜色，公告广播已裁剪）。
		if err := s.checkFirstPass(ctx, uid, battleId, battleFlag); err != nil {
			return nil, err
		}
	}
	// 更新通关最大值
	newId := battleId*100 + battleLevel
	if battleFlag == 0 && int64(newId) > passId {
		if _, err := s.db.Exec(ctx, "update sys_pk_user set normal=? where uid=?", newId, uid); err != nil {
			return nil, err
		}
	} else if battleFlag == 1 && int64(newId) > passId {
		if _, err := s.db.Exec(ctx, "update sys_pk_user set special=? where uid=?", newId, uid); err != nil {
			return nil, err
		}
	}
	// 裁剪：通关提示 sendSysInform 全服公告（sys_inform 无表）→ 不广播。
	return ret, nil
}

// ── checkUserPassLevel / checkUserLevel（PKFunc.php:321/351）─────────────

// checkUserPassLevel 对齐 checkUserPassLevel：越级报 command_exception。
func (s *Service) checkUserPassLevel(ctx context.Context, uid, battleId, battleLevel, battleFlag int) error {
	var passId int64
	if battleFlag == 1 {
		passId, _ = s.cellInt(ctx, "select special from sys_pk_user where uid=?", uid)
	} else {
		passId, _ = s.cellInt(ctx, "select normal from sys_pk_user where uid=?", uid)
	}
	passBattleId := int(passId / 100)
	tmpLevel := int(passId % 100)
	maxLevel, err := s.cellInt(ctx, "select max(levelid) from cfg_pk_level where battleid=? and type=?", battleId, battleFlag)
	if err != nil {
		return err
	}
	if battleId == passBattleId {
		if int64(battleLevel) > maxLevel || battleLevel > tmpLevel+1 {
			return errLegacy(msgCommandExc)
		}
	} else if battleId == passBattleId+1 {
		if battleLevel > 1 {
			return errLegacy(msgCommandExc)
		}
	} else if battleId > passBattleId+1 {
		return errLegacy(msgCommandExc)
	}
	return nil
}

// checkUserLevel 对齐 checkUserLevel：flag=0 查君主将等级；flag=1 查君主修为。
// 裁剪：sys_user_level 无表 → userLevel 恒 0（与 hero/armor 模块同口径）。
func (s *Service) checkUserLevel(ctx context.Context, uid, battleId, flag int) error {
	if flag == 0 {
		heroLevel, err := s.cellInt(ctx, "select level from heroes where user_id=? and hero_type=1000", uid)
		if err != nil {
			return err
		}
		needLevel, err := s.cellInt(ctx, "select hero_level from cfg_pk_battle where id=?", battleId)
		if err != nil {
			return err
		}
		if heroLevel < needLevel {
			return errLegacy(msgHeroLevelLow)
		}
	} else {
		userLevel := int64(0) // 裁剪：sys_user_level 无表
		needLevel, err := s.cellInt(ctx, "select user_level from cfg_pk_battle where id=?", battleId)
		if err != nil {
			return err
		}
		if userLevel < needLevel {
			return errLegacy(msgUserLevelLow)
		}
	}
	return nil
}

// ── getOneBattleRet（PKFunc.php:371）─────────────────────────────────────

// getOneBattleRet 对齐 getOneBattleRet($uid,$param)：打一关，返回 [战斗结果(含 reward)]。
func (s *Service) getOneBattleRet(ctx context.Context, uid, battleId, battleFlag, battleLevel int, battleHids []int) ([]any, error) {
	var ret []any
	err := s.WithUserLock(ctx, uid, "getOneBattleRet", func(ctx context.Context) error {
		if len(battleHids) < 3 {
			return errLegacy(msgInvalidParam)
		}
		ok, err := s.checkGoods(ctx, uid, 19200)
		if err != nil {
			return err
		}
		if !ok {
			return errLegacy(msgNoAdvLijianfu)
		}
		if err := s.checkUserLevel(ctx, uid, battleId, battleFlag); err != nil {
			return err
		}
		if err := s.checkUserPassLevel(ctx, uid, battleId, battleLevel, battleFlag); err != nil {
			return err
		}
		heroId1, heroId2, heroId3 := battleHids[0], battleHids[1], battleHids[2]
		checkNum, err := s.cellInt(ctx, "select count(*) from heroes where user_id=? and id in (?,?,?)", uid, heroId1, heroId2, heroId3)
		if err != nil {
			return err
		}
		if checkNum != 3 {
			return errLegacy(msgNoHeroInfo)
		}

		userHero1, err := s.userHero(ctx, heroId1, 11)
		if err != nil {
			return err
		}
		userHero2, err := s.userHero(ctx, heroId2, 12)
		if err != nil {
			return err
		}
		userHero3, err := s.userHero(ctx, heroId3, 13)
		if err != nil {
			return err
		}
		npcHero1, err := s.npcHero(ctx, battleId, battleLevel, battleFlag, 1, 21)
		if err != nil {
			return err
		}
		npcHero2, err := s.npcHero(ctx, battleId, battleLevel, battleFlag, 2, 22)
		if err != nil {
			return err
		}
		npcHero3, err := s.npcHero(ctx, battleId, battleLevel, battleFlag, 3, 23)
		if err != nil {
			return err
		}

		battle := s.startUserPK(
			[]map[string]any{userHero1, userHero2, userHero3},
			[]map[string]any{npcHero1, npcHero2, npcHero3})

		// 扣掉一个军令
		if err := s.reduceGoods(ctx, uid, 19200, 1, 0); err != nil {
			return err
		}
		// 发放奖励
		msg := []any{}
		if model.Int(battle, "winer") == 1 {
			msg, err = s.sendUserReward(ctx, uid, battleId, battleLevel, battleFlag)
			if err != nil {
				return err
			}
		}
		battle["reward"] = msg
		ret = []any{battle}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// userHero 对齐 getOneBattleRet 的 userHero 查询（sys_city_hero a,mem_hero_blood b）。
func (s *Service) userHero(ctx context.Context, heroId, standIndex int) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx, `select a.id as hid, a.sex, b.`+"`force`"+` as energy, ? as standIndex,
		command_base+command_add_on as command,
		affairs_base+affairs_add+affairs_add_on as affair,
		bravery_base+bravery_add+bravery_add_on as bravery,
		wisdom_base+wisdom_add+wisdom_add_on as wisdom,
		speed_add_on as speed
		from heroes a, hero_blood b where a.id=b.hero_id and a.id=?`, standIndex, heroId)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// npcHero 对齐 getOneBattleRet 的 NPC 查询（cfg_pk_level + cfg_pk_hero）。
func (s *Service) npcHero(ctx context.Context, battleId, battleLevel, battleFlag, area, standIndex int) (map[string]any, error) {
	npcHid, err := s.cellInt(ctx, "select hid from cfg_pk_level where battleid=? and levelid=? and type=? and area=?", battleId, battleLevel, battleFlag, area)
	if err != nil {
		return nil, err
	}
	row, err := s.db.FetchOne(ctx, `select hid,sex,energy+energy_add as energy, ? as standIndex,
		command_base+command_add as command,
		affair_base+affair_add as affair,
		bravery_base+bravery_add as bravery,
		wisdom_base+wisdom_add as wisdom,
		speed_base+speed_add as speed
		from cfg_pk_hero where hid=?`, standIndex, npcHid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// ── getPkFirstReward（PKFunc.php:424）────────────────────────────────────

// getPkFirstReward 对齐 getPkFirstReward：领取榜首奖励。返回 [parseAndAddReward 结果]。
func (s *Service) getPkFirstReward(ctx context.Context, uid, battleId, flag, rankId int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "getPkFirstReward", func(ctx context.Context) error {
		ok, err := s.db.Exists(ctx, "select 1 from cfg_pk_first where uid=? and battleid=? and rankid=? and type=?", uid, battleId, rankId, flag)
		if err != nil {
			return err
		}
		if !ok {
			return errLegacy(msgInvalidParam)
		}
		got, err := s.db.Exists(ctx, "select 1 from cfg_pk_first where uid=? and battleid=? and rankid=? and type=? and time>1", uid, battleId, rankId, flag)
		if err != nil {
			return err
		}
		if got {
			return errLegacy(msgHasGetReward)
		}
		reward, err := s.cellStr(ctx, "select reward from cfg_pk_first where uid=? and battleid=? and type=? and rankid=?", uid, battleId, flag, rankId)
		if err != nil {
			return err
		}
		parsed, err := s.parseAndAddReward(ctx, uid, reward, 5, 5, 5, 65)
		if err != nil {
			return err
		}
		out = []any{parsed}
		if _, err := s.db.Exec(ctx, "update cfg_pk_first set time=unix_timestamp() where uid=? and battleid=? and rankid=? and type=?", uid, battleId, rankId, flag); err != nil {
			return err
		}
		return nil
	})
	return out, err
}

// ── loadCampaignInitData（PKFunc.php:452）────────────────────────────────

// loadCampaignInitData 对齐 loadCampaignInitData：
// 返回 [levelInfo, normalHeroes, specialHeroes, rewards, goods, battleDesc]。
func (s *Service) loadCampaignInitData(ctx context.Context, uid int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "loadCampaignInitData", func(ctx context.Context) error {
		levelInfo, err := s.db.FetchOne(ctx, "select normal,special from sys_pk_user where uid=?", uid)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := s.db.Exec(ctx, "insert into sys_pk_user(`uid`,`normal`,`special`) values(?,1,1)", uid); err != nil {
				return err
			}
			levelInfo, err = s.db.FetchOne(ctx, "select normal,special from sys_pk_user where uid=?", uid)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		out = append(out, levelInfo)

		normalHeroes, err := s.db.FetchRows(ctx, `select b.battleid,b.levelid,a.hero_level,a.user_level,b.hid,
			c.face,c.sex,c.flag,c.name,b.area,c.energy,c.energy_add,c.command_base,c.command_add,
			c.affair_base,c.affair_add,c.bravery_base,c.bravery_add,c.wisdom_base,c.wisdom_add,c.speed_base,c.speed_add,
			FLOOR((c.bravery_base+c.bravery_add)*5+(c.energy+c.energy_add)*10+c.wisdom_base+c.wisdom_add) as battle
			from cfg_pk_battle a, cfg_pk_level b, cfg_pk_hero c
			where a.id=b.battleid and b.hid=c.hid and b.type=0`)
		if err != nil {
			return err
		}
		out = append(out, normalHeroes)

		specialHeroes, err := s.db.FetchRows(ctx, `select b.battleid,b.levelid,a.hero_level,a.user_level,b.hid,
			c.face,c.sex,c.flag,c.name,b.area,c.energy,c.energy_add,c.command_base,c.command_add,
			c.affair_base,c.affair_add,c.bravery_base,c.bravery_add,c.wisdom_base,c.wisdom_add,c.speed_base,c.speed_add,
			FLOOR((c.bravery_base+c.bravery_add)*5+(c.energy+c.energy_add)*10+c.wisdom_base+c.wisdom_add) as battle
			from cfg_pk_battle a, cfg_pk_level b, cfg_pk_hero c
			where a.id=b.battleid and b.hid=c.hid and b.type=1`)
		if err != nil {
			return err
		}
		out = append(out, specialHeroes)

		rewards := []map[string]any{}
		battleids, err := s.db.FetchRows(ctx, "select id from cfg_pk_battle")
		if err != nil {
			return err
		}
		for _, b := range battleids {
			bid := model.Int(b, "id")
			for _, flag := range []int{0, 1} {
				gid := getPkGid(bid, flag)
				goods, _ := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", gid)
				rewards = append(rewards, map[string]any{"battleid": bid, "flag": flag, "goods": goods})
			}
		}
		out = append(out, rewards)

		goodsInfo := map[string]any{}
		junling, err := s.db.FetchOne(ctx, "select * from user_goods where user_id=? and gid=19200", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		goodsInfo["junling"] = model.Int(junling, "count")
		if junling == nil {
			// 首次送玩家 10 个军令
			if err := s.addGoods(ctx, uid, 19200, 10, 5); err != nil {
				return err
			}
			goodsInfo["junling"] = 10
		}
		wuzhuqian, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=10195", uid)
		if err != nil {
			return err
		}
		goodsInfo["wuzhuqian"] = wuzhuqian
		out = append(out, goodsInfo)

		battleDesc, err := s.db.FetchRows(ctx, "select id,description from cfg_pk_battle")
		if err != nil {
			return err
		}
		out = append(out, battleDesc)
		return nil
	})
	return out, err
}

// loadPKRewardRank 对齐 loadPKRewardRank：返回 [rankRewardFrontThree]。
func (s *Service) loadPKRewardRank(ctx context.Context, uid, battleId, battleType int) ([]any, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_pk_first where battleid=? and type=? order by rankid asc limit 3", battleId, battleType)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		uidOne := model.Int(rows[i], "uid")
		if uidOne != 0 {
			rows[i]["userName"], err = s.cellStr(ctx, "select name from users where id=?", uidOne)
			if err != nil {
				return nil, err
			}
			rows[i]["formatTime"], err = s.cellStr(ctx, "select from_unixtime(?)", model.Int64(rows[i], "passtime"))
			if err != nil {
				return nil, err
			}
		}
		rewardArr := strings.Split(model.Str(rows[i], "reward"), ",")
		if len(rewardArr) > 3 && rewardArr[1] == "0" { // 奖励为物品
			gid := atoiSafe(rewardArr[2])
			rows[i]["rewardType"] = 0
			good, _ := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", gid)
			rows[i]["rewardGood"] = good
			rows[i]["rewardCount"] = rewardArr[3]
		} else if len(rewardArr) > 3 { // 奖品为装备
			armorid := atoiSafe(rewardArr[2])
			rows[i]["rewardType"] = 1
			good, _ := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", armorid)
			rows[i]["rewardGood"] = good
			rows[i]["rewardCount"] = rewardArr[3]
		}
	}
	return []any{rows}, nil
}

// regetCampaignMaxData 对齐 regetCampaignMaxData：返回 [userBattleInfo, junlin, wuzhuqian]。
func (s *Service) regetCampaignMaxData(ctx context.Context, uid int) ([]any, error) {
	userBattleInfo, err := s.db.FetchOne(ctx, "select normal,special from sys_pk_user where uid=?", uid)
	if errors.Is(err, sql.ErrNoRows) {
		userBattleInfo = nil
	} else if err != nil {
		return nil, err
	}
	junlin, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=19200", uid)
	if err != nil {
		return nil, err
	}
	wuzhuqian, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=10195", uid)
	if err != nil {
		return nil, err
	}
	return []any{userBattleInfo, junlin, wuzhuqian}, nil
}

// buyJunlingFunc 对齐 buyJunlingFunc：购买军令。返回 [type, newCount, money]。
func (s *Service) buyJunlingFunc(ctx context.Context, uid, typ, count int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "buyJunlingFunc", func(ctx context.Context) error {
		const price = 5 // 购买价格
		if typ != 0 {   // 目前只允许元宝购买
			return errLegacy(msgInvalidPayType)
		}
		if count <= 0 {
			return errLegacy(msgInvalidAmount)
		}
		needCost := int64(count * price)
		if typ == 0 {
			ok, err := s.checkMoney(ctx, uid, needCost)
			if err != nil {
				return err
			}
			if !ok {
				return errLegacy(msgNoEnoughYuan)
			}
		}
		currentCount, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=19200", uid)
		if err != nil {
			return err
		}
		maxBuyCount := 0
		if currentCount == 0 {
			maxBuyCount = 100
		} else {
			maxBuyCount = 100 - int(currentCount)
		}
		if count > maxBuyCount {
			return errLegacy(fmt.Sprintf(msgJunlingMax, maxBuyCount))
		}
		out = []any{typ, int(currentCount) + count}
		if typ == 0 {
			if err := s.addMoney(ctx, uid, -needCost, typ); err != nil {
				return err
			}
			money, err := s.cellInt(ctx, "select money from users where id=?", uid)
			if err != nil {
				return err
			}
			out = append(out, money)
		}
		if err := s.addGoods(ctx, uid, 19200, int64(count), 0); err != nil {
			return err
		}
		return nil
	})
	return out, err
}

// getAllHeroByUid 对齐 getAllHeroByUid：分页取玩家将领（君主将 page=1 且未选时置首）。
func (s *Service) getAllHeroByUid(ctx context.Context, uid int, hasSelected []int, page int) ([]any, error) {
	if page <= 0 {
		return nil, errLegacy(msgInvalidParam)
	}
	hidStrs := make([]string, 0, len(hasSelected))
	for _, h := range hasSelected {
		hidStrs = append(hidStrs, strconv.Itoa(h))
	}
	hidStr := strings.Join(hidStrs, ",")
	selected := map[int]bool{}
	for _, h := range hasSelected {
		selected[h] = true
	}
	ret := []any{}
	startIndex := (page - 1) * 10

	tenQuery := func(limitClause string, exclude bool) ([]map[string]any, error) {
		q := `select h.*, (h.bravery_base+h.bravery_add+h.bravery_add_on) as heroBravery, m.*
			from heroes h, hero_blood m where h.id=m.hero_id and h.user_id=? and h.hero_type<>1000`
		args := []any{uid}
		if exclude && hidStr != "" {
			q += " and h.id not in (" + hidStr + ")"
		}
		q += " order by heroBravery desc " + limitClause
		return s.db.FetchRows(ctx, q, args...)
	}

	if page == 1 {
		kingHero, err := s.db.FetchOne(ctx, `select h.*, m.* from heroes h left join hero_blood m on m.hero_id=h.id
			where h.user_id=? and h.hero_type=1000`, uid)
		if errors.Is(err, sql.ErrNoRows) {
			kingHero = nil
		} else if err != nil {
			return nil, err
		}
		if hidStr == "" {
			tenHeros, err := tenQuery("limit 9", false)
			if err != nil {
				return nil, err
			}
			ret = append(ret, tenHeros, page, kingHero)
			return ret, nil
		}
		kingHid, _ := s.cellInt(ctx, "select id from heroes where user_id=? and hero_type=1000", uid)
		if selected[int(kingHid)] {
			tenHeros, err := tenQuery("limit 10", true)
			if err != nil {
				return nil, err
			}
			ret = append(ret, tenHeros, page)
			return ret, nil
		}
		tenHeros, err := tenQuery("limit 9", true)
		if err != nil {
			return nil, err
		}
		ret = append(ret, tenHeros, page, kingHero)
		return ret, nil
	}

	exclude := hidStr != ""
	tenHeros, err := tenQuery(fmt.Sprintf("limit %d,10", startIndex), exclude)
	if err != nil {
		return nil, err
	}
	ret = append(ret, tenHeros, page)
	return ret, nil
}

// ── 小工具 ────────────────────────────────────────────────────────────────

func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func safeIdx(ss []string, i int) string {
	if i < len(ss) {
		return ss[i]
	}
	return "0"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
