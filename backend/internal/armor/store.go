package armor

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
)

// store.go 复刻 legacy utils.php 的元宝/道具/碎片底层，供 armor 包内部使用
// （与 hero/goods 包同名实现相互独立，避免跨包耦合，口径逐字对齐）：
//   addMoney(1119) / checkMoney(1106) / addGoods(971) / reduceGoods(938) /
//   checkGoods(933) / addThings(1030) / getBufferNobility(1534)。
// 降级：log_day_money / log_money_type 表无 → 每日消耗统计省略；
// things 表（← sys_things）由 0009 建表。

// addMoney 对齐 utils.php:1119（money==0 返回；log_money 流水 + users.money 累加，带符号）。
func (s *Service) addMoney(ctx context.Context, uid int, money int64, typ int) error {
	if money == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_money (user_id, `count`, `time`, `type`) values (?,?,unix_timestamp(),?)",
		uid, money, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set money=money+? where id=?", money, uid)
	return err
}

// checkMoney 对齐 utils.php:1106：money 不足（含无行/0）返回 false。
func (s *Service) checkMoneyOK(ctx context.Context, uid int, need int64) (bool, error) {
	v, err := cellInt(ctx, s.db, "select money from users where id=? limit 1", uid)
	if err != nil {
		return false, err
	}
	if v == 0 { // legacy empty($usermoney)
		return false, nil
	}
	return v >= need, nil
}

// addGoods 对齐 utils.php:971（cnt==0 返回；upsert 带符号；log_goods type）。
func (s *Service) addGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, gid, cnt, cnt); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, cnt, typ)
	return err
}

// goodsCount 读取 user_goods.count；无行返回 0（对齐 legacy empty→0）。
func (s *Service) goodsCount(ctx context.Context, uid, gid int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// checkGoods 对齐 utils.php:933（>=1）。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= 1, nil
}

// checkGoodsCount 对齐 utils.php:1159（legacy 内部 lockUser 由上层 WithUserLock 统一持有）。
func (s *Service) checkGoodsCount(ctx context.Context, uid, gid int, need int64) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= need, nil
}

// addThings 对齐 utils.php:1030（cnt==0 返回；log_things 流水 + things upsert 带符号）。
func (s *Service) addThings(ctx context.Context, uid, tid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_things (user_id, tid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, tid, cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into things (user_id, tid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, tid, cnt, cnt)
	return err
}

// getBufNobility 对齐 utils.php:1534 getBufferNobility：
// user_buffers buftype∈{16,18} 按 bufparam 降序取 1 条；bufparam=5 封顶 19、=2 封顶 18。
func (s *Service) getBufNobility(ctx context.Context, uid int, realNobility float64) (int, error) {
	bufparam, err := s.db.FetchCellInt64(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		bufparam = 0
	}
	if bufparam != 0 {
		n := int64(realNobility) + bufparam
		if bufparam == 5 && n > 19 {
			n = 19
		}
		if bufparam == 2 && n > 18 {
			n = 18
		}
		if float64(n) > realNobility {
			return int(n), nil
		}
	}
	return int(realNobility), nil
}

// mtRand 对齐 PHP mt_rand(a,b)（含端点）。
func mtRand(a, b int) int {
	if b < a {
		return 0
	}
	return a + rand.Intn(b-a+1)
}
