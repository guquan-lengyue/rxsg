package tavern

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"strconv"
)

// store.go tavern 包内部底层（与 hero/goods 包同口径，避免跨包耦合）：
//   checkGoods(933) / reduceGoods(938) / addCityResources(193) 的按需子集。

// cellInt64Zero 读取单列，无行→0（对齐 legacy sql_fetch_one_cell false 语义）。
func (s *Service) cellInt64Zero(ctx context.Context, query string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// cellFloat 读取单列并按浮点解析（users.nobility 为 varchar，对齐 PHP 松散数值化）。
func (s *Service) cellFloat(ctx context.Context, query string, args ...any) (float64, error) {
	v, err := s.db.FetchCellString(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, nil
	}
	return f, nil
}

// checkGoods 对齐 utils.php:933（>=1）。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	v, err := s.cellInt64Zero(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if err != nil {
		return false, err
	}
	return v >= 1, nil
}

// reduceGoods 对齐 utils.php:938（log_goods 记 -count；GREATEST 夹 0；武魂成就属 M8）。
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

// levelTotalExp 读取 cfg_hero_levels.total_exp；无行返回 ok=false
// （对齐 generateRecruitHero:205-206 empty→throw no_data_of_this_level）。
func (s *Service) levelTotalExp(ctx context.Context, level int) (int64, bool, error) {
	v, err := s.db.FetchCellInt64(ctx, "select total_exp from cfg_hero_levels where level=?", level)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

// randInclusive 对齐 PHP mt_rand(a,b)（含端点）。
func randInclusive(min, max int) int {
	if max < min {
		return 0
	}
	return min + rand.Intn(max-min+1)
}
