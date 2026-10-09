package lottery

// service.go 1:1 复刻 legacy server/game/LotteryFunc.php 全文。
// 入口函数对照（文件:行号）：
//   getGoods(9) / startLottery(82) / useGoodsToPayAgain(145) / getLotteryReward(156) / autoGetReward(175)
//   getTodayCount(201) / randWin(218) / logIDs(303) / retrieveGoods(320) / getAllLevelGoods(343)
//   getGoodsByType(396) / randType(446) / getWin(460) / sendSysInformHere(512) / restart(538)
//   addCount(566) / checkLotteryMoney(596) / useMoney(609) / specialProp(626) / checkLottoryTime(669)
//
// 原版怪癖 1:1 保留：
//   - checkLottoryTime(669)：原版源码把时间窗判断整段注释掉，函数体内仅剩 `throw not_available_time`
//     ——即"任何时刻抽奖都不可用"。此处保留该行为（默认恒抛 not_available_time），
//     另提供 SetTimeGate(true) 测试/运营放行开关（不改变默认行为），见函数注释；
//   - getGoods(24) `got==1` 即重开盘面，win 重置为 '-1,0,0'，restart_count 清 0（每次 Round 都清）；
//   - randWin(258/265) 双层循环去重 `$last_win_id/$last_win_type`：先按中奖档位向下找，仍与上次相同则
//     从 8 等倒序再找一次；若全盘仅有一个物品且与上次相同则结果保持上次（原版如此）；
//   - getAllLevelGoods(359) 当 r1/r2 全空时强制 r3 取 1；getGoodsByType(399) 用 `$total` 限制总数不超过 8；
//   - getGoodsByType(405/417) 7/8 等强制 type=1（道具）；
//   - getGoodsByType(426) case3（礼金）在 randType 去除礼金分支后已成死代码，仍保留；
//   - useMoney(611) 仅在 `$count>1` 才扣元宝，否则直接返回 [1]；
//   - checkLotteryMoney(600) `$money<$use_money` 用 6 作阈值（原版注释写"消耗10个元宝"但实为 6）。
//
// 裁剪（保留判断结构，按"原版会返回 false/空"语义返回并注释）：
//   - sendSysInformHere(512) 的全服滚动公告（sys_inform 无表）——仅保留写表/返回值，广播调用裁剪；
//   - getWin(474/484/492) 的 addGift/addArmor/addGoods 本地实现（不跨包）；completeTaskWithTaskid(completeTask 293) 属 M8，省略。

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

type Service struct {
	db *db.DB
	lk *lock.Locker

	mu       sync.Mutex
	rnd      *rand.Rand
	timeGate bool // 默认 false=复刻原版"恒抛 not_available_time"；true=放行（测试注入）
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB), rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// SetSeed 注入随机种子（1:1 逻辑不可注入 mt_rand，此处仅为集成测试可复现）。
func (s *Service) SetSeed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rnd = rand.New(rand.NewSource(seed))
}

// SetTimeGate 放行 checkLottoryTime 的时间闸（默认 false 复刻原版恒抛；测试用）。
func (s *Service) SetTimeGate(open bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeGate = open
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

// errLegacy 构造与 legacy throw new Exception 等价的错误响应（HTTP 400，code=lottery_error）。
func errLegacy(msg string) error {
	return httpx.BadRequest("lottery_error", msg)
}

// lang 文案（server/game/lang.php 逐字）。
const (
	msgInformWinGoods = "恭喜【%s】玩家在幸运宝匣中获得【%s】！"                                     // lottery.inform_win_goods
	msgNoSuchGoods    = "没有这样的物品"                                                   // lottery.no_such_goods
	msgNoSuchArmor    = "没有这样的装备"                                                   // lottery.no_such_armor
	msgGetWin         = "领奖成功"                                                      // lottery.get_win
	msgFullPlayCount  = "你今天玩过了"                                                    // lottery.full_playcount
	msgRestartLimit   = "你每天只有一次重开机会"                                               // lottery.restart_limit
	msgNoMoney        = "你的元宝不足，请充值"                                                // lottery.no_money
	msgNotAvailable   = "该功能未开放"                                                    // lottery.not_available
	msgNotAvailableT  = "幸运宝盒在每天的11点和19点开启，每次开启时间为1小时。每次开启可免费抽奖一次，如不满意，您还有一次重开的机会。" // lottery.not_available_time
	msgNobilityLimit  = "你爵位未达到公士，不能使用幸运宝匣。"                                        // lottery.nobility_limit
	msgWaiguaInvalid  = "数据异常"                                                      // waigua.invalid
	msgServerBusy     = "服务器忙，请稍后再进行操作。"                                            // pacifyPeople.server_busy
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

// cellStr 读取单列字符串；无行返回空串。
func (s *Service) cellStr(ctx context.Context, q string, args ...any) (string, error) {
	v, err := s.db.FetchCellString(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// addGift 对齐 utils.php:1135 addGift。
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

// addGoods 对齐 utils.php:971 addGoods（gid==0 礼金 / gid==152 铜钱双记账怪癖，与 M8 同口径）。
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

// checkGoods 对齐 utils.php:933（拥有数量>=1）。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	v, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	return v >= 1, err
}

// addArmor 对齐 utils.php:1067 addArmor（updateBattleOpenState/sendDesignation 属 M7/M8，省略）。
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

// ── checkLottoryTime（LotteryFunc.php:669）───────────────────────────────

// checkLottoryTime 复刻原版：原版时间窗判断被整段注释，函数体仅剩 throw not_available_time，
// 即任何时刻都不可用（原版怪癖，1:1 保留）。提供 timeGate 放行开关供测试/运营注入，
// 默认 false → 保持恒抛 not_available_time。
func (s *Service) checkLottoryTime(ctx context.Context) error {
	// 原版仍会读取服务器当前时间（select now()）并取小时；此处保留该读取以维持副作用一致。
	if _, err := s.cellStr(ctx, "select now()"); err != nil {
		return err
	}
	s.mu.Lock()
	gate := s.timeGate
	s.mu.Unlock()
	if gate {
		return nil
	}
	return errLegacy(msgNotAvailableT)
}

// lotteryOpen 检查总开关：mem_state state=150，非 0 → 未开放（对齐 LotteryFunc.php:11/91）。
// 无行（empty）→ 视为 0（开放），与原版一致。
func (s *Service) lotteryOpen(ctx context.Context) (bool, error) {
	v, err := s.cellInt(ctx, "select value from mem_state where state=150")
	if err != nil {
		return false, err
	}
	return v == 0, nil
}

// ── getGoods（LotteryFunc.php:9）──────────────────────────────────────────

// GetGoods 对齐 getGoods($uid)。返回 [records, win_type, win_id, win_count, restart_count, todayCount]。
func (s *Service) GetGoods(ctx context.Context, uid int) ([]any, error) {
	infor, err := s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=?", uid)
	noRow := errors.Is(err, sql.ErrNoRows)
	if err != nil && !noRow {
		return nil, err
	}
	var records [][]map[string]any
	if noRow || model.Int(infor, "got") == 1 { // 这个奖已经领过了 / 无盘面 → 重开
		all := s.getAllLevelGoods(ctx)
		recordStr := s.recordsToString(all)
		if noRow {
			_, err = s.db.Exec(ctx,
				"insert into mem_lottery_goods(uid, records, `time`, win, got, restart_count) values(?,?,current_date(),'-1,0,0',0,0)",
				uid, recordStr)
			if err != nil {
				return nil, err
			}
			infor, err = s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=?", uid)
			if err != nil {
				return nil, err
			}
		} else {
			_, err = s.db.Exec(ctx,
				"update mem_lottery_goods set records=?, `time`=current_date(), win='-1,0,0', got=0, restart_count=0 where uid=?",
				recordStr, uid)
			if err != nil {
				return nil, err
			}
			infor, err = s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=?", uid)
			if err != nil {
				return nil, err
			}
		}
		records = s.retrieveGoods(ctx, model.Str(infor, "records"))
	} else {
		records = s.retrieveGoods(ctx, model.Str(infor, "records"))
	}

	winAry := strings.Split(model.Str(infor, "win"), ",")
	for len(winAry) < 3 {
		winAry = append(winAry, "0")
	}
	tcount, err := s.GetTodayCount(ctx, uid)
	if err != nil {
		return nil, err
	}
	return []any{records, winAry[0], winAry[1], winAry[2], model.Int(infor, "restart_count"), tcount}, nil
}

// ── startLottery（LotteryFunc.php:82）────────────────────────────────────

// StartLottery 对齐 startLottery($uid)。返回 [win_type, win_id, win_count, todayCount]；
// 使用次数达上限时返回 [-1,-2,-1,count]。
func (s *Service) StartLottery(ctx context.Context, uid int) ([]any, error) {
	var ret []any
	err := s.WithUserLock(ctx, uid, "startLottery", func(ctx context.Context) error {
		// 爵位限制：公士(>=1) 才能用幸运宝盒（含推恩）。
		real, err := s.nobilityOf(ctx, uid)
		if err != nil {
			return err
		}
		nobility, err := s.getBufferNobility(ctx, uid, real)
		if err != nil {
			return err
		}
		if nobility < 1 {
			return errLegacy(msgNobilityLimit)
		}
		open, err := s.lotteryOpen(ctx)
		if err != nil {
			return err
		}
		if !open {
			return errLegacy(msgNotAvailable)
		}
		if err := s.checkLottoryTime(ctx); err != nil {
			return err
		}
		count, err := s.GetTodayCount(ctx, uid)
		if err != nil {
			return err
		}
		if count >= 1 { // 第一次免费
			paid := false
			for _, p := range []struct {
				gid   int
				limit int
			}{{19989, 0}, {10191, 4}, {159, 20}} { // 幸运宝盒机会 / 虎尾 / 幸运之钥
				ok, err := s.useGoodsToPayAgain(ctx, uid, p.gid, p.limit)
				if err != nil {
					return err
				}
				if ok {
					paid = true
					break
				}
			}
			if !paid {
				ret = []any{-1, -2, -1, int(count)} // 使用次数达到上限
				return nil
			}
		}

		infor, err := s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=? and got=0", uid)
		if errors.Is(err, sql.ErrNoRows) {
			return errLegacy(msgWaiguaInvalid)
		}
		if err != nil {
			return err
		}
		records := s.retrieveGoods(ctx, model.Str(infor, "records"))
		tmp, err := s.randWin(ctx, uid, records, false, -100, -1)
		if err != nil {
			return err
		}
		ret = []any{tmp[0], tmp[1], tmp[2]}
		if _, err := s.db.Exec(ctx, "insert into log_lottery(uid, `time`, gid, `type`) values(?, NOW(), ?, ?)", uid, tmp[1], tmp[0]); err != nil {
			return err
		}
		count, err = s.GetTodayCount(ctx, uid)
		if err != nil {
			return err
		}
		ret = append(ret, int(count))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// useGoodsToPayAgain 对齐 useGoodsToPayAgain（LotteryFunc.php:145）。
func (s *Service) useGoodsToPayAgain(ctx context.Context, uid, gid, dayUseLimit int) (bool, error) {
	have, err := s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if err != nil {
		return false, err
	}
	if have < 1 {
		return false, nil
	}
	if dayUseLimit != 0 {
		used, err := s.cellInt(ctx,
			"select count(*) from log_goods where user_id=? and gid=? and type=0 and count=-1 and time>=unix_timestamp(curdate())", uid, gid)
		if err != nil {
			return false, err
		}
		if used >= int64(dayUseLimit) {
			return false, nil
		}
	}
	if err := s.reduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return false, err
	}
	return true, nil
}

// ── getLotteryReward / autoGetReward（LotteryFunc.php:156/175）───────────

// GetLotteryReward 对齐 getLotteryReward：领奖并重开盘面。返回 getGoods 结果。
func (s *Service) GetLotteryReward(ctx context.Context, uid int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "getLotteryReward", func(ctx context.Context) error {
		open, err := s.lotteryOpen(ctx)
		if err != nil {
			return err
		}
		if !open {
			return errLegacy(msgNotAvailable)
		}
		if err := s.checkLottoryTime(ctx); err != nil {
			return err
		}
		ok, err := s.db.Exists(ctx, "select 1 from mem_lottery_goods where uid=? and got=0", uid)
		if err != nil {
			return err
		}
		if !ok {
			return errLegacy(msgWaiguaInvalid)
		}
		if _, err := s.addCount(ctx, uid); err != nil {
			return err
		}
		out, err = s.GetGoods(ctx, uid)
		return err
	})
	return out, err
}

// AutoGetReward 对齐 autoGetReward：返回 [winObj, newRoundGoods]。
func (s *Service) AutoGetReward(ctx context.Context, uid int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "autoGetReward", func(ctx context.Context) error {
		open, err := s.lotteryOpen(ctx)
		if err != nil {
			return err
		}
		if !open {
			return errLegacy(msgNotAvailable)
		}
		if err := s.checkLottoryTime(ctx); err != nil {
			return err
		}
		ok, err := s.db.Exists(ctx, "select 1 from mem_lottery_goods where uid=? and got=0", uid)
		if err != nil {
			return err
		}
		if !ok {
			return errLegacy(msgWaiguaInvalid)
		}
		winObj, err := s.addCount(ctx, uid)
		if err != nil {
			return err
		}
		newRoundGoods, err := s.GetGoods(ctx, uid)
		if err != nil {
			return err
		}
		out = []any{winObj, newRoundGoods}
		return nil
	})
	return out, err
}

// ── getTodayCount（LotteryFunc.php:201）──────────────────────────────────

// GetTodayCount 对齐 getTodayCount：近 1 小时内 log_lottery 条数（函数名沿用原版）。
func (s *Service) GetTodayCount(ctx context.Context, uid int) (int64, error) {
	return s.cellInt(ctx, "select count(*) from log_lottery where uid=? and (unix_timestamp()-unix_timestamp(time))<=3600", uid)
}

// ── specialProp（LotteryFunc.php:626）────────────────────────────────────

// specialProp 对齐 specialProp：默认概率数组，命中"7 连无优"或"4 连无优"时改写概率。
func (s *Service) specialProp(ctx context.Context, uid int) ([]int, error) {
	prob := []int{10, 190, 300, 500, 1000, 1500, 2500, 4000}
	logs, err := s.db.FetchRows(ctx,
		"select l.*, c.level as level from log_lottery l left join cfg_goods c on l.gid=c.gid where l.uid=? and to_days(l.time)=TO_DAYS(NOW()) order by l.time desc", uid)
	if err != nil {
		return nil, err
	}
	count := len(logs)
	if count%7 == 6 {
		hasBetter6 := false
		for i := 0; i < 6 && i < len(logs); i++ {
			if model.Int(logs[i], "level") < 6 { // 缺 level（left join 未命中）→ 0 → 视为优于 6 等（原版 null<6 为真）
				hasBetter6 = true
				break
			}
		}
		if !hasBetter6 {
			prob = []int{10, 190, 300, 500, 9000, 0, 0, 0}
		}
	} else if count%4 == 3 {
		hasBetter7 := false
		for i := 0; i < 3 && i < len(logs); i++ {
			if model.Int(logs[i], "level") < 7 {
				hasBetter7 = true
				break
			}
		}
		if !hasBetter7 {
			prob = []int{10, 190, 300, 500, 1000, 8000, 0, 0}
		}
	}
	return prob, nil
}

// ── randWin（LotteryFunc.php:218）────────────────────────────────────────

// randWin 对齐 randWin($uid,$records,$is_restart_req,$last_win_id,$last_win_type)。
// 返回 [win_type, win_id, win_count]；并按 is_restart_req 写回 mem_lottery_goods.win。
func (s *Service) randWin(ctx context.Context, uid int, records [][]map[string]any, isRestartReq bool, lastWinID, lastWinType int) ([]int, error) {
	prob, err := s.specialProp(ctx, uid)
	if err != nil {
		return nil, err
	}
	winRand := s.randInt(1, 10000)
	winID := -1
	winType := 0
	winCount := 1

	winIndex := 7 // 几等奖（7=8等）
	sum := 0
	for i := 0; i < 8; i++ {
		sum += prob[i]
		if sum >= winRand {
			winIndex = i
			break
		}
	}

	// 第一次：从 winIndex 向下（到 8 等）找，保证能获奖；与上次相同则 continue。
	for i := winIndex; i < 8; i++ {
		if i >= len(records) {
			break
		}
		rec := records[i]
		if len(rec) == 0 {
			continue
		}
		idx := s.randInt(0, len(rec)-1)
		if _, hasGid := rec[idx]["gid"]; hasGid {
			winID = model.Int(rec[idx], "gid")
			winType = 0
			winCount = model.Int(rec[idx], "count")
		} else {
			winID = model.Int(rec[idx], "id")
			winType = 1 // 装备
		}
		if lastWinID == winID && lastWinType == winType {
			continue
		}
		break
	}
	// 第二次（保证获奖）：若结果仍与上次相同，从 8 等倒序再找一次。
	if lastWinID == winID && lastWinType == winType {
		for i := 7; i >= 0; i-- {
			if i >= len(records) {
				continue
			}
			rec := records[i]
			if len(rec) == 0 {
				continue
			}
			idx := s.randInt(0, len(rec)-1)
			if _, hasGid := rec[idx]["gid"]; hasGid {
				winID = model.Int(rec[idx], "gid")
				winType = 0
				winCount = model.Int(rec[idx], "count")
			} else {
				winID = model.Int(rec[idx], "id")
				winType = 1
			}
			if lastWinID == winID && lastWinType == winType {
				continue
			}
			break
		}
	}

	winStr := fmt.Sprintf("%d,%d,%d", winType, winID, winCount)
	if isRestartReq {
		if _, err := s.db.Exec(ctx, "update mem_lottery_goods set win=?, `restart_count`=`restart_count`+1 where uid=?", winStr, uid); err != nil {
			return nil, err
		}
	} else {
		if _, err := s.db.Exec(ctx, "update mem_lottery_goods set win=? where uid=?", winStr, uid); err != nil {
			return nil, err
		}
	}
	return []int{winType, winID, winCount}, nil
}

// logIDs 对齐 logIDs($rs)：goods（有 gid）→ "0,gid,count"；armor（无 gid）→ "1,id,count"。
func logIDs(rs map[string]any) string {
	count := model.Int(rs, "count")
	if _, hasGid := rs["gid"]; hasGid {
		return fmt.Sprintf("0,%d,%d", model.Int(rs, "gid"), count)
	}
	return fmt.Sprintf("1,%d,%d", model.Int(rs, "id"), count)
}

// retrieveGoods 对齐 retrieveGoods($ids)：解析 records 字符串为 8 个档位的物品数组。
func (s *Service) retrieveGoods(ctx context.Context, ids string) [][]map[string]any {
	ret := make([][]map[string]any, 8)
	for i := range ret {
		ret[i] = []map[string]any{}
	}
	parts := strings.Split(ids, ",")
	for i := 0; i+2 < len(parts); i += 3 {
		typ := atoiSafe(parts[i])
		id := atoiSafe(parts[i+1])
		count := atoiSafe(parts[i+2])
		var goods map[string]any
		var err error
		if typ == 1 {
			goods, err = s.db.FetchOne(ctx, "select * from cfg_armor where id=?", id)
		} else if typ == 0 {
			goods, err = s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", id)
		}
		if err != nil || goods == nil {
			continue
		}
		goods["count"] = count
		lv := model.Int(goods, "level")
		if lv >= 1 && lv <= 8 {
			ret[lv-1] = append(ret[lv-1], goods)
		}
	}
	return ret
}

// recordsToString 把 8 档物品拍平成 records 字符串（对齐 getGoods 循环 + logIDs）。
func (s *Service) recordsToString(records [][]map[string]any) string {
	parts := []string{}
	for i := 0; i < 8; i++ {
		for _, g := range records[i] {
			parts = append(parts, logIDs(g))
		}
	}
	return strings.Join(parts, ",")
}

// ── getAllLevelGoods（LotteryFunc.php:343）───────────────────────────────

// getAllLevelGoods 对齐 getAllLevelGoods：按 1,2,3,7,8,4,5,6 的顺序生成 8 档物品，返回 ret[0..7]=1~8 等。
func (s *Service) getAllLevelGoods(ctx context.Context) [][]map[string]any {
	total := 0
	r1 := s.getGoodsByType(ctx, s.randInt(0, 1), 1, &total, false)
	total += len(r1)
	r2 := s.getGoodsByType(ctx, s.randInt(0, 1), 2, &total, false)
	total += len(r2)
	c3 := s.randInt(0, 1)
	if total == 0 {
		c3 = 1
	}
	r3 := s.getGoodsByType(ctx, c3, 3, &total, false)
	total += len(r3)
	r7 := s.getGoodsByType(ctx, s.randInt(1, 2), 7, &total, false)
	total += len(r7)
	r8 := s.getGoodsByType(ctx, s.randInt(1, 2), 8, &total, false)
	total += len(r8)
	r4 := s.getGoodsByType(ctx, s.randInt(0, 2), 4, &total, false)
	total += len(r4)
	r5 := s.getGoodsByType(ctx, s.randInt(0, 2), 5, &total, false)
	total += len(r5)
	c6 := s.randInt(0, 2)
	if total+c6 < 8 {
		c6 = 8 - total // 保证 8 个
	}
	r6 := s.getGoodsByType(ctx, c6, 6, &total, true)
	total += len(r6)

	return [][]map[string]any{r1, r2, r3, r4, r5, r6, r7, r8}
}

// getGoodsByType 对齐 getGoodsByType($count,$level,$total,$is_last)。
func (s *Service) getGoodsByType(ctx context.Context, count, level int, total *int, isLast bool) []map[string]any {
	ret := []map[string]any{}
	if *total+count > 8 {
		count = max(0, 8-*total)
	}
	for i := 0; i < count; i++ {
		typ := s.randType()
		if level == 8 || level == 7 {
			typ = 1
		}
		var record map[string]any
		switch typ {
		case 0: // 材料
			record, _ = s.db.FetchOne(ctx, "select * from cfg_goods where gid<10000 and group_id in (4,5) and level=? order by rand() limit 1", level)
			if record == nil {
				break
			}
			record["count"] = int(math.Floor((float64(s.randInt(0, 30))/100)*float64(level-1) + 1))
		case 1: // 道具
			record, _ = s.db.FetchOne(ctx, "select * from cfg_goods where gid<10000 and group_id in (0,1,2,3) and level=? order by rand() limit 1", level)
			if record == nil {
				break
			}
			record["count"] = 1
		case 2: // 装备
			record, _ = s.db.FetchOne(ctx, "select * from cfg_armor where level=? order by rand() limit 1", level)
			if record == nil {
				break
			}
			record["count"] = 1
		case 3: // 礼金（randType 已去除该分支→死代码，1:1 保留）
			record, _ = s.db.FetchOne(ctx, "select * from cfg_goods where gid=0")
			if record != nil {
				record["count"] = int(math.Floor(600 / math.Pow(float64(level), 2)))
			}
		}
		if record != nil {
			ret = append(ret, record)
		}
	}
	if isLast {
		if *total+len(ret) < 8 {
			left := 8 - *total - len(ret)
			for i := 0; i < left; i++ {
				record, _ := s.db.FetchOne(ctx, "select * from cfg_goods where gid<10000 and group_id in (0,1,2,3) and level=6 order by rand() limit 1")
				if record == nil {
					break
				}
				record["count"] = 1
				ret = append(ret, record)
			}
		}
	}
	return ret
}

// randType 对齐 randType：0 材料(<=50) / 1 道具(<=75) / 2 装备(>75)；礼金分支已去除。
func (s *Service) randType() int {
	r := s.randInt(1, 100)
	if r <= 50 {
		return 0
	} else if r > 50 && r <= 75 {
		return 1
	}
	return 2
}

// ── getWin（LotteryFunc.php:460）─────────────────────────────────────────

// getWin 对齐 getWin($uid,$type,$id,$count)：发放奖励、置 got=1、返回 winObj（含 count）。
func (s *Service) getWin(ctx context.Context, uid, typ, id int, count int64) (map[string]any, error) {
	var winObj map[string]any
	var err error
	if id == 0 && typ == 0 { // 礼金
		winObj, err = s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errLegacy(msgNoSuchGoods)
		}
		if err != nil {
			return nil, err
		}
		if err := s.addGift(ctx, uid, count, 1000); err != nil {
			return nil, err
		}
	} else if typ == 1 { // 装备
		winObj, err = s.db.FetchOne(ctx, "select * from cfg_armor where id=?", id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errLegacy(msgNoSuchArmor)
		}
		if err != nil {
			return nil, err
		}
		if err := s.addArmor(ctx, uid, winObj, 1, 1000); err != nil {
			return nil, err
		}
	} else if typ == 0 { // 道具
		winObj, err = s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errLegacy(msgNoSuchGoods)
		}
		if err != nil {
			return nil, err
		}
		if err := s.addGoods(ctx, uid, id, count, 1000); err != nil {
			return nil, err
		}
	} else {
		return nil, errLegacy(msgNoSuchGoods)
	}
	if _, err := s.db.Exec(ctx, "update mem_lottery_goods set got=1 where uid=?", uid); err != nil {
		return nil, err
	}
	// 裁剪：sendSysInformHere 全服滚动公告（sys_inform 无表）→ 不写公告。
	winObj["count"] = count
	return winObj, nil
}

// ── sendSysInformHere（LotteryFunc.php:512）—— 裁剪（广播）──────────────

// sendSysInformHere 原版：type=1 且 armor.level<=5，或 type=0 且 goods.level<=5 时，
// 向 sys_inform 写一条全服滚动公告。裁剪：sys_inform 无表 / 公告广播不在范围内 → 空实现（保留入口）。
func (s *Service) sendSysInformHere(ctx context.Context, uid, typ, id int, count int64) error {
	return nil
}

// ── restart（LotteryFunc.php:538）────────────────────────────────────────

// Restart 对齐 restart($uid,$param)：重开一次（restart_count>=1 → 拒绝），返回 randWin 结果。
func (s *Service) Restart(ctx context.Context, uid, winID, winType int) ([]any, error) {
	var out []any
	err := s.WithUserLock(ctx, uid, "restart", func(ctx context.Context) error {
		open, err := s.lotteryOpen(ctx)
		if err != nil {
			return err
		}
		if !open {
			return errLegacy(msgNotAvailable)
		}
		if err := s.checkLottoryTime(ctx); err != nil {
			return err
		}
		infor, err := s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=? and got=0", uid)
		if errors.Is(err, sql.ErrNoRows) {
			return errLegacy(msgWaiguaInvalid)
		}
		if err != nil {
			return err
		}
		if model.Int(infor, "restart_count") >= 1 {
			return errLegacy(msgRestartLimit)
		}
		records := s.retrieveGoods(ctx, model.Str(infor, "records"))
		r, err := s.randWin(ctx, uid, records, true, winID, winType)
		if err != nil {
			return err
		}
		out = []any{r[0], r[1], r[2]}
		return nil
	})
	return out, err
}

// ── addCount（LotteryFunc.php:566）───────────────────────────────────────

// addCount 对齐 addCount：按当前盘面 win 发奖，返回 winObj。对应 RPC addCount。
func (s *Service) AddCount(ctx context.Context, uid int) (map[string]any, error) {
	var out map[string]any
	err := s.WithUserLock(ctx, uid, "addCount", func(ctx context.Context) error {
		open, err := s.lotteryOpen(ctx)
		if err != nil {
			return err
		}
		if !open {
			return errLegacy(msgNotAvailable)
		}
		if err := s.checkLottoryTime(ctx); err != nil {
			return err
		}
		rec, err := s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=? and got=0", uid)
		if errors.Is(err, sql.ErrNoRows) {
			return errLegacy(msgWaiguaInvalid)
		}
		if err != nil {
			return err
		}
		ary := strings.Split(model.Str(rec, "win"), ",")
		for len(ary) < 3 {
			ary = append(ary, "0")
		}
		typ := atoiSafe(ary[0])
		gid := atoiSafe(ary[1])
		count := int64(atoiSafe(ary[2]))
		w, err := s.getWin(ctx, uid, typ, gid, count)
		if err != nil {
			return err
		}
		// 裁剪：completeTaskWithTaskid(uid,293) 属 M8 任务系统（此处不接线，避免跨包耦合）。
		out = w
		return nil
	})
	return out, err
}

// addCount 内部版本（供 getLotteryReward/autoGetReward 复用，假定已加锁）。
func (s *Service) addCount(ctx context.Context, uid int) (map[string]any, error) {
	rec, err := s.db.FetchOne(ctx, "select * from mem_lottery_goods where uid=? and got=0", uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy(msgWaiguaInvalid)
	}
	if err != nil {
		return nil, err
	}
	ary := strings.Split(model.Str(rec, "win"), ",")
	for len(ary) < 3 {
		ary = append(ary, "0")
	}
	return s.getWin(ctx, uid, atoiSafe(ary[0]), atoiSafe(ary[1]), int64(atoiSafe(ary[2])))
}

// ── checkLotteryMoney / useMoney（LotteryFunc.php:596/609）───────────────

// CheckLotteryMoney 对齐 checkLotteryMoney：money >= 6 返回 true，否则 false（原版注释 10 实为 6）。
func (s *Service) CheckLotteryMoney(ctx context.Context, uid int) (bool, error) {
	const useMoney = 6
	money, err := s.cellInt(ctx, "select money from users where id=?", uid)
	if err != nil {
		return false, err
	}
	if money < useMoney {
		return false, nil
	}
	return true, nil
}

// UseMoney 对齐 useMoney($uid,$count)：count>1 才扣 6 元宝；不足返回 [0]，否则 [1]。
func (s *Service) UseMoney(ctx context.Context, uid, count int) ([]any, error) {
	const useMoney = 6
	ret := []any{}
	if count > 1 {
		money, err := s.cellInt(ctx, "select money from users where id=?", uid)
		if err != nil {
			return nil, err
		}
		if money < useMoney {
			return []any{0}, nil
		}
		if _, err := s.db.Exec(ctx, "update users set money=money-? where id=?", useMoney, uid); err != nil {
			return nil, err
		}
	}
	ret = append(ret, 1)
	return ret, nil
}

// ── 鉴权辅助（nobility 相关，与 M8 同口径）──────────────────────────────

func (s *Service) nobilityOf(ctx context.Context, uid int) (int, error) {
	str, err := s.cellStr(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(str))
	return n, nil
}

// getBufferNobility 对齐 utils.php:1534 getBufferNobility（推恩令 buftype=16/18）。
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

// atoiSafe 解析字符串为 int，失败返回 0（对齐 PHP intval/隐式转换）。
func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
