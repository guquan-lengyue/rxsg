package hero

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
)

// store.go 复刻 legacy utils.php 的元宝/道具底层，供 hero 包内部使用：
//   addMoney(1119) / checkGoods(933) / checkGoodsCount(1159) / reduceGoods(938) / addGoods(971)。
// 与 goods 包同名实现相互独立（避免跨包耦合），口径逐字对齐：
//   - addMoney：money==0 直接返回；写 log_money 流水 + update users.money；
//     log_day_money 表无 → 每日消耗量统计省略；
//   - reduceGoods：count>0 才处理，log_goods 记 -count，count=GREATEST(0,count-n)；
//   - checkGoodsCount：goodsCount>=need（legacy 内部 lockUser 由上层 WithUserLock 统一持有）。

// addMoney 对齐 utils.php:1119。
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

// goodsCount 读取 user_goods.count；无行返回 0。
func (s *Service) goodsCount(ctx context.Context, uid, gid int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// checkGoods 对齐 utils.php:933（>=1）。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	return s.checkGoodsCount(ctx, uid, gid, 1)
}

// checkGoodsCount 对齐 utils.php:1159。
func (s *Service) checkGoodsCount(ctx context.Context, uid, gid int, need int64) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= need, nil
}

// reduceGoods 对齐 utils.php:938（武魂段成就省略）。
func (s *Service) reduceGoods(ctx context.Context, uid, gid int, cnt int64) error {
	if cnt <= 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),0)",
		uid, gid, -cnt); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update user_goods set `count`=GREATEST(0,`count`-?) where user_id=? and gid=?", cnt, uid, gid)
	return err
}

// addGoods 对齐 utils.php:971（cnt==0 返回；upsert；log_goods type）。
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

// bufferActive 判断 user_buffers 是否存在某 buftype 行（巡查令 buftype=100）。
func (s *Service) bufferActive(ctx context.Context, uid, buftype int) (bool, error) {
	return s.db.Exists(ctx, "select 1 from user_buffers where user_id=? and buftype=? limit 1", uid, buftype)
}

// cityGold 读取城池黄金（mem_city_resource.gold → city_resources.gold）。
func (s *Service) cityGold(ctx context.Context, cid int) (int64, error) {
	return s.cellInt64Zero(ctx, "select gold from city_resources where city_id=?", cid)
}

// userMoney 读取元宝（sys_user.money → users.money）。
func (s *Service) userMoney(ctx context.Context, uid int) (int64, error) {
	return s.cellInt64Zero(ctx, "select money from users where id=?", uid)
}

// heroRowByID 按 hid 取武将行（beginExprHero 只按 hid 查询，不校验 uid/cid——原版怪癖）。
// 无行返回 nil（对齐 legacy sql_fetch_one false → 字段按 null 处理）。
func (s *Service) heroRowByID(ctx context.Context, hid int) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx, "select * from heroes where id=? limit 1", hid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// timeNow 读取数据库时间（unix 秒），失败返回 0（对齐 legacy $GLOBALS['now']）。
func timeNow(ctx context.Context, s *Service) int64 {
	v, err := s.db.Now(ctx)
	if err != nil {
		return 0
	}
	return v
}

// randInclusive 对齐 PHP mt_rand(a,b)（含端点）。
func randInclusive(min, max int) int {
	if max < min {
		return 0
	}
	return min + rand.Intn(max-min+1)
}

// randRange 对齐 HeroExpr.php:1088 randomRange（minValue + mt_rand()%(maxValue+1-minValue)）。
func randRange(minValue, maxValue int) int {
	if maxValue == minValue {
		return minValue
	}
	return minValue + rand.Intn(maxValue+1-minValue)
}
